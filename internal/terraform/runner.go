// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// PluginCache is where providers are kept between runs (a volume in the
// panel's image); a run falls back to its own folder when it isn't there.
const PluginCache = "/var/cache/panel-tofu"

// runTimeout bounds a whole run; maxLog is how much output is kept.
const (
	runTimeout = 2 * time.Hour
	maxLog     = 2 << 20
)

// logBuffer collects a run's output — its end, when long — with every
// sensitive value masked: a provider or a local-exec can print them.
type logBuffer struct {
	mu     sync.Mutex
	b      []byte
	secret []string
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, p...)
	if len(l.b) > maxLog {
		l.b = append([]byte("[… earlier output trimmed …]\n"), l.b[len(l.b)-maxLog:]...)
	}
	return len(p), nil
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := string(l.b)
	for _, v := range l.secret {
		if len(v) >= 3 {
			s = strings.ReplaceAll(s, v, "********")
		}
	}
	return s
}

// Job is what a run needs, decrypted: the code, variables, the state the
// panel keeps (nil when the code has its own backend, or there's none yet)
// and, for an apply, the saved plan.
type Job struct {
	Run     *Run
	Files   []File
	Vars    []Variable // with Value decrypted
	State   []byte
	Managed bool
	Plan    []byte
}

// Result is what a run produced, for the caller to store (encrypted).
type Result struct {
	Plan     []byte // a plan run's saved plan
	State    []byte // the state after the run, when the panel keeps it and it changed
	Serial   int
	LockFile string // the lock file init wrote, when it changed
}

// Runner runs one job per workspace at a time.
type Runner struct {
	busy    sync.Map // workspace ID -> run ID
	cancels sync.Map // run ID -> context.CancelFunc
}

func NewRunner() *Runner { return &Runner{} }

// Busy reports the run going on in a workspace, if any.
func (r *Runner) Busy(workspaceID string) (string, bool) {
	v, ok := r.busy.Load(workspaceID)
	if !ok {
		return "", false
	}
	return v.(string), true
}

// Start runs job in the background; done gets the outcome when it ends,
// with release to free the workspace for its next run — to call once what
// the run left (its state) is saved, and before the run is reported done,
// so a run seen as finished never leaves its workspace still busy.
// It fails if the workspace already has a run going.
func (r *Runner) Start(job Job, saveLog func(string), done func(status, errMsg, log string, res Result, release func())) error {
	if other, loaded := r.busy.LoadOrStore(job.Run.WorkspaceID, job.Run.ID); loaded {
		return fmt.Errorf("run %s is still going in this workspace; wait for it, or cancel it", other)
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	r.cancels.Store(job.Run.ID, cancel)
	// Only this run's hold on the workspace is released: once release has
	// run, the next run may already hold it.
	release := func() { r.busy.CompareAndDelete(job.Run.WorkspaceID, job.Run.ID) }
	go func() {
		defer release()
		defer r.cancels.Delete(job.Run.ID)
		defer cancel()
		out := &logBuffer{}
		for _, v := range job.Vars {
			if v.Sensitive {
				out.secret = append(out.secret, v.Value)
			}
		}
		stop := make(chan struct{})
		go func() {
			t := time.NewTicker(2 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					saveLog(out.String())
				}
			}
		}()
		status, errMsg, res := execute(ctx, job, out)
		close(stop)
		done(status, errMsg, out.String(), res, release)
	}()
	return nil
}

// Cancel stops a run: OpenTofu is interrupted first, so an apply stops
// cleanly and writes the state it got to.
func (r *Runner) Cancel(runID string) bool {
	c, ok := r.cancels.Load(runID)
	if ok {
		c.(context.CancelFunc)()
	}
	return ok
}

