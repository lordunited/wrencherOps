// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package haproxyctl finds HAProxy containers on a remote server and
// determines how to check whether one is actually healthy. Unlike the
// panel's own generated stacks (which always ship a known /healthz), a
// connected HAProxy's config is arbitrary — there's no fixed convention to
// rely on, so this reads the config the container actually runs and picks
// the best probe it can find in it: a monitor-uri, else the stats page,
// else just whether any bound port accepts a connection at all.
package haproxyctl

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

// IsHAProxyImage reports whether an image is an HAProxy build.
func IsHAProxyImage(image string) bool {
	return dockerx.ImageBase(image) == "haproxy"
}

// Running inspects a container and requires it to be running.
func Running(c *sshexec.Client, name string) (*dockerx.Container, error) {
	ct, err := dockerx.Inspect(c, name)
	if err != nil {
		return nil, err
	}
	if ct.State != "running" {
		return nil, fmt.Errorf("the HAProxy container %s is %s; start it first", ct.Name, ct.State)
	}
	return ct, nil
}

// Discover finds the HAProxy container of the compose project in deployDir,
// for a service whose pinned container name hasn't been recorded yet (a
// fresh deploy, before the first sync) — mirrors nginxctl.Discover.
func Discover(c *sshexec.Client, deployDir string) (*dockerx.Container, error) {
	out, errOut, err := c.Output(sshexec.AsRootPrelude + sshexec.ComposeIn(deployDir) + " ps --all --format '{{.Name}}\t{{.Image}}'")
	if err != nil {
		return nil, fmt.Errorf("listing containers in %s failed: %v %s", deployDir, err, strings.TrimSpace(errOut))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, image, ok := strings.Cut(line, "\t")
		if ok && IsHAProxyImage(image) {
			return Running(c, name)
		}
	}
	return nil, fmt.Errorf("no HAProxy container found in %s", deployDir)
}

