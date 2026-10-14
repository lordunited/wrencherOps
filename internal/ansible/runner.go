// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// Login is how to reach one target server.
type Login struct {
	Target
	Port     int
	Password string
}

// runTimeout bounds a whole run.
const runTimeout = time.Hour

// maxLog is how much of a run's output is kept: its end, when it's longer.
const maxLog = 2 << 20

// ansibleCfg is the run's configuration: no host key prompts (the servers
// are the panel's own, already logged in to), password logins only, no
// colors or retry files, and Python found on its own on each server.
const ansibleCfg = `[defaults]
host_key_checking = False
retry_files_enabled = False
interpreter_python = auto_silent
timeout = 20
forks = 10
nocows = 1
callback_result_format = yaml

[ssh_connection]
ssh_args = -o ControlMaster=auto -o ControlPersist=60s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive
`

var hostNameRe = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// Ansible splits an inventory line like a shell does, so a host or user
// with a space, quote, "=" or backslash would add variables of its own
// (ansible_ssh_common_args runs commands on the panel). Only plain values
// go in: hostnames, IPv4, IPv6 (with a zone), and the usual user names.
var (
	inventoryHostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:%-]*$`)
	inventoryUserRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._@$-]*$`)
)

// ErrBadTarget is a server whose address or user can't be put in an
// inventory safely.
var ErrBadTarget = errors.New("can't be used in an Ansible inventory")

// inventory names each server for Ansible (from its name, made unique) and
// points its password and become password at an environment variable, so
// no password is ever written to disk.
func inventory(logins []Login) (string, []Target, []string, error) {
	var b strings.Builder
	b.WriteString("[targets]\n")
	used := map[string]bool{}
	targets := make([]Target, len(logins))
	env := make([]string, len(logins))
	for i, l := range logins {
		if !inventoryHostRe.MatchString(l.Host) {
			return "", nil, nil, fmt.Errorf("server %s: address %q %w", l.Name, l.Host, ErrBadTarget)
		}
		if !inventoryUserRe.MatchString(l.User) {
			return "", nil, nil, fmt.Errorf("server %s: SSH user %q %w", l.Name, l.User, ErrBadTarget)
		}
		name := strings.Trim(hostNameRe.ReplaceAllString(l.Name, "_"), "_.-")
		if name == "" {
			name = "server"
		}
		base := name
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s_%d", base, n)
		}
		used[name] = true
		v := fmt.Sprintf("PANEL_PW_%d", i)
		fmt.Fprintf(&b, "%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=\"{{ lookup('env', '%s') }}\" ansible_become_password=\"{{ lookup('env', '%s') }}\"\n",
			name, l.Host, l.Port, l.User, v, v)
		t := l.Target
		t.Name = name
		targets[i] = t
		env[i] = v + "=" + l.Password
	}
	return b.String(), targets, env, nil
}

// ValidateExtraVars checks extra vars are a YAML (or JSON) mapping.
func ValidateExtraVars(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal([]byte(s), &m); err != nil {
		return fmt.Errorf("extra vars must be YAML or JSON key: value pairs: %v", err)
	}
	return nil
}

var recapRe = regexp.MustCompile(`(?m)^(\S+)\s+:\s+ok=(\d+)\s+changed=(\d+)\s+unreachable=(\d+)\s+failed=(\d+)(?:\s+skipped=(\d+))?(?:\s+rescued=(\d+))?(?:\s+ignored=(\d+))?`)

// parseRecap reads the PLAY RECAP lines of a run's output.
func parseRecap(out string) []HostRecap {
	i := strings.LastIndex(out, "PLAY RECAP")
	if i < 0 {
		return []HostRecap{}
	}
	var recap []HostRecap
	for _, m := range recapRe.FindAllStringSubmatch(out[i:], -1) {
		n := func(k int) int { v, _ := strconv.Atoi(m[k]); return v }
		recap = append(recap, HostRecap{Host: m[1], OK: n(2), Changed: n(3), Unreachable: n(4), Failed: n(5), Skipped: n(6), Rescued: n(7), Ignored: n(8)})
	}
	if recap == nil {
		return []HostRecap{}
	}
	return recap
}

// logBuffer collects a run's output, keeping its end when it's long. Every
// saved password of the run's servers is masked in what it gives back: a
// task on localhost, or a lookup('env', …), could otherwise print them —
// the controller holds them, and local tasks run as the controller.
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
	for _, p := range l.secret {
		if p != "" {
			s = strings.ReplaceAll(s, p, "********")
		}
	}
	return s
}

// Runner runs playbooks, one goroutine per run.
type Runner struct {
	store *Store
	// cancels holds each running run's cancel function, by ID.
	cancels sync.Map
}

func NewRunner(store *Store) *Runner {
	return &Runner{store: store}
}

