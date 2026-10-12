// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package promctl inspects and changes the configuration of a Prometheus
// server running in Docker on a remote host, over SSH. It reads what the
// running Prometheus actually loaded (through its own HTTP API, called from
// inside the container), and applies changes by validating them with
// promtool, backing up the old file, writing the new one and hot-reloading
// Prometheus with SIGHUP.
package promctl

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"path"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"devops-wrapper/internal/sshexec"
)

const defaultConfigPath = "/etc/prometheus/prometheus.yml"

// Instance is a running Prometheus container found on a server.
type Instance struct {
	Container   string
	Image       string
	ConfigPath  string // --config.file, as seen inside the container
	Port        string // from --web.listen-address
	RoutePrefix string // from --web.route-prefix or --web.external-url, e.g. "/prometheus"; "" for none
	WebConfig   string // --web.config.file; when set, the API may require TLS or a password
	IP          string // the container's address on its Docker network
	Mounts      []Mount
	Labels      map[string]string
	State       string // "running", "exited", ...
}

// Mount is one bind mount of the container.
type Mount struct {
	Source      string // path on the server
	Destination string // path inside the container
}

// HostPath maps a path inside the container to the server path it's
// bind-mounted from. ok is false when the path isn't on a bind mount, which
// means it only exists inside the container and is lost when the container
// is recreated.
func (i *Instance) HostPath(containerPath string) (hostPath string, ok bool) {
	best := -1
	for _, m := range i.Mounts {
		d := strings.TrimRight(m.Destination, "/")
		if containerPath == d || strings.HasPrefix(containerPath, d+"/") {
			if len(d) > best {
				best = len(d)
				hostPath = strings.TrimRight(m.Source, "/") + containerPath[len(d):]
				ok = true
			}
		}
	}
	return hostPath, ok
}

// ConfigDir is the folder holding the main config file inside the
// container. Relative rule_files paths resolve against it.
func (i *Instance) ConfigDir() string { return path.Dir(i.ConfigPath) }

type composePsEntry struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	Image   string `json:"Image"`
	State   string `json:"State"`
}

