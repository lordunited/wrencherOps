// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package envoyctl finds Envoy containers on a remote server and manages
// their bootstrap configuration: it reads the config and Envoy's live state
// (its admin API: readiness, listeners, clusters and per-endpoint health),
// dry-runs a changed config with "envoy --mode validate", and applies it.
//
// Envoy has no reload signal for a static config, so applying restarts the
// container — after the change has passed validation, and with the previous
// file put back (and Envoy restarted again) if it doesn't come up ready.
// Everything here was checked against a real envoyproxy/envoy:v1.39 image.
package envoyctl

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

// DefaultConfigPath is the official image's own bootstrap path (its CMD is
// "envoy -c /etc/envoy/envoy.yaml").
const DefaultConfigPath = "/etc/envoy/envoy.yaml"

// IsEnvoyImage reports whether an image is an Envoy build: the official
// envoyproxy/envoy (and its -distroless/-contrib variants) or any image
// named envoy.
func IsEnvoyImage(image string) bool {
	base := dockerx.ImageBase(image)
	return base == "envoy" || strings.HasPrefix(base, "envoy-") || strings.Contains(image, "envoyproxy/envoy")
}

// Running inspects a container and requires it to be running.
func Running(c *sshexec.Client, name string) (*dockerx.Container, error) {
	ct, err := dockerx.Inspect(c, name)
	if err != nil {
		return nil, err
	}
	if ct.State != "running" {
		return nil, fmt.Errorf("the Envoy container %s is %s; start it first", ct.Name, ct.State)
	}
	return ct, nil
}

// Discover finds the Envoy container of the compose project in deployDir.
func Discover(c *sshexec.Client, deployDir string) (*dockerx.Container, error) {
	out, errOut, err := c.Output(sshexec.AsRootPrelude + sshexec.ComposeIn(deployDir) + " ps --all --format '{{.Name}}\t{{.Image}}'")
	if err != nil {
		return nil, fmt.Errorf("listing containers in %s failed: %v %s", deployDir, err, strings.TrimSpace(errOut))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, image, ok := strings.Cut(line, "\t")
		if ok && IsEnvoyImage(image) {
			return Running(c, name)
		}
	}
	return nil, fmt.Errorf("no Envoy container found in %s", deployDir)
}