// Start runs a playbook in the background and returns the run, already
// saved as running. mode is "check" (--check --diff) or "apply" (--diff).
func (r *Runner) Start(ctx context.Context, pb *Playbook, logins []Login, mode, extraVars string) (*Run, error) {
	inv, targets, pwEnv, err := inventory(logins)
	if err != nil {
		return nil, err
	}
	run := &Run{PlaybookID: pb.ID, PlaybookName: pb.Name, Mode: mode, ContentSHA: ContentSHA(pb.Files, pb.Entry, extraVars), Targets: targets, ExtraVars: extraVars}
	if err := r.store.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), runTimeout)
	r.cancels.Store(run.ID, cancel)
	files := append([]File(nil), pb.Files...)
	entry := pb.Entry
	go func() {
		defer cancel()
		defer r.cancels.Delete(run.ID)
		r.execute(runCtx, run, files, entry, inv, pwEnv)
	}()
	return run, nil
}

// Cancel stops a running run. It reports false if it isn't running here.
func (r *Runner) Cancel(id string) bool {
	c, ok := r.cancels.Load(id)
	if ok {
		c.(context.CancelFunc)()
	}
	return ok
}

func (r *Runner) execute(ctx context.Context, run *Run, files []File, entry, inv string, pwEnv []string) {
	out := &logBuffer{}
	for _, kv := range pwEnv {
		_, pw, _ := strings.Cut(kv, "=")
		out.secret = append(out.secret, pw)
	}
	finish := func(status, errMsg string) {
		run.Status, run.Error, run.Log = status, errMsg, out.String()
		run.Recap = parseRecap(run.Log)
		if err := r.store.FinishRun(context.Background(), run); err != nil {
			log.Printf("ansible: saving run %s: %v", run.ID, err)
		}
	}

	dir, err := os.MkdirTemp("", "panel-ansible-")
	if err != nil {
		finish("failed", "making a work folder: "+err.Error())
		return
	}
	defer os.RemoveAll(dir)
	write := func(rel, content string, mode os.FileMode) error {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if !strings.HasPrefix(p, dir+string(os.PathSeparator)) {
			return fmt.Errorf("%s is outside the work folder", rel)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(content), mode)
	}
	for _, f := range files {
		if err := write(f.Path, f.Content, 0o600); err != nil {
			finish("failed", "writing "+f.Path+": "+err.Error())
			return
		}
	}
	if err := write("ansible.cfg", ansibleCfg, 0o600); err != nil {
		finish("failed", err.Error())
		return
	}
	if err := write("inventory.ini", inv, 0o600); err != nil {
		finish("failed", err.Error())
		return
	}
	args := []string{"-i", "inventory.ini", entry}
	if strings.TrimSpace(run.ExtraVars) != "" {
		if err := write("extra_vars.yml", run.ExtraVars, 0o600); err != nil {
			finish("failed", err.Error())
			return
		}
		args = append(args, "-e", "@extra_vars.yml")
	}

	// A clean environment: never the panel's own (its database URL, keys
	// and tokens), only what Ansible needs and the servers' passwords.
	env := []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + dir, "LANG=C.UTF-8", "PYTHONUNBUFFERED=1",
		"ANSIBLE_CONFIG=" + filepath.Join(dir, "ansible.cfg"),
		"ANSIBLE_LOCAL_TEMP=" + filepath.Join(dir, ".ansible", "tmp"),
		"ANSIBLE_SSH_CONTROL_PATH_DIR=" + filepath.Join(dir, "cp"),
		"ANSIBLE_NOCOLOR=1", "ANSIBLE_FORCE_COLOR=0",
	}
	env = append(env, pwEnv...)

	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				r.store.SaveLog(context.Background(), run.ID, out.String())
			}
		}
	}()
	defer close(stop)

	fmt.Fprintf(out, "== Syntax check (ansible-playbook --syntax-check)\n")
	if err := r.command(ctx, dir, env, append(append([]string{}, args...), "--syntax-check"), out); err != nil {
		if ctx.Err() == context.Canceled {
			finish("canceled", "canceled")
			return
		}
		finish("failed", "the playbook has a syntax error, so it wasn't run")
		return
	}
	switch run.Mode {
	case "check":
		fmt.Fprintf(out, "\n== Dry run (--check --diff): shows what would change, changes nothing\n")
		args = append(args, "--check", "--diff")
	default:
		fmt.Fprintf(out, "\n== Run (--diff)\n")
		args = append(args, "--diff")
	}
	err = r.command(ctx, dir, env, args, out)
	switch {
	case ctx.Err() == context.Canceled:
		finish("canceled", "canceled")
	case ctx.Err() == context.DeadlineExceeded:
		finish("failed", fmt.Sprintf("stopped after %s", runTimeout))
	case err != nil:
		recap := parseRecap(out.String())
		failed, unreachable := 0, 0
		for _, h := range recap {
			if h.Failed > 0 {
				failed++
			}
			if h.Unreachable > 0 {
				unreachable++
			}
		}
		msg := "ansible-playbook failed: " + err.Error()
		if failed+unreachable > 0 {
			msg = fmt.Sprintf("%d server(s) failed, %d unreachable", failed, unreachable)
		}
		finish("failed", msg)
	default:
		finish("success", "")
	}
}

// command runs ansible-playbook in its own process group, so cancelling
// stops it and every SSH connection it opened.
func (r *Runner) command(ctx context.Context, dir string, env, args []string, out io.Writer) error {
	cmd := exec.Command("ansible-playbook", args...)
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
		syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return <-done
		}
	}
}