// Discover finds the Prometheus container of the compose project in
// deployDir and reads how it was started.
func Discover(c *sshexec.Client, deployDir string) (*Instance, error) {
	out, errOut, err := c.Output(sshexec.AsRootPrelude + sshexec.ComposeIn(deployDir) + " ps --all --format json")
	if err != nil {
		return nil, fmt.Errorf("listing containers in %s failed: %v %s", deployDir, err, strings.TrimSpace(errOut))
	}
	entries := parseComposePs(out)

	var found *composePsEntry
	for i := range entries {
		e := &entries[i]
		if isPrometheusImage(e.Image) || (found == nil && e.Service == "prometheus") {
			found = e
			if isPrometheusImage(e.Image) {
				break
			}
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no Prometheus container found in %s", deployDir)
	}
	inst, err := DiscoverContainer(c, found.Name)
	if err != nil {
		return nil, err
	}
	if inst.State != "running" {
		return nil, fmt.Errorf("the Prometheus container %s is %s; start the service first", inst.Container, inst.State)
	}
	return inst, nil
}

// DiscoverContainer reads how a named Prometheus container was started:
// its config path, listen port, URL prefix and bind mounts.
func DiscoverContainer(c *sshexec.Client, name string) (*Instance, error) {
	out, errOut, err := c.Output(sshexec.AsRoot("docker inspect --format '{{json .}}' " + sshexec.ShellQuote(name)))
	if err != nil {
		return nil, fmt.Errorf("container %s not found on the server: %v %s", name, err, strings.TrimSpace(errOut))
	}
	var inspect struct {
		Name   string   `json:"Name"`
		Args   []string `json:"Args"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &inspect); err != nil {
		return nil, fmt.Errorf("reading container details: %v", err)
	}

	inst := &Instance{
		Container:   strings.TrimPrefix(inspect.Name, "/"),
		Image:       inspect.Config.Image,
		ConfigPath:  flagValue(inspect.Args, "config.file", defaultConfigPath),
		Port:        listenPort(flagValue(inspect.Args, "web.listen-address", ":9090")),
		RoutePrefix: routePrefix(inspect.Args),
		WebConfig:   flagValue(inspect.Args, "web.config.file", ""),
		Labels:      inspect.Config.Labels,
		State:       inspect.State.Status,
	}
	if inst.Container == "" {
		inst.Container = name
	}
	for _, n := range inspect.NetworkSettings.Networks {
		if n.IPAddress != "" {
			inst.IP = n.IPAddress
			break
		}
	}
	for _, m := range inspect.Mounts {
		if m.Type == "bind" {
			inst.Mounts = append(inst.Mounts, Mount{Source: m.Source, Destination: m.Destination})
		}
	}
	return inst, nil
}

// routePrefix is the path Prometheus serves under: --web.route-prefix, or
// else the path of --web.external-url (Prometheus's own default).
func routePrefix(args []string) string {
	p := flagValue(args, "web.route-prefix", "")
	if p == "" {
		if u, err := neturl.Parse(flagValue(args, "web.external-url", "")); err == nil {
			p = u.Path
		}
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// Candidate is a Prometheus container found on a server, as offered when
// connecting an existing stack.
type Candidate struct {
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	State          string   `json:"state"`
	ComposeProject string   `json:"composeProject,omitempty"`
	ComposeService string   `json:"composeService,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	ComposeFiles   []string `json:"composeFiles,omitempty"`
	ConfigPath     string   `json:"configPath"`
	ConfigHostPath string   `json:"configHostPath,omitempty"`
	Port           string   `json:"port"`
	RoutePrefix    string   `json:"routePrefix,omitempty"`
	WebConfig      bool     `json:"webConfig"`
}

// FindAll lists every Prometheus container on the server, running or not.
func FindAll(c *sshexec.Client) ([]Candidate, error) {
	out, errOut, err := c.Output(sshexec.AsRoot("docker ps -a --format '{{.Names}}\t{{.Image}}'"))
	if err != nil {
		return nil, fmt.Errorf("listing containers failed (the SSH user needs access to Docker, directly or via sudo): %v %s", err, strings.TrimSpace(errOut))
	}
	candidates := []Candidate{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, image, ok := strings.Cut(line, "\t")
		if !ok || !isPrometheusImage(image) {
			continue
		}
		inst, err := DiscoverContainer(c, name)
		if err != nil {
			continue
		}
		cand := Candidate{
			Container:      inst.Container,
			Image:          inst.Image,
			State:          inst.State,
			ComposeProject: inst.Labels["com.docker.compose.project"],
			ComposeService: inst.Labels["com.docker.compose.service"],
			WorkingDir:     inst.Labels["com.docker.compose.project.working_dir"],
			ConfigPath:     inst.ConfigPath,
			Port:           inst.Port,
			RoutePrefix:    inst.RoutePrefix,
			WebConfig:      inst.WebConfig != "",
		}
		if files := inst.Labels["com.docker.compose.project.config_files"]; files != "" {
			cand.ComposeFiles = strings.Split(files, ",")
		}
		cand.ConfigHostPath, _ = inst.HostPath(inst.ConfigPath)
		candidates = append(candidates, cand)
	}
	return candidates, nil
}

// ReadHostFiles reads files on the server itself (not in a container),
// such as a stack's compose files, returning them joined with a header
// line per file. Files the SSH user can't read are read through sudo -n.
func ReadHostFiles(c *sshexec.Client, paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		cmd := asRoot + "$AS_ROOT cat " + sshexec.ShellQuote(p)
		out, _, err := c.Output(cmd)
		if err != nil {
			continue
		}
		if len(paths) > 1 {
			fmt.Fprintf(&b, "# ---- %s ----\n", p)
		}
		b.WriteString(out)
		if !strings.HasSuffix(out, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func isPrometheusImage(image string) bool {
	name := image
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name, _, _ = strings.Cut(name, ":")
	name, _, _ = strings.Cut(name, "@")
	return name == "prometheus"
}

// parseComposePs accepts both output shapes of "docker compose ps --format
// json": one JSON object per line (newer Compose) or a single JSON array.
func parseComposePs(out string) []composePsEntry {
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, "[") {
		var all []composePsEntry
		if json.Unmarshal([]byte(trimmed), &all) == nil {
			return all
		}
	}
	var entries []composePsEntry
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var e composePsEntry
		if json.Unmarshal([]byte(strings.TrimSpace(scanner.Text())), &e) == nil && e.Name != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// flagValue reads a Prometheus flag from the container's arguments, in
// either "--name=value" or "--name value" form.
func flagValue(args []string, name, fallback string) string {
	for i, a := range args {
		a = strings.TrimLeft(a, "-")
		if v, ok := strings.CutPrefix(a, name+"="); ok {
			return v
		}
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return fallback
}

func listenPort(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 && i+1 < len(addr) {
		return addr[i+1:]
	}
	return "9090"
}

// api calls Prometheus's HTTP API from inside its own container (busybox
// wget ships in the official image), so it works whether or not the port
// is published on the host.
func api(c *sshexec.Client, inst *Instance, endpoint string, into any) error {
	url := fmt.Sprintf("http://127.0.0.1:%s%s/api/v1/%s", inst.Port, inst.RoutePrefix, endpoint)
	out, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " wget -qO- " + sshexec.ShellQuote(url)))
	if err != nil {
		if inst.WebConfig != "" {
			return fmt.Errorf("Prometheus API %s: this Prometheus uses a web config file (%s) for TLS or a password, which the panel can't use yet: %s",
				endpoint, inst.WebConfig, strings.TrimSpace(errOut))
		}
		return fmt.Errorf("Prometheus API %s: %v %s", endpoint, err, strings.TrimSpace(errOut))
	}
	var envelope struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
		Error  string          `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		return fmt.Errorf("Prometheus API %s: unreadable response", endpoint)
	}
	if envelope.Status != "success" {
		return fmt.Errorf("Prometheus API %s: %s", endpoint, envelope.Error)
	}
	return json.Unmarshal(envelope.Data, into)
}

// File is a config or rule file, as read from inside the container.
type File struct {
	Path     string `json:"path"`               // inside the container
	HostPath string `json:"hostPath,omitempty"` // on the server; empty when it only exists in the container
	Content  string `json:"content"`
	SHA      string `json:"sha"`
}

func sha(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// readFiles reads every file matching patterns inside the container.
// Patterns may be globs, as rule_files entries often are. Contents travel
// base64-encoded so no file content can break the output framing.
func readFiles(c *sshexec.Client, inst *Instance, patterns []string) ([]File, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	script := `for P in "$@"; do for f in $P; do [ -f "$f" ] && printf '%s %s\n' "$f" "$(base64 < "$f" | tr -d '\n')"; done; done; true`
	cmd := sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " sh -c " + sshexec.ShellQuote(script) + " sh")
	for _, p := range patterns {
		cmd += " " + sshexec.ShellQuote(p)
	}
	out, errOut, err := c.Output(cmd)
	if err != nil {
		return nil, fmt.Errorf("reading files: %v %s", err, strings.TrimSpace(errOut))
	}

	seen := map[string]bool{}
	var files []File
	for _, line := range strings.Split(out, "\n") {
		// An empty file prints as just its path, with nothing after it.
		p, encoded, _ := strings.Cut(strings.TrimSpace(line), " ")
		if p == "" || seen[p] {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		seen[p] = true
		f := File{Path: p, Content: string(raw), SHA: sha(string(raw))}
		f.HostPath, _ = inst.HostPath(p)
		files = append(files, f)
	}
	return files, nil
}

// ruleFilePatterns reads rule_files from a config and resolves relative
// entries against the config's folder, as Prometheus does.
func ruleFilePatterns(config, configDir string) []string {
	var parsed struct {
		RuleFiles []string `yaml:"rule_files"`
	}
	if yaml.Unmarshal([]byte(config), &parsed) != nil {
		return nil
	}
	var out []string
	for _, p := range parsed.RuleFiles {
		if !path.IsAbs(p) {
			p = path.Join(configDir, p)
		}
		out = append(out, p)
	}
	return out
}

// Report is everything the panel shows about a Prometheus server.
type Report struct {
	Container     string      `json:"container"`
	Image         string      `json:"image"`
	Version       string      `json:"version"`
	ConfigPath    string      `json:"configPath"`
	OnDisk        bool        `json:"onDisk"` // config file is bind-mounted from the server, so edits survive a redeploy
	ReloadOK      bool        `json:"reloadOk"`
	LastReload    string      `json:"lastReload"`
	Config        File        `json:"config"`
	RuleFiles     []File      `json:"ruleFiles"`
	RulePatterns  []string    `json:"rulePatterns"`
	Jobs          []Job       `json:"jobs"`
	RuleGroups    []RuleGroup `json:"ruleGroups"`
	TargetsUp     int         `json:"targetsUp"`
	TargetsDown   int         `json:"targetsDown"`
	AlertsFiring  int         `json:"alertsFiring"`
	AlertsPending int         `json:"alertsPending"`
}

// Job is one scrape job and the live health of its targets.
type Job struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"` // "exporter" or "blackbox"
	Module   string   `json:"module,omitempty"`
	Exporter string   `json:"exporter,omitempty"`
	Targets  []Target `json:"targets"`
}

// Target is one scraped endpoint.
type Target struct {
	Instance   string `json:"instance"`
	Note       string `json:"note,omitempty"` // the comment next to it in the config, if any
	ScrapeURL  string `json:"scrapeUrl"`
	Health     string `json:"health"` // "up", "down" or "unknown"
	LastError  string `json:"lastError,omitempty"`
	LastScrape string `json:"lastScrape,omitempty"`
}

// RuleGroup is one group of rules as Prometheus loaded it.
type RuleGroup struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// Rule is one alerting or recording rule and its current state.
type Rule struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "alerting" or "recording"
	Query     string `json:"query"`
	Duration  int    `json:"duration"` // "for", in seconds
	Health    string `json:"health"`
	State     string `json:"state,omitempty"` // "firing", "pending" or "inactive"
	LastError string `json:"lastError,omitempty"`
	Active    int    `json:"active"` // alerts currently pending or firing
}

// Inspect reads the running Prometheus: its version, the files it loaded,
// and the live state of every target and rule.
func Inspect(c *sshexec.Client, inst *Instance) (*Report, error) {
	r := &Report{Container: inst.Container, Image: inst.Image, ConfigPath: inst.ConfigPath}

	var build struct {
		Version string `json:"version"`
	}
	var runtime struct {
		ReloadConfigSuccess bool   `json:"reloadConfigSuccess"`
		LastConfigTime      string `json:"lastConfigTime"`
	}
	var targets struct {
		Active []struct {
			Discovered map[string]string `json:"discoveredLabels"`
			Labels     map[string]string `json:"labels"`
			ScrapePool string            `json:"scrapePool"`
			ScrapeURL  string            `json:"scrapeUrl"`
			LastError  string            `json:"lastError"`
			LastScrape string            `json:"lastScrape"`
			Health     string            `json:"health"`
		} `json:"activeTargets"`
	}
	var rules struct {
		Groups []struct {
			Name  string `json:"name"`
			File  string `json:"file"`
			Rules []struct {
				Name      string        `json:"name"`
				Type      string        `json:"type"`
				Query     string        `json:"query"`
				Duration  float64       `json:"duration"`
				Health    string        `json:"health"`
				State     string        `json:"state"`
				LastError string        `json:"lastError"`
				Alerts    []interface{} `json:"alerts"`
			} `json:"rules"`
		} `json:"groups"`
	}
	var configs []File

	// Each of these is one SSH round trip; run them side by side on the
	// same connection rather than one after another.
	var g errgroup.Group
	g.SetLimit(4) // SSH servers allow 10 sessions per connection by default; leave room
	g.Go(func() error { return api(c, inst, "status/buildinfo", &build) })
	g.Go(func() error { return api(c, inst, "status/runtimeinfo", &runtime) })
	g.Go(func() error { return api(c, inst, "targets?state=active", &targets) })
	g.Go(func() error { return api(c, inst, "rules", &rules) })
	g.Go(func() (err error) { configs, err = readFiles(c, inst, []string{inst.ConfigPath}); return err })
	if err := g.Wait(); err != nil {
		return nil, err
	}
	r.Version = build.Version
	r.ReloadOK = runtime.ReloadConfigSuccess
	r.LastReload = runtime.LastConfigTime
	if len(configs) == 0 {
		return nil, fmt.Errorf("config file %s not found in the container", inst.ConfigPath)
	}
	r.Config = configs[0]
	_, r.OnDisk = inst.HostPath(inst.ConfigPath)

	r.RulePatterns = ruleFilePatterns(r.Config.Content, inst.ConfigDir())
	var err error
	if r.RuleFiles, err = readFiles(c, inst, r.RulePatterns); err != nil {
		return nil, err
	}
	if r.RuleFiles == nil {
		r.RuleFiles = []File{}
	}

	// Start from the jobs in the config, so a job with no live targets
	// (all commented out, say) still shows up.
	jobs := map[string]*Job{}
	var order []string
	for _, j := range ConfigJobs(r.Config.Content) {
		jobs[j.Name] = &Job{Name: j.Name, Kind: j.Kind, Module: j.Module, Exporter: j.Exporter, Targets: []Target{}}
		order = append(order, j.Name)
	}
	notes := TargetNotes(r.Config.Content)
	for _, t := range targets.Active {
		j, ok := jobs[t.ScrapePool]
		if !ok {
			j = &Job{Name: t.ScrapePool, Kind: "exporter", Targets: []Target{}}
			jobs[t.ScrapePool] = j
			order = append(order, t.ScrapePool)
		}
		note := notes[t.ScrapePool][t.Discovered["__address__"]]
		if note == "" {
			note = notes[t.ScrapePool][t.Labels["instance"]]
		}
		j.Targets = append(j.Targets, Target{
			Note:       note,
			Instance:   t.Labels["instance"],
			ScrapeURL:  t.ScrapeURL,
			Health:     t.Health,
			LastError:  t.LastError,
			LastScrape: t.LastScrape,
		})
		if t.Health == "up" {
			r.TargetsUp++
		} else {
			r.TargetsDown++
		}
	}
	for _, name := range order {
		j := jobs[name]
		sort.SliceStable(j.Targets, func(a, b int) bool {
			// Down targets first: they're what needs attention.
			return j.Targets[a].Health != "up" && j.Targets[b].Health == "up"
		})
		r.Jobs = append(r.Jobs, *j)
	}
	if r.Jobs == nil {
		r.Jobs = []Job{}
	}

	r.RuleGroups = []RuleGroup{}
	for _, g := range rules.Groups {
		group := RuleGroup{File: g.File, Name: g.Name, Rules: []Rule{}}
		for _, rule := range g.Rules {
			group.Rules = append(group.Rules, Rule{
				Name: rule.Name, Type: rule.Type, Query: rule.Query, Duration: int(rule.Duration),
				Health: rule.Health, State: rule.State, LastError: rule.LastError, Active: len(rule.Alerts),
			})
			switch rule.State {
			case "firing":
				r.AlertsFiring++
			case "pending":
				r.AlertsPending++
			}
		}
		r.RuleGroups = append(r.RuleGroups, group)
	}
	return r, nil
}

// RulePatterns returns the config's rule_files entries as absolute paths
// (or globs) inside the container.
func RulePatterns(config, configDir string) []string { return ruleFilePatterns(config, configDir) }

// APIGet performs a read-only request against the Prometheus HTTP API and
// returns the status and raw body. It connects to the container directly
// through the SSH connection, so error responses (a PromQL syntax error is
// a 400 with a JSON body) come back intact. When that isn't possible, for
// example because the SSH server forbids forwarding, it falls back to wget
// inside the container, which can only report success or failure.
func APIGet(ctx context.Context, c *sshexec.Client, inst *Instance, pathAndQuery string) (int, []byte, error) {
	if inst.IP != "" {
		status, body, err := c.HTTPGet(ctx, net.JoinHostPort(inst.IP, inst.Port), inst.RoutePrefix+pathAndQuery)
		if err == nil {
			return status, body, nil
		}
	}
	url := fmt.Sprintf("http://127.0.0.1:%s%s%s", inst.Port, inst.RoutePrefix, pathAndQuery)
	out, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " wget -qO- " + sshexec.ShellQuote(url)))
	if err != nil {
		// wget drops the body of an error response, so for a query ask
		// promtool, which prints Prometheus's own explanation.
		if q, ok := queryParam(pathAndQuery); ok {
			server := fmt.Sprintf("http://127.0.0.1:%s%s", inst.Port, inst.RoutePrefix)
			_, pErr, qErr := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " promtool query instant " +
				sshexec.ShellQuote(server) + " " + sshexec.ShellQuote(q)))
			if msg := strings.TrimSpace(pErr); qErr != nil && msg != "" {
				body, _ := json.Marshal(map[string]string{"status": "error", "errorType": "bad_data", "error": cleanPromtoolError(msg)})
				return http.StatusBadRequest, body, nil
			}
		}
		msg := strings.TrimSpace(errOut)
		if msg == "" {
			msg = err.Error()
		}
		body, _ := json.Marshal(map[string]string{"status": "error", "errorType": "unavailable", "error": msg})
		return http.StatusBadGateway, body, nil
	}
	return http.StatusOK, []byte(out), nil
}