// Candidate is an Envoy container found on a server, offered for connecting.
type Candidate struct {
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	State          string   `json:"state"`
	ComposeProject string   `json:"composeProject,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	ComposeFiles   []string `json:"composeFiles,omitempty"`
	ConfigPath     string   `json:"configPath"`
}

// FindAll lists every Envoy container on the server.
func FindAll(c *sshexec.Client) ([]Candidate, error) {
	cts, err := dockerx.FindByImage(c, IsEnvoyImage)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, ct := range cts {
		project, dir, files := ct.Compose()
		out = append(out, Candidate{Container: ct.Name, Image: ct.Image, State: ct.State, ComposeProject: project, WorkingDir: dir, ComposeFiles: files, ConfigPath: ConfigPath(ct.Args)})
	}
	return out, nil
}

// ConfigPath is the bootstrap file Envoy was started with: its "-c" or
// "--config-path" argument, else the image's default.
func ConfigPath(args []string) string {
	for i, a := range args {
		for _, flag := range []string{"-c", "--config-path"} {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
			if v, ok := strings.CutPrefix(a, flag+"="); ok {
				return v
			}
		}
	}
	return DefaultConfigPath
}

// --- the admin API

// adminScript makes an HTTP/1.1 request with bash's /dev/tcp (the official
// image has bash but no curl or wget). The admin API answers HTTP/1.0 with
// "426 Upgrade Required", and sends its body chunked.
const adminScript = `exec 3<>"/dev/tcp/$1/$2" || exit 7
printf 'GET %s HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n' "$3" >&3
cat <&3`

// AdminGet makes a GET request to Envoy's admin API at addr (host:port, as
// the admin block of its config has it). It's tried from inside the
// container first, which reaches an admin bound to 127.0.0.1; an image
// without bash (distroless) is reached instead through the SSH connection,
// at the container's own address or, with host networking, the server's.
func AdminGet(c *sshexec.Client, ct *dockerx.Container, addr, pathAndQuery string) (status int, body []byte, err error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, nil, err
	}
	inner := host
	if inner == "" || inner == "0.0.0.0" || inner == "::" {
		inner = "127.0.0.1"
	}
	raw, _, runErr := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(ct.Name) + " bash -c " + sshexec.ShellQuote(adminScript) + " bash " +
		sshexec.ShellQuote(inner) + " " + sshexec.ShellQuote(port) + " " + sshexec.ShellQuote(pathAndQuery)))
	if runErr == nil && strings.HasPrefix(raw, "HTTP/") {
		resp, err := http.ReadResponse(bufio.NewReader(strings.NewReader(raw)), nil)
		if err == nil {
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			return resp.StatusCode, b, err
		}
	}
	// No bash in the image, or the admin isn't on the container's loopback.
	targets := []string{}
	if ct.IP != "" && inner != "127.0.0.1" && inner != "::1" {
		targets = append(targets, net.JoinHostPort(ct.IP, port))
	}
	targets = append(targets, net.JoinHostPort(inner, port))
	var lastErr error = fmt.Errorf("the admin API at %s didn't answer", addr)
	for _, t := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		code, b, err := c.HTTPGet(ctx, t, pathAndQuery)
		cancel()
		if err == nil {
			return code, b, nil
		}
		lastErr = err
	}
	return 0, nil, lastErr
}

// Endpoint is one upstream host of a cluster, as Envoy sees it right now.
type Endpoint struct {
	Address string `json:"address"`
	Healthy bool   `json:"healthy"`
	// Status says why not, in words: "failed active health check",
	// "waiting for its first health check", "draining", ...
	Status   string `json:"status"`
	CxActive int64  `json:"cxActive"`
	RqTotal  int64  `json:"rqTotal"`
	RqError  int64  `json:"rqError"`
}

// LiveCluster is a cluster Envoy is running, with its endpoints' health.
type LiveCluster struct {
	Name string `json:"name"`
	// Dynamic marks a cluster from a control plane (xDS), not the file.
	Dynamic   bool       `json:"dynamic"`
	Endpoints []Endpoint `json:"endpoints"`
}

type clustersJSON struct {
	ClusterStatuses []struct {
		Name         string `json:"name"`
		AddedViaAPI  bool   `json:"added_via_api"`
		HostStatuses []struct {
			Address struct {
				SocketAddress struct {
					Address   string `json:"address"`
					PortValue int    `json:"port_value"`
				} `json:"socket_address"`
				Pipe struct {
					Path string `json:"path"`
				} `json:"pipe"`
			} `json:"address"`
			HealthStatus map[string]any `json:"health_status"`
			Stats        []struct {
				Name  string          `json:"name"`
				Value json.RawMessage `json:"value"`
			} `json:"stats"`
		} `json:"host_statuses"`
	} `json:"cluster_statuses"`
}

// endpointHealth reads a host's health_status: healthy unless a check
// failed or EDS marks it otherwise. Verified field names from a real
// /clusters?format=json.
func endpointHealth(hs map[string]any) (bool, string) {
	flag := func(k string) bool { v, _ := hs[k].(bool); return v }
	switch {
	case flag("failed_active_health_check"):
		return false, "failed active health check"
	case flag("failed_outlier_check"):
		return false, "ejected by outlier detection"
	case flag("active_hc_timeout"):
		return false, "health check timed out"
	case flag("pending_active_hc"):
		return false, "waiting for its first health check"
	case flag("pending_dynamic_removal"):
		return false, "being removed"
	case flag("excluded_via_immediate_hc_fail"):
		return false, "excluded (immediate health check fail)"
	}
	switch eds, _ := hs["eds_health_status"].(string); eds {
	case "", "HEALTHY", "UNKNOWN":
		if flag("failed_active_degraded_check") {
			return true, "degraded"
		}
		return true, "healthy"
	default:
		return false, strings.ToLower(eds)
	}
}

func parseClusters(b []byte) ([]LiveCluster, error) {
	var raw clustersJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := []LiveCluster{}
	for _, c := range raw.ClusterStatuses {
		lc := LiveCluster{Name: c.Name, Dynamic: c.AddedViaAPI, Endpoints: []Endpoint{}}
		for _, h := range c.HostStatuses {
			addr := h.Address.Pipe.Path
			if h.Address.SocketAddress.Address != "" {
				addr = net.JoinHostPort(h.Address.SocketAddress.Address, strconv.Itoa(h.Address.SocketAddress.PortValue))
			}
			e := Endpoint{Address: addr}
			e.Healthy, e.Status = endpointHealth(h.HealthStatus)
			for _, s := range h.Stats {
				var n int64
				var str string
				if json.Unmarshal(s.Value, &n) != nil && json.Unmarshal(s.Value, &str) == nil {
					n, _ = strconv.ParseInt(str, 10, 64)
				}
				switch s.Name {
				case "cx_active":
					e.CxActive = n
				case "rq_total":
					e.RqTotal = n
				case "rq_error":
					e.RqError = n
				}
			}
			lc.Endpoints = append(lc.Endpoints, e)
		}
		out = append(out, lc)
	}
	return out, nil
}

// versionOf turns /server_info's "<commit>/1.39.0/Clean/RELEASE/BoringSSL"
// into "1.39.0 (RELEASE, BoringSSL)".
func versionOf(v string) string {
	p := strings.Split(v, "/")
	if len(p) >= 5 {
		return p[1] + " (" + p[3] + ", " + p[4] + ")"
	}
	return v
}

type serverInfo struct {
	Version            string `json:"version"`
	State              string `json:"state"`
	UptimeCurrentEpoch string `json:"uptime_current_epoch"`
}

// --- inspect

// Stats is the container's resource use right now.
type Stats struct {
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	MemShare string `json:"memShare"`
	NetIO    string `json:"netIO"`
	PIDs     string `json:"pids"`
}

// Report is everything the panel shows about an Envoy.
type Report struct {
	Container  string         `json:"container"`
	Image      string         `json:"image"`
	Version    string         `json:"version"`
	State      string         `json:"state"`  // LIVE, DRAINING, ... from /server_info
	Uptime     string         `json:"uptime"` // e.g. "3600s"
	ConfigPath string         `json:"configPath"`
	Files      []dockerx.File `json:"files"`
	// TestOK/TestOutput are "envoy --mode validate" on the config as it is.
	TestOK     bool   `json:"testOk"`
	TestOutput string `json:"testOutput"`
	Config     Config `json:"config"`
	// Admin is the admin API's address from the config, "" when it has none.
	Admin          string        `json:"admin"`
	AdminReachable bool          `json:"adminReachable"`
	AdminError     string        `json:"adminError,omitempty"`
	LiveClusters   []LiveCluster `json:"liveClusters"`
	Healthy        bool          `json:"healthy"`
	HealthDetail   string        `json:"healthDetail"`
	RecentErrors   []string      `json:"recentErrors"`
	Stats          Stats         `json:"stats"`
}

// Inspect reads the Envoy's bootstrap config and live state.
func Inspect(c *sshexec.Client, ct *dockerx.Container) (*Report, error) {
	r := &Report{Container: ct.Name, Image: ct.Image, ConfigPath: ConfigPath(ct.Args), RecentErrors: []string{}, LiveClusters: []LiveCluster{}}
	files, err := dockerx.ReadFiles(c, ct, []string{r.ConfigPath})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		// A distroless image has no shell to read with; read it on the
		// server when it's bind-mounted.
		if host, ok := ct.HostPath(r.ConfigPath); ok {
			out, _, err := c.Output(sshexec.AsRoot("cat " + sshexec.ShellQuote(host)))
			if err == nil {
				files = []dockerx.File{{Path: r.ConfigPath, HostPath: host, Content: out, SHA: dockerx.SHA(out)}}
			}
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("couldn't read Envoy's config %s", r.ConfigPath)
	}
	r.Files = files
	r.Config = ParseConfig(files[0].Content)
	r.Admin = r.Config.Admin

	var g errgroup.Group
	g.SetLimit(4)
	g.Go(func() error {
		r.TestOK, r.TestOutput, _ = Validate(c, ct, files[0].Content)
		return nil
	})
	g.Go(func() error {
		out, _, _ := c.Output(sshexec.AsRoot("docker logs --tail 400 " + sshexec.ShellQuote(ct.Name) + " 2>&1 | grep -E '\\]\\[(warning|error|critical)\\]' | tail -n 15"))
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
	g.Go(func() error {
		if r.Admin == "" {
			return nil
		}
		code, b, err := AdminGet(c, ct, r.Admin, "/server_info")
		if err != nil || code != http.StatusOK {
			r.AdminError = fmt.Sprintf("the admin API at %s didn't answer: %v", r.Admin, errOrStatus(err, code))
			return nil
		}
		r.AdminReachable = true
		var info serverInfo
		if json.Unmarshal(b, &info) == nil {
			r.Version, r.State, r.Uptime = versionOf(info.Version), info.State, info.UptimeCurrentEpoch
		}
		if code, b, err := AdminGet(c, ct, r.Admin, "/clusters?format=json"); err == nil && code == http.StatusOK {
			if lc, err := parseClusters(b); err == nil {
				r.LiveClusters = lc
			}
		}
		return nil
	})
	g.Wait()
	r.Healthy, r.HealthDetail = summarize(r.AdminReachable, r.Admin, r.State, r.AdminError, r.LiveClusters)
	return r, nil
}

func errOrStatus(err error, code int) string {
	if err != nil {
		return err.Error()
	}
	return "HTTP " + strconv.Itoa(code)
}

// summarize turns Envoy's state and its endpoints' health into a verdict.
func summarize(reachable bool, admin, state, adminErr string, clusters []LiveCluster) (bool, string) {
	if admin == "" {
		return true, "running (its config has no admin API, so readiness and endpoint health can't be read)"
	}
	if !reachable {
		return false, adminErr
	}
	healthy, total := 0, 0
	var down []string
	for _, c := range clusters {
		for _, e := range c.Endpoints {
			total++
			if e.Healthy {
				healthy++
			} else {
				down = append(down, c.Name+" "+e.Address+" ("+e.Status+")")
			}
		}
	}
	detail := "Envoy " + strings.ToLower(state)
	if total > 0 {
		detail += fmt.Sprintf(", %d/%d endpoints healthy", healthy, total)
	}
	if state != "LIVE" {
		return false, detail
	}
	if len(down) > 0 {
		if len(down) > 3 {
			down = append(down[:3], fmt.Sprintf("+%d more", len(down)-3))
		}
		return false, detail + " — down: " + strings.Join(down, "; ")
	}
	return true, detail
}

// CheckHealth is Envoy's own verdict (/ready, server state) and its
// endpoints' health, from the config the container actually runs.
func CheckHealth(c *sshexec.Client, ct *dockerx.Container) (bool, string) {
	files, err := dockerx.ReadFiles(c, ct, []string{ConfigPath(ct.Args)})
	admin := ""
	if err == nil && len(files) > 0 {
		admin = ParseConfig(files[0].Content).Admin
	}
	if admin == "" {
		return summarize(false, "", "", "", nil)
	}
	code, b, err := AdminGet(c, ct, admin, "/server_info")
	if err != nil || code != http.StatusOK {
		return summarize(false, admin, "", fmt.Sprintf("the admin API at %s didn't answer: %v", admin, errOrStatus(err, code)), nil)
	}
	var info serverInfo
	json.Unmarshal(b, &info)
	var clusters []LiveCluster
	if code, b, err := AdminGet(c, ct, admin, "/clusters?format=json"); err == nil && code == http.StatusOK {
		clusters, _ = parseClusters(b)
	}
	return summarize(true, admin, info.State, "", clusters)
}

// --- validate and apply

// Validate dry-runs a bootstrap config with "envoy --mode validate" in a
// throwaway container of the very image the live one runs (by ID, so a
// re-pulled tag can't make it check against another version), with its volumes
// (so certificates and other files it references are there) and no
// network. The running Envoy is never touched. Verified: a good config
// prints "configuration '…' OK"; a bad one exits non-zero naming the
// problem, e.g. "route: unknown cluster 'nope'".
func Validate(c *sshexec.Client, ct *dockerx.Container, content string) (ok bool, output string, err error) {
	script := sshexec.AsRootPrelude + fmt.Sprintf(`D=$($AS_ROOT mktemp -d /tmp/panel-envoy-check.XXXXXX) || exit 9
$AS_ROOT chmod 755 "$D"
printf '%%s' %s | base64 -d | $AS_ROOT tee "$D/envoy.yaml" > /dev/null
$AS_ROOT chmod 644 "$D/envoy.yaml"
IMG=$($AS_ROOT docker inspect --format '{{.Image}}' %s) || exit 9
$AS_ROOT docker run --rm --network none --volumes-from %s -v "$D:/panel-check:ro" --entrypoint envoy "$IMG" --mode validate -c /panel-check/envoy.yaml 2>&1
RC=$?
$AS_ROOT rm -rf "$D"
exit $RC`, sshexec.ShellQuote(b64(content)), sshexec.ShellQuote(ct.Name), sshexec.ShellQuote(ct.Name))
	out, errOut, runErr := c.Output(script)
	output = strings.TrimSpace(out + errOut)
	if runErr != nil {
		if strings.Contains(output, "Unable to find image") || strings.Contains(output, "No such container") {
			return false, output, fmt.Errorf("couldn't run the check: %s", lastLine(output))
		}
		return false, output, nil
	}
	return strings.Contains(output, "OK"), output, nil
}

// lastLine picks the line explaining a failed validation: the [critical]
// one Envoy prints, else the last line.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for _, l := range lines {
		if strings.Contains(l, "[critical]") || strings.Contains(l, "error initializing") {
			if i := strings.Index(l, "] "); i >= 0 && strings.Count(l, "][") > 0 {
				return strings.TrimSpace(l[strings.LastIndex(l, "] ")+2:])
			}
			return strings.TrimSpace(l)
		}
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// Change is a new version of the bootstrap file.
type Change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	BaseSHA string `json:"baseSha"`
}

// readyTimeout is how long Apply waits for Envoy to come back ready.
const readyTimeout = 40 * time.Second

// Apply writes a new bootstrap config: only if the file is still the one
// that was read, only once it passes Validate, with the previous version
// backed up. Envoy is then restarted (it can't reload a static config) and
// must report LIVE within readyTimeout; if it doesn't, the previous file is
// put back and Envoy restarted on it.
func Apply(c *sshexec.Client, ct *dockerx.Container, ch Change, log io.Writer) error {
	if !path.IsAbs(ch.Path) {
		return fmt.Errorf("%s is not an absolute path", ch.Path)
	}
	if ch.Path != ConfigPath(ct.Args) {
		return fmt.Errorf("%s isn't the config this Envoy runs (%s)", ch.Path, ConfigPath(ct.Args))
	}
	fmt.Fprintln(log, "Checking that the config hasn't changed on the server since it was read...")
	current, err := dockerx.ReadFiles(c, ct, []string{ch.Path})
	if err != nil {
		return err
	}
	if len(current) == 0 || current[0].SHA != ch.BaseSHA {
		return fmt.Errorf("%s was changed on the server since it was read; inspect again and retry", ch.Path)
	}

	fmt.Fprintln(log, "Validating the new config (envoy --mode validate, in a throwaway container)...")
	ok, out, err := Validate(c, ct, ch.Content)
	if err != nil {
		return err
	}
	fmt.Fprintln(log, indent(out))
	if !ok {
		fmt.Fprintln(log, "Validation failed. Nothing was written; Envoy keeps running its current config.")
		return fmt.Errorf("envoy rejected the config, so nothing was applied: %s", lastLine(out))
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	where, err := dockerx.WriteFile(c, ct, ch.Path, ch.Content, stamp)
	if err != nil {
		return fmt.Errorf("writing %s: %v", ch.Path, err)
	}
	fmt.Fprintf(log, "Wrote %s\n", where)

	admin := ParseConfig(ch.Content).Admin
	fmt.Fprintln(log, "Restarting Envoy to load it (a static config needs a restart; open connections are closed)...")
	if err := restartAndWait(c, ct, admin, log); err != nil {
		fmt.Fprintf(log, "Envoy didn't come back ready: %v\nPutting the previous config back and restarting it...\n", err)
		if rerr := dockerx.RestoreFile(c, ct, ch.Path, stamp); rerr != nil {
			fmt.Fprintf(log, "Could not restore %s: %v\n", ch.Path, rerr)
			return fmt.Errorf("Envoy didn't come back ready, and restoring the previous config failed: %v", rerr)
		}
		if rerr := restartAndWait(c, ct, ParseConfig(current[0].Content).Admin, log); rerr != nil {
			return fmt.Errorf("Envoy didn't come back ready with the new config, nor after restoring the previous one: %v", rerr)
		}
		return fmt.Errorf("Envoy didn't come back ready with the new config (%v); the previous config was restored and Envoy is running it", err)
	}
	fmt.Fprintln(log, "Envoy is LIVE with the new config.")
	return nil
}

// restartAndWait restarts the container and waits for Envoy to report
// LIVE on its admin API — or, with no admin API, for the container to
// stay running.
func restartAndWait(c *sshexec.Client, ct *dockerx.Container, admin string, log io.Writer) error {
	if _, errOut, err := c.Output(sshexec.AsRoot("docker restart -t 10 " + sshexec.ShellQuote(ct.Name))); err != nil {
		return fmt.Errorf("docker restart failed: %v %s", err, strings.TrimSpace(errOut))
	}
	deadline := time.Now().Add(readyTimeout)
	last := ""
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		fresh, err := dockerx.Inspect(c, ct.Name)
		if err != nil {
			last = err.Error()
			continue
		}
		if fresh.State != "running" {
			last = "the container is " + fresh.State
			continue
		}
		if admin == "" {
			return nil
		}
		ct.IP = fresh.IP
		code, body, err := AdminGet(c, fresh, admin, "/ready")
		if err == nil && code == http.StatusOK && strings.Contains(string(body), "LIVE") {
			return nil
		}
		last = strings.TrimSpace(string(body))
		if err != nil {
			last = err.Error()
		}
	}
	if last == "" {
		last = "timed out"
	}
	return fmt.Errorf("not ready after %s: %s", readyTimeout, last)
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