func execute(ctx context.Context, job Job, out *logBuffer) (status, errMsg string, res Result) {
	dir, err := os.MkdirTemp("", "panel-tofu-")
	if err != nil {
		return "failed", "making a work folder: " + err.Error(), res
	}
	defer os.RemoveAll(dir)
	write := func(rel string, content []byte) error {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if !strings.HasPrefix(p, dir+string(os.PathSeparator)) {
			return fmt.Errorf("%s is outside the work folder", rel)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		return os.WriteFile(p, content, 0o600)
	}
	lockBefore := ""
	for _, f := range job.Files {
		if generated(f.Path) {
			continue
		}
		if f.Path == LockFile {
			lockBefore = f.Content
		}
		if err := write(f.Path, []byte(f.Content)); err != nil {
			return "failed", "writing " + f.Path + ": " + err.Error(), res
		}
	}
	if job.Managed && len(job.State) > 0 {
		if err := write("terraform.tfstate", job.State); err != nil {
			return "failed", err.Error(), res
		}
	}
	cache := PluginCache
	if st, err := os.Stat(cache); err != nil || !st.IsDir() || syscall.Access(cache, 2) != nil {
		cache = filepath.Join(dir, ".plugins")
		os.MkdirAll(cache, 0o700)
	}
	// A clean environment: never the panel's own (database URL, keys,
	// tokens) — only OpenTofu's settings and the workspace's variables.
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + dir, "LANG=C.UTF-8",
		"TF_IN_AUTOMATION=1", "TF_INPUT=0", "CHECKPOINT_DISABLE=1",
		"TF_PLUGIN_CACHE_DIR=" + cache,
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=1",
	}
	for _, v := range job.Vars {
		if v.Kind == "env" {
			env = append(env, v.Key+"="+v.Value)
		} else {
			env = append(env, "TF_VAR_"+v.Key+"="+v.Value)
		}
	}

	tofu := func(label string, args ...string) error {
		fmt.Fprintf(out, "\n== %s (tofu %s)\n", label, strings.Join(args, " "))
		return command(ctx, dir, env, args, out)
	}
	fail := func(what string, err error) (string, string, Result) {
		if ctx.Err() == context.Canceled {
			return "canceled", "canceled", res
		}
		if ctx.Err() == context.DeadlineExceeded {
			return "failed", fmt.Sprintf("stopped after %s", runTimeout), res
		}
		return "failed", what, res
	}

	if err := tofu("Init: providers and modules", "init", "-input=false", "-no-color"); err != nil {
		return fail("init failed: a provider or module couldn't be installed, or the backend couldn't be reached", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, LockFile)); err == nil && string(b) != lockBefore {
		res.LockFile = string(b)
	}

	switch job.Run.Kind {
	case "plan", "destroy-plan":
		// The standard's formatting rule (TF010), enforced: fmt lists the
		// files that aren't formatted, and exits 3.
		if err := tofu("Standard: formatting (tofu fmt -check)", "fmt", "-check", "-recursive", "-list=true", "-no-color"); err != nil {
			return fail("the code isn't formatted (rule TF010): press Format, save, and plan again", err)
		}
		if err := tofu("Validate", "validate", "-no-color"); err != nil {
			return fail("the configuration isn't valid, so it wasn't planned", err)
		}
		args := []string{"plan", "-input=false", "-no-color", "-out=plan.tfplan"}
		label := "Plan: what applying would change (nothing is changed now)"
		if job.Run.Kind == "destroy-plan" {
			args = append(args, "-destroy")
			label = "Destroy plan: what destroying would remove (nothing is changed now)"
		}
		for _, addr := range job.Run.Options.Replace {
			if !ValidReplaceAddr(addr) {
				return "failed", fmt.Sprintf("%q isn't a resource address to replace", addr), res
			}
			args = append(args, "-replace="+addr)
		}
		if err := tofu(label, args...); err != nil {
			return fail("the plan failed", err)
		}
		plan, err := os.ReadFile(filepath.Join(dir, "plan.tfplan"))
		if err != nil {
			return "failed", "reading the saved plan: " + err.Error(), res
		}
		res.Plan = plan
		var js strings.Builder
		cmd := exec.CommandContext(ctx, "tofu", "show", "-json", "plan.tfplan")
		cmd.Dir, cmd.Env, cmd.Stdout = dir, env, &js
		if err := cmd.Run(); err == nil {
			job.Run.Summary = summarize([]byte(js.String()))
		}
	case "apply":
		if err := write("plan.tfplan", job.Plan); err != nil {
			return "failed", err.Error(), res
		}
		applyErr := tofu("Apply the saved plan, exactly as reviewed", "apply", "-input=false", "-no-color", "plan.tfplan")
		// Whatever happened, keep the state it left: an apply that failed
		// halfway still created or changed real things.
		if job.Managed {
			if b, err := os.ReadFile(filepath.Join(dir, "terraform.tfstate")); err == nil {
				var st struct {
					Serial int `json:"serial"`
				}
				if json.Unmarshal(b, &st) == nil && (len(job.State) == 0 || string(b) != string(job.State)) {
					res.State, res.Serial = b, st.Serial
				}
			}
		}
		if applyErr != nil {
			return fail("the apply failed; what it did change is in the state", applyErr)
		}
	default:
		return "failed", "unknown kind of run " + job.Run.Kind, res
	}
	return "success", "", res
}

type planJSON struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Change  struct {
			Actions   []string        `json:"actions"`
			Importing json.RawMessage `json:"importing"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// summarize counts what a plan does, from "tofu show -json" (verified:
// each resource_changes entry's actions — ["create"], ["update"],
// ["delete"], ["delete","create"] for a replace, ["no-op"], ["read"] — and
// "importing" set for a resource an import block brings in).
func summarize(b []byte) Summary {
	var p planJSON
	s := Summary{Changes: []Change{}}
	if json.Unmarshal(b, &p) != nil {
		return s
	}
	for _, rc := range p.ResourceChanges {
		a := strings.Join(rc.Change.Actions, ",")
		action := ""
		// An import block's resource: imported, and maybe updated too.
		if len(rc.Change.Importing) > 0 && string(rc.Change.Importing) != "null" {
			s.Import++
			s.Changes = append(s.Changes, Change{Address: rc.Address, Action: "import"})
		}
		switch a {
		case "create":
			s.Add++
			action = "create"
		case "update":
			s.Change++
			action = "update"
		case "delete":
			s.Destroy++
			action = "delete"
		case "delete,create", "create,delete":
			s.Replace++
			action = "replace"
		case "read":
			action = "read"
		default:
			continue
		}
		s.Changes = append(s.Changes, Change{Address: rc.Address, Action: action})
	}
	return s
}

// command runs tofu in its own process group. Cancelling interrupts it
// (SIGINT: it stops and saves state), and kills it if it hasn't stopped
// after 30 seconds.
func command(ctx context.Context, dir string, env, args []string, out io.Writer) error {
	cmd := exec.Command("tofu", args...)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dir, env, out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
		select {
		case err := <-done:
			return err
		case <-time.After(30 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			log.Printf("terraform: killed a run that didn't stop on interrupt")
			return <-done
		}
	}
}