// queryParam returns the query of an /api/v1/query request.
func queryParam(pathAndQuery string) (string, bool) {
	p, rawQuery, _ := strings.Cut(pathAndQuery, "?")
	if p != "/api/v1/query" {
		return "", false
	}
	values, err := neturl.ParseQuery(rawQuery)
	if err != nil || values.Get("query") == "" {
		return "", false
	}
	return values.Get("query"), true
}

// cleanPromtoolError trims promtool's wrapping ("query error: bad_data:")
// down to Prometheus's own message.
func cleanPromtoolError(msg string) string {
	msg = strings.TrimSpace(strings.Split(msg, "\n")[0])
	for _, prefix := range []string{"query error:", "bad_data:", " "} {
		msg = strings.TrimSpace(strings.TrimPrefix(msg, prefix))
	}
	return msg
}

// Ready reports whether Prometheus answers its readiness check and is
// running the config it was last given. why explains a false result.
func Ready(c *sshexec.Client, inst *Instance) (ok bool, why string) {
	url := fmt.Sprintf("http://127.0.0.1:%s%s/-/ready", inst.Port, inst.RoutePrefix)
	if _, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " wget -qO- " + sshexec.ShellQuote(url))); err != nil {
		msg := strings.TrimSpace(errOut)
		if msg == "" {
			msg = err.Error()
		}
		return false, msg
	}
	var runtime struct {
		ReloadConfigSuccess bool `json:"reloadConfigSuccess"`
	}
	if err := api(c, inst, "status/runtimeinfo", &runtime); err != nil {
		return false, err.Error()
	}
	if !runtime.ReloadConfigSuccess {
		return false, "its last config reload failed"
	}
	return true, ""
}
