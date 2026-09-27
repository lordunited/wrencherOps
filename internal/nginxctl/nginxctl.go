package nginxctl

import (
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

const mainConfig = "/etc/nginx/nginx.conf"

// IsNginxImage reports whether an image is an Nginx build: the official
// image, nginx-unprivileged, OpenResty and similar.
func IsNginxImage(image string) bool {
	base := dockerx.ImageBase(image)
	return strings.Contains(base, "nginx") || strings.Contains(base, "openresty")
}

// Discover finds the Nginx container of the compose project in deployDir.
func Discover(c *sshexec.Client, deployDir string) (*dockerx.Container, error) {
	out, errOut, err := c.Output("cd " + sshexec.ShellQuote(deployDir) + " && docker compose ps --all --format '{{.Name}}\t{{.Image}}'")
	if err != nil {
		return nil, fmt.Errorf("listing containers in %s failed: %v %s", deployDir, err, strings.TrimSpace(errOut))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, image, ok := strings.Cut(line, "\t")
		if ok && IsNginxImage(image) {
			return Running(c, name)
		}
	}
	return nil, fmt.Errorf("no Nginx container found in %s", deployDir)
}

// Running inspects a container and requires it to be running.
func Running(c *sshexec.Client, name string) (*dockerx.Container, error) {
	ct, err := dockerx.Inspect(c, name)
	if err != nil {
		return nil, err
	}
	if ct.State != "running" {
		return nil, fmt.Errorf("the Nginx container %s is %s; start it first", ct.Name, ct.State)
	}
	return ct, nil
}

// Candidate is an Nginx container found on a server, offered for connecting.
type Candidate struct {
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	State          string   `json:"state"`
	ComposeProject string   `json:"composeProject,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	ComposeFiles   []string `json:"composeFiles,omitempty"`
	ConfigPath     string   `json:"configPath"`
	ConfigHostPath string   `json:"configHostPath,omitempty"`
	ConfDHostPath  string   `json:"confDHostPath,omitempty"`
}

// FindAll lists every Nginx container on the server.
func FindAll(c *sshexec.Client) ([]Candidate, error) {
	cts, err := dockerx.FindByImage(c, IsNginxImage)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, ct := range cts {
		project, dir, files := ct.Compose()
		cand := Candidate{Container: ct.Name, Image: ct.Image, State: ct.State, ComposeProject: project,
			WorkingDir: dir, ComposeFiles: files, ConfigPath: mainConfig}
		cand.ConfigHostPath, _ = ct.HostPath(mainConfig)
		cand.ConfDHostPath, _ = ct.HostPath("/etc/nginx/conf.d")
		out = append(out, cand)
	}
	return out, nil
}

// Report is everything the panel shows about an Nginx.
type Report struct {
	Container  string         `json:"container"`
	Image      string         `json:"image"`
	Version    string         `json:"version"`
	TestOK     bool           `json:"testOk"`
	TestOutput string         `json:"testOutput"`
	Files      []dockerx.File `json:"files"`
	Overview   Overview       `json:"overview"`
	// NewFileDir is where new server blocks go: an included folder,
	// preferably one on the server's disk.
	NewFileDir       string   `json:"newFileDir"`
	NewFileDirOnDisk bool     `json:"newFileDirOnDisk"`
	RecentErrors     []string `json:"recentErrors"`
	Stats            Stats    `json:"stats"`
}

// Stats is the container's resource use right now.
type Stats struct {
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	MemShare string `json:"memShare"`
	NetIO    string `json:"netIO"`
	PIDs     string `json:"pids"`
}

// skippedFiles are reference files nginx -T lists that nobody edits.
var skippedFiles = regexp.MustCompile(`(^|/)(mime\.types|[a-z_]*_params|koi-utf|koi-win|win-utf)$`)

var fileMarker = regexp.MustCompile(`(?m)^# configuration file (.+):$`)

// Inspect reads the running Nginx's configuration and state.
func Inspect(c *sshexec.Client, ct *dockerx.Container) (*Report, error) {
	r := &Report{Container: ct.Name, Image: ct.Image, RecentErrors: []string{}}
	var dumpOut string
	var g errgroup.Group
	g.SetLimit(4)
	g.Go(func() error {
		_, v, _ := c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -v")
		r.Version = strings.TrimPrefix(strings.TrimSpace(v), "nginx version: ")
		return nil
	})
	g.Go(func() error {
		out, errOut, err := c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -t")
		r.TestOK = err == nil
		r.TestOutput = strings.TrimSpace(out + errOut)
		return nil
	})
	g.Go(func() error {
		dumpOut, _, _ = c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -T")
		return nil
	})
	g.Go(func() error {
		out, _, _ := c.Output("docker logs --tail 400 " + sshexec.ShellQuote(ct.Name) + " 2>&1 | grep -E '\\[(emerg|alert|crit|error)\\]' | tail -n 15")
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				r.RecentErrors = append(r.RecentErrors, l)
			}
		}
		return nil
	})
	g.Go(func() error {
		out, _, _ := c.Output("docker stats --no-stream --format '{{json .}}' " + sshexec.ShellQuote(ct.Name))
		var st struct{ CPUPerc, MemUsage, MemPerc, NetIO, PIDs string }
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &st) == nil {
			r.Stats = Stats{CPU: st.CPUPerc, Memory: st.MemUsage, MemShare: st.MemPerc, NetIO: st.NetIO, PIDs: st.PIDs}
		}
		return nil
	})
	g.Wait()

	// The files that make up the running config, as nginx itself resolves
	// the includes. If the config is broken nginx -T prints nothing, so
	// fall back to the usual locations.
	var paths []string
	for _, m := range fileMarker.FindAllStringSubmatch(dumpOut, -1) {
		if !skippedFiles.MatchString(m[1]) {
			paths = append(paths, m[1])
		}
	}
	if len(paths) == 0 {
		paths = []string{mainConfig, "/etc/nginx/conf.d/*.conf", "/etc/nginx/sites-enabled/*", "/etc/nginx/snippets/*"}
	}
	files, err := dockerx.ReadFiles(c, ct, paths)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(a, b int) bool {
		if (files[a].Path == mainConfig) != (files[b].Path == mainConfig) {
			return files[a].Path == mainConfig
		}
		return files[a].Path < files[b].Path
	})
	if files == nil {
		files = []dockerx.File{}
	}
	r.Files = files

	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Content
	}
	r.Overview = Build(byPath, mainConfig)
	r.NewFileDir = newFileDir(r.Overview.Globals.Includes)
	_, r.NewFileDirOnDisk = ct.HostPath(r.NewFileDir)
	return r, nil
}