// Candidate is an HAProxy container found on a server, offered for connecting.
type Candidate struct {
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	State          string   `json:"state"`
	ComposeProject string   `json:"composeProject,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	ComposeFiles   []string `json:"composeFiles,omitempty"`
	// ConfigPaths is every "-f" argument the container was started with
	// (haproxy's own default when there's none).
	ConfigPaths []string `json:"configPaths"`
}

// FindAll lists every HAProxy container on the server.
func FindAll(c *sshexec.Client) ([]Candidate, error) {
	cts, err := dockerx.FindByImage(c, IsHAProxyImage)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, ct := range cts {
		project, dir, files := ct.Compose()
		out = append(out, Candidate{
			Container: ct.Name, Image: ct.Image, State: ct.State,
			ComposeProject: project, WorkingDir: dir, ComposeFiles: files,
			ConfigPaths: configPaths(ct.Args),
		})
	}
	return out, nil
}

// configPaths extracts every "-f <path>" argument haproxy was started
// with — repeatable, and each may be a file or a directory of *.cfg
// fragments — falling back to the official image's own default when the
// container's command has none (verified against a real container: with
// no override, .Args is exactly ["haproxy", "-f",
// "/usr/local/etc/haproxy/haproxy.cfg"]).
func configPaths(args []string) []string {
	var out []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-f" {
			out = append(out, args[i+1])
		}
	}
	if len(out) == 0 {
		out = []string{"/usr/local/etc/haproxy/haproxy.cfg"}
	}
	return out
}

// ReadConfig reads every config file this container actually uses,
// concatenated. Each configured path is tried both as a file and as a
// directory of *.cfg fragments — dockerx.ReadFiles silently skips whatever
// doesn't exist, so trying both is always safe.
func ReadConfig(c *sshexec.Client, ct *dockerx.Container) (string, error) {
	raw := configPaths(ct.Args)
	var patterns []string
	for _, p := range raw {
		patterns = append(patterns, p, strings.TrimRight(p, "/")+"/*.cfg")
	}
	files, err := dockerx.ReadFiles(c, ct, patterns)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no readable config file matched %s", strings.Join(raw, ", "))
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f.Content)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// bindPortRe pulls the port out of a bind address: "*:80", ":80",
// "0.0.0.0:8404", "127.0.0.1:80-85" (a range — the first port is used).
// Binds with no port at all (a Unix socket path) don't match and are
// skipped by firstPort.
var bindPortRe = regexp.MustCompile(`:(\d+)(-\d+)?(\s|$)`)

func bindPort(bind string) (int, bool) {
	m := bindPortRe.FindStringSubmatch(bind)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func firstPort(binds []string) (int, bool) {
	for _, b := range binds {
		if p, ok := bindPort(b); ok {
			return p, true
		}
	}
	return 0, false
}

// Probe describes how to check whether a connected HAProxy is alive,
// determined from its own config.
type Probe struct {
	Kind    string `json:"kind"` // "monitor-uri", "stats", "port" or "none"
	Port    int    `json:"port,omitempty"`
	Path    string `json:"path,omitempty"`
	Section string `json:"section,omitempty"` // e.g. "frontend main", for the detail message
}

// DetermineProbe picks the best health probe it can find: a monitor-uri
// (answers with no backend involved, exactly what the panel's own
// generated configs use), else the stats page (default URI
// "/haproxy?stats" when "stats uri" isn't set — verified against a real
// container), else just whether any frontend/listen's bound port accepts a
// connection at all.
func DetermineProbe(sections []section) Probe {
	// monitor-uri and stats are HTTP-only: in TCP mode HAProxy ignores them
	// with just a warning, so they'd never answer.
	modes := effectiveModes(sections)
	for i, s := range sections {
		if s.monitorURI == "" || modes[i] != "http" {
			continue
		}
		if port, ok := firstPort(s.binds); ok {
			return Probe{Kind: "monitor-uri", Port: port, Path: s.monitorURI, Section: s.describe()}
		}
	}
	if port, uri, ok := statsEndpoint(sections); ok {
		for _, s := range sections {
			if s.statsEnabled {
				if p, _ := firstPort(s.binds); p == port {
					return Probe{Kind: "stats", Port: port, Path: uri, Section: s.describe()}
				}
			}
		}
	}
	for _, s := range sections {
		if s.kind != "frontend" && s.kind != "listen" {
			continue
		}
		if port, ok := firstPort(s.binds); ok {
			return Probe{Kind: "port", Port: port, Section: s.describe()}
		}
	}
	return Probe{Kind: "none"}
}

// httpStatusRe extracts the status code wget names on a failed request —
// verified against a real container: this image's wget prints no
// parseable line on success (exit 0, just a progress bar), and on failure
// either names an HTTP status ("server returned error: HTTP/1.1 503 ...")
// or a connection failure ("Connection refused" / "Connection timed out" /
// "No route to host") with no HTTP/ text at all — which is what actually
// distinguishes "up but erroring" from "not there."
var httpStatusRe = regexp.MustCompile(`HTTP/\d\.\d (\d+)`)

// probeHTTP requests path on port from inside the container. status is the
// HTTP status if the server answered at all (200 on outright success,
// since wget prints no code in that case); reachable is false only when
// the connection itself failed.
func probeHTTP(c *sshexec.Client, ct *dockerx.Container, port int, path string) (status int, reachable bool) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	out, _, err := c.Output(dockerx.Exec(ct.Name, false, `wget -T 3 -O /dev/null "$1" 2>&1`, url))
	if err == nil {
		return 200, true
	}
	if m := httpStatusRe.FindStringSubmatch(out); m != nil {
		code, _ := strconv.Atoi(m[1])
		return code, true
	}
	return 0, false
}

// CheckHealth reads ct's actual config, determines the best probe it can,
// and checks it.
func CheckHealth(c *sshexec.Client, ct *dockerx.Container) (healthy bool, detail string) {
	cfg, err := ReadConfig(c, ct)
	if err != nil {
		return false, "could not read haproxy config: " + err.Error()
	}
	probe := DetermineProbe(parseSections(cfg))
	return evaluateProbe(c, ct, probe)
}

// evaluateProbe checks an already-determined probe, separated from
// CheckHealth so Inspect can reuse the probe it just parsed instead of
// reading the config a second time.
func evaluateProbe(c *sshexec.Client, ct *dockerx.Container, probe Probe) (healthy bool, detail string) {
	switch probe.Kind {
	case "none":
		return true, "container running; no frontend/listen with a bind found in its config to probe"
	case "port":
		_, reachable := probeHTTP(c, ct, probe.Port, "/")
		if reachable {
			return true, fmt.Sprintf("port %d (%s) is accepting connections; no monitor-uri or stats page found to check more precisely", probe.Port, probe.Section)
		}
		return false, fmt.Sprintf("port %d (%s) refused a connection", probe.Port, probe.Section)
	default: // "monitor-uri" or "stats"
		label := "monitor-uri"
		if probe.Kind == "stats" {
			label = "stats page"
		}
		status, reachable := probeHTTP(c, ct, probe.Port, probe.Path)
		if !reachable {
			return false, fmt.Sprintf("%s %s on :%d (%s) did not respond", label, probe.Path, probe.Port, probe.Section)
		}
		healthy := status >= 200 && status < 400
		return healthy, fmt.Sprintf("%s %s on :%d (%s) -> HTTP %d", label, probe.Path, probe.Port, probe.Section, status)
	}
}

// Stats is the container's resource use right now.
type Stats struct {
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	MemShare string `json:"memShare"`
	NetIO    string `json:"netIO"`
	PIDs     string `json:"pids"`
}

// Report is everything the panel shows about a connected HAProxy.
type Report struct {
	Container    string         `json:"container"`
	Image        string         `json:"image"`
	Version      string         `json:"version"`
	ConfigPaths  []string       `json:"configPaths"`
	TestOK       bool           `json:"testOk"`
	TestOutput   string         `json:"testOutput"`
	Files        []dockerx.File `json:"files"`
	Sections     []SectionView  `json:"sections"`
	Probe        Probe          `json:"probe"`
	Healthy      bool           `json:"healthy"`
	HealthDetail string         `json:"healthDetail"`
	RecentErrors []string       `json:"recentErrors"`
	Stats        Stats          `json:"stats"`
	// HasStatsPage is false when nothing in the config enables "stats" —
	// in that case BackendServers is always empty, since HAProxy's own
	// stats page is the only place per-server UP/DOWN status exists; the
	// static config only says what servers WOULD be there, not whether
	// their health check is currently passing.
	HasStatsPage   bool            `json:"hasStatsPage"`
	BackendServers []BackendServer `json:"backendServers"`
}

// BackendServer is one backend server's live status, exactly as HAProxy's
// own stats page reports it right now (not the static config's server
// line, which says nothing about whether its health check is passing).
type BackendServer struct {
	Backend     string `json:"backend"`
	Name        string `json:"name"`
	Status      string `json:"status"` // "UP", "DOWN", "MAINT", "DRAIN", "NOLB", "no check", ...
	Weight      string `json:"weight"`
	CheckStatus string `json:"checkStatus,omitempty"`
	CheckDesc   string `json:"checkDesc,omitempty"`
	LastCheck   string `json:"lastCheck,omitempty"`         // detail on the last check, e.g. a connection error
	LastChange  string `json:"lastChangeSeconds,omitempty"` // seconds since the status last changed
	Downtime    string `json:"downtimeSeconds,omitempty"`
}

// statsEndpoint finds the first section with a stats page enabled and a
// bound port. Separate from DetermineProbe, which picks ONE probe by
// priority for the health check (a monitor-uri wins there even when stats
// is also enabled) — backend server status can only come from a stats
// page, so this looks for one directly, independent of what was chosen as
// the primary health probe.
//
// A stats page in TCP mode doesn't count: verified against a real
// container that HAProxy ignores "stats" there with only a warning ("haproxy
// -c" still passes), so it would otherwise look enabled and never answer.
// A section's mode is its own, else the one from the nearest "defaults"
// above it — the same inheritance HAProxy itself applies.
func statsEndpoint(sections []section) (port int, uri string, ok bool) {
	modes := effectiveModes(sections)
	for i, s := range sections {
		if !s.statsEnabled || modes[i] != "http" {
			continue
		}
		if p, has := firstPort(s.binds); has {
			u := s.statsURI
			if u == "" {
				u = "/haproxy?stats"
			}
			return p, u, true
		}
	}
	return 0, "", false
}

// parseStatsCSV reads HAProxy's stats page CSV output (the ";csv" variant
// of its stats URI) and returns each individual server's row — the
// "FRONTEND" and "BACKEND" aggregate rows every proxy also reports are
// skipped. Columns are looked up by name from the header line (which
// HAProxy always sends first, prefixed with "# ") rather than by a fixed
// position: verified against a real container that the column set is
// large and version-dependent (HAProxy 3.4 reports well over 150 of
// them, many QUIC/HTTP3-specific), so a name lookup is the only way this
// keeps working across versions.
func parseStatsCSV(raw string) []BackendServer {
	r := csv.NewReader(strings.NewReader(raw))
	r.FieldsPerRecord = -1 // rows aren't guaranteed to match the header's width across versions
	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		return nil
	}
	header := records[0]
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "# ")
	}
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[h] = i
	}
	field := func(row []string, name string) string {
		i, ok := idx[name]
		if !ok || i >= len(row) {
			return ""
		}
		return row[i]
	}

	var out []BackendServer
	for _, row := range records[1:] {
		sv := field(row, "svname")
		if sv == "" || sv == "FRONTEND" || sv == "BACKEND" {
			continue
		}
		out = append(out, BackendServer{
			Backend:     field(row, "pxname"),
			Name:        sv,
			Status:      field(row, "status"),
			Weight:      field(row, "weight"),
			CheckStatus: field(row, "check_status"),
			CheckDesc:   field(row, "check_desc"),
			LastCheck:   field(row, "last_chk"),
			LastChange:  field(row, "lastchg"),
			Downtime:    field(row, "downtime"),
		})
	}
	return out
}

// testCommand builds "haproxy -c -f p1 -f p2 ...": the same set of -f
// arguments the container itself runs with, so the test resolves includes
// and fragment directories exactly like the real process does.
func testCommand(paths []string) string {
	var b strings.Builder
	b.WriteString("haproxy -c")
	for _, p := range paths {
		b.WriteString(" -f ")
		b.WriteString(sshexec.ShellQuote(p))
	}
	return b.String()
}

// firstLine trims "haproxy -v" down to its version line.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(s, "HAProxy version ")
}

// Inspect reads the running HAProxy's configuration and state: its version,
// a config-test result (haproxy -c, the same pre-flight check Apply runs
// before ever reloading), every config file, the parsed section structure,
// the health probe determined from that structure and whether it currently
// passes, recent log errors, and live resource stats.
func Inspect(c *sshexec.Client, ct *dockerx.Container) (*Report, error) {
	r := &Report{Container: ct.Name, Image: ct.Image, ConfigPaths: configPaths(ct.Args), RecentErrors: []string{}}
	var g errgroup.Group
	g.SetLimit(4)
	g.Go(func() error {
		_, v, _ := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(ct.Name) + " haproxy -v"))
		r.Version = firstLine(v)
		return nil
	})
	g.Go(func() error {
		out, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(ct.Name) + " " + testCommand(r.ConfigPaths)))
		r.TestOK = err == nil
		r.TestOutput = strings.TrimSpace(out + errOut)
		return nil
	})
	g.Go(func() error {
		out, _, _ := c.Output(sshexec.AsRoot("docker logs --tail 400 " + sshexec.ShellQuote(ct.Name) + " 2>&1 | grep -iE '\\[(alert|warning|err)\\]' | tail -n 15"))
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				r.RecentErrors = append(r.RecentErrors, l)
			}
		}
		return nil
	})
	g.Go(func() error {
		out, _, _ := c.Output(sshexec.AsRoot("docker stats --no-stream --format '{{json .}}' " + sshexec.ShellQuote(ct.Name)))
		var st struct{ CPUPerc, MemUsage, MemPerc, NetIO, PIDs string }
		if json.Unmarshal([]byte(strings.TrimSpace(out)), &st) == nil {
			r.Stats = Stats{CPU: st.CPUPerc, Memory: st.MemUsage, MemShare: st.MemPerc, NetIO: st.NetIO, PIDs: st.PIDs}
		}
		return nil
	})
	g.Wait()

	var patterns []string
	for _, p := range r.ConfigPaths {
		patterns = append(patterns, p, strings.TrimRight(p, "/")+"/*.cfg")
	}
	files, err := dockerx.ReadFiles(c, ct, patterns)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(a, b int) bool { return files[a].Path < files[b].Path })
	if files == nil {
		files = []dockerx.File{}
	}
	r.Files = files

	sections := parseFiles(files)
	modes := effectiveModes(sections)
	r.Sections = make([]SectionView, 0, len(sections))
	for i, s := range sections {
		r.Sections = append(r.Sections, s.view(modes[i]))
	}
	r.Probe = DetermineProbe(sections)
	r.Healthy, r.HealthDetail = evaluateProbe(c, ct, r.Probe)

	r.BackendServers = []BackendServer{}
	if port, uri, ok := statsEndpoint(sections); ok {
		r.HasStatsPage = true
		url := fmt.Sprintf("http://127.0.0.1:%d%s;csv", port, uri)
		out, _, err := c.Output(dockerx.Exec(ct.Name, false, `wget -q -T 3 -O - "$1"`, url))
		if err == nil {
			if servers := parseStatsCSV(out); servers != nil {
				r.BackendServers = servers
			}
		}
	}
	return r, nil
}

// Change is a new version of one config file, or its removal.
type Change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	BaseSHA string `json:"baseSha"` // "" for a new file
	Delete  bool   `json:"delete"`
}

// b64 encodes content for safe embedding in a shell command (its alphabet
// has no shell metacharacters, so the far side just needs "base64 -d").
func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// validateScratchRoot is where Validate stages a scratch copy of the
// configured paths to dry-run changes against, entirely inside /tmp — well
// outside any real config path or include glob, so it can never be picked
// up by a real reload, and removed again once the check is done.
const validateScratchRoot = "/tmp/panel-haproxy-check"

// Validate dry-runs changes with "haproxy -c" against a scratch copy of
// every "-f" path the container actually runs with: it copies each real
// config path (file or directory) into a scratch root inside the
// container, overlays the changes there, and tests that — never touching
// a real file. ok is false exactly when haproxy -c rejects the result;
// output is its combined stdout/stderr either way.
func Validate(c *sshexec.Client, ct *dockerx.Container, changes []Change) (ok bool, output string, err error) {
	realPaths := configPaths(ct.Args)
	s := validateScratchRoot

	var script strings.Builder
	script.WriteString(`rm -rf "` + s + `"; mkdir -p "` + s + `"; `)
	var args []string
	for _, p := range realPaths {
		script.WriteString(`D="$1"; mkdir -p "` + s + `$(dirname "$D")"; [ -e "$D" ] && cp -r "$D" "` + s + `$D" 2>/dev/null; shift; `)
		args = append(args, p)
	}
	for _, ch := range changes {
		if ch.Delete {
			script.WriteString(`rm -f "` + s + `$1"; shift; `)
			args = append(args, ch.Path)
		} else {
			script.WriteString(`mkdir -p "` + s + `$(dirname "$1")"; echo "$2" | base64 -d > "` + s + `$1"; shift 2; `)
			args = append(args, ch.Path, b64(ch.Content))
		}
	}
	script.WriteString("haproxy -c")
	for _, p := range realPaths {
		script.WriteString(" -f " + sshexec.ShellQuote(s+p))
	}
	script.WriteString(`; RC=$?; rm -rf "` + s + `"; exit $RC`)

	out, errOut, runErr := c.Output(dockerx.Exec(ct.Name, true, script.String(), args...))
	output = strings.TrimSpace(out + errOut)
	if runErr != nil {
		return false, output, nil
	}
	return true, output, nil
}

// Apply dry-runs changes with Validate before touching anything real, so a
// syntax error never reaches disk, let alone gets reloaded. Only once that
// passes does it write the changes for real and reload gracefully by
// sending SIGUSR2 to the master process (PID 1 in the official image —
// verified against a real container: this triggers the same graceful
// reload "docker compose restart" never does, spawning a new worker with
// "-sf <old-pid>" while the old one drains). The reload itself is
// asynchronous — the new worker takes a few seconds to fully take over —
// so, like nginx's own Apply, this reports success once the signal was
// sent, rather than polling for the new config to be fully live.
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

	fmt.Fprintln(log, "Dry-running the configuration against a scratch copy (haproxy -c)...")
	ok, testOut, err := Validate(c, ct, changes)
	if err != nil {
		return fmt.Errorf("checking the configuration: %v", err)
	}
	fmt.Fprintln(log, indent(testOut))
	if !ok {
		fmt.Fprintln(log, "The test failed. Nothing was written; HAProxy keeps running its current configuration.")
		return fmt.Errorf("haproxy -c rejected the change, so nothing was applied: %s", lastLine(testOut))
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

	fmt.Fprintln(log, "Reloading HAProxy gracefully (SIGUSR2 to the master process)...")
	if _, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(ct.Name) + " kill -SIGUSR2 1")); err != nil {
		fmt.Fprintf(log, "Reload failed: %v %s\nPutting the previous files back...\n", err, strings.TrimSpace(errOut))
		restoreAll()
		c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(ct.Name) + " kill -SIGUSR2 1"))
		return fmt.Errorf("HAProxy did not reload; the previous files were restored")
	}
	fmt.Fprintln(log, "HAProxy reloaded with the new configuration (the new worker takes over within a few seconds; connections already being served are not dropped).")
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
		if _, errOut, err := c.Output(sshexec.AsRoot("rm -f " + sshexec.ShellQuote(host))); err != nil {
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

// lastLine picks the line of haproxy -c output that explains a failure: the
// [ALERT] line naming the file and problem (verified against a real
// container's output for a broken config), else the last line.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, want := range []string{"[ALERT]", "[WARNING]"} {
		for _, l := range lines {
			if strings.Contains(l, want) {
				return strings.TrimSpace(l)
			}
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}