// newFileDir picks the folder new server blocks go into: one the http
// block already includes with a *.conf (or *) glob.
func newFileDir(includes []string) string {
	for _, preferred := range []string{"/etc/nginx/conf.d", "/etc/nginx/sites-enabled"} {
		for _, inc := range includes {
			if path.Dir(inc) == preferred || strings.TrimPrefix(path.Dir(inc), "/etc/nginx/") == strings.TrimPrefix(preferred, "/etc/nginx/") {
				return preferred
			}
		}
	}
	for _, inc := range includes {
		if strings.Contains(path.Base(inc), "*") {
			dir := path.Dir(inc)
			if !path.IsAbs(dir) {
				dir = path.Join("/etc/nginx", dir)
			}
			return dir
		}
	}
	return "/etc/nginx/conf.d"
}

// Change is a new version of one config file, or its removal.
type Change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	BaseSHA string `json:"baseSha"` // "" for a new file
	Delete  bool   `json:"delete"`
}

// Apply writes changes, tests the whole configuration with nginx -t, and
// reloads gracefully. If the test fails every file is put back and nothing
// is reloaded, so Nginx keeps serving the old configuration throughout.
func Apply(c *sshexec.Client, ct *dockerx.Container, changes []Change, log io.Writer) error {
	if len(changes) == 0 {
		return fmt.Errorf("nothing to apply")
	}
	var paths []string
	for _, ch := range changes {
		if !path.IsAbs(ch.Path) {
			return fmt.Errorf("%s is not an absolute path", ch.Path)
		}
		paths = append(paths, ch.Path)
	}

	fmt.Fprintln(log, "Checking that the files haven't changed on the server since they were read...")
	current, err := dockerx.ReadFiles(c, ct, paths)
	if err != nil {
		return err
	}
	now := map[string]string{}
	for _, f := range current {
		now[f.Path] = f.SHA
	}
	for _, ch := range changes {
		got, exists := now[ch.Path]
		switch {
		case ch.BaseSHA == "" && exists:
			return fmt.Errorf("%s already exists on the server; inspect again and retry", ch.Path)
		case ch.BaseSHA != "" && !exists:
			return fmt.Errorf("%s no longer exists on the server; inspect again and retry", ch.Path)
		case ch.BaseSHA != "" && got != ch.BaseSHA:
			return fmt.Errorf("%s was changed on the server since it was read; inspect again and retry", ch.Path)
		}
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	var written []Change
	restoreAll := func() {
		for _, ch := range written {
			if err := dockerx.RestoreFile(c, ct, ch.Path, stamp); err != nil {
				fmt.Fprintf(log, "Could not restore %s: %v\n", ch.Path, err)
			} else {
				fmt.Fprintf(log, "Restored %s\n", ch.Path)
			}
		}
	}
	for _, ch := range changes {
		var where string
		var err error
		if ch.Delete {
			where, err = deleteFile(c, ct, ch.Path, stamp)
		} else {
			where, err = dockerx.WriteFile(c, ct, ch.Path, ch.Content, stamp)
		}
		if err != nil {
			restoreAll()
			return fmt.Errorf("writing %s: %v", ch.Path, err)
		}
		written = append(written, ch)
		if ch.Delete {
			fmt.Fprintf(log, "Removed %s\n", where)
		} else {
			fmt.Fprintf(log, "Wrote %s\n", where)
		}
	}

	fmt.Fprintln(log, "Testing the configuration (nginx -t)...")
	out, errOut, err := c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -t")
	test := strings.TrimSpace(out + errOut)
	fmt.Fprintln(log, indent(test))
	if err != nil {
		fmt.Fprintln(log, "The test failed. Putting the previous files back; Nginx was not reloaded.")
		restoreAll()
		return fmt.Errorf("nginx -t rejected the change, so nothing was reloaded: %s", lastLine(test))
	}

	fmt.Fprintln(log, "Reloading Nginx gracefully (nginx -s reload)...")
	if _, errOut, err := c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -s reload"); err != nil {
		fmt.Fprintf(log, "Reload failed: %v %s\nPutting the previous files back...\n", err, strings.TrimSpace(errOut))
		restoreAll()
		c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -s reload")
		return fmt.Errorf("Nginx did not reload; the previous files were restored")
	}

	// A new file only takes effect if something includes it.
	dump, _, _ := c.Output("docker exec " + sshexec.ShellQuote(ct.Name) + " nginx -T")
	for _, ch := range changes {
		if !ch.Delete && ch.BaseSHA == "" && !strings.Contains(dump, "# configuration file "+ch.Path+":") {
			fmt.Fprintf(log, "Note: %s is not included by nginx.conf, so it has no effect yet.\n", ch.Path)
		}
	}
	fmt.Fprintln(log, "Nginx reloaded with the new configuration. Open connections were not dropped.")
	return nil
}

func deleteFile(c *sshexec.Client, ct *dockerx.Container, p, stamp string) (string, error) {
	// Back it up by "writing" its current content, then remove it.
	cur, err := dockerx.ReadFiles(c, ct, []string{p})
	if err != nil || len(cur) == 0 {
		return "", fmt.Errorf("could not read %s before removing it", p)
	}
	if _, err := dockerx.WriteFile(c, ct, p, cur[0].Content, stamp); err != nil {
		return "", err
	}
	if host, ok := ct.HostPath(p); ok {
		cmd := `if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; else AS_ROOT="sudo -n"; fi; $AS_ROOT rm -f ` + sshexec.ShellQuote(host)
		if _, errOut, err := c.Output(cmd); err != nil {
			return "", fmt.Errorf("%v %s", err, errOut)
		}
		return host + " on the server (kept at " + dockerx.BackupPath(host, stamp) + ")", nil
	}
	if _, errOut, err := c.Output(dockerx.Exec(ct.Name, true, `rm -f "$1"`, p)); err != nil {
		return "", fmt.Errorf("%v %s", err, errOut)
	}
	return p + " inside the container", nil
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "[emerg]") || strings.Contains(lines[i], "failed") {
			return strings.TrimSpace(lines[i])
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}
