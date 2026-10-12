// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/envoyctl"
	"devops-wrapper/internal/gitlabctl"
	"devops-wrapper/internal/haproxyctl"
	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// composePsEntry matches one line of `docker compose ps --format json`
// (newline-delimited JSON, one object per container) — verified against a
// real docker compose v2 stack rather than assumed from memory.
type composePsEntry struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Status  string `json:"Status"` // "Up 2 hours (Paused)", "Exited (0) 3 minutes ago"
	Image   string `json:"Image"`  // as the container was created from: "nginx:alpine"
}

// ContainerState is one container of a stack at the last health check:
// Docker's own state and status, and what the panel made of it.
type ContainerState struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	State   string `json:"state"`  // running, paused, exited, restarting, created, dead
	Status  string `json:"status"` // as docker ps shows it
	Image   string `json:"image"`  // the image it runs, e.g. "nginx:1.27-alpine"
	// Role is the panel's reading: "running", "starting", "unhealthy",
	// "paused", "stopped", "job" (a setup job that finished) or "failing"
	// (crashed, crash-looping, dead).
	Role string `json:"role"`
	Note string `json:"note,omitempty"`
}

// CheckHealth SSHes to the service's target and inspects whether it's
// actually running right now — not whether the last deploy succeeded.
// obs is every currently-running container's resource use and every port
// the stack publishes, each tried from the panel (empty for Kubernetes,
// which has no fixed notion of "the containers").
func CheckHealth(target sshexec.Target, svc *Service) (health, detail string, obs Observed) {
	client, err := sshexec.Dial(target)
	if err != nil {
		return "unreachable", "SSH connection failed: " + err.Error(), Observed{}
	}
	defer client.Close()
	return CheckHealthOn(client, target.Host, svc)
}

// CheckHealthOn is CheckHealth over a connection already open to the
// service's server (reached at host), so one connection can check every
// service on that server.
func CheckHealthOn(client *sshexec.Client, host string, svc *Service) (health, detail string, obs Observed) {
	if svc.Type == "kubernetes" && !svc.Connected() {
		health, detail := checkKubernetesHealth(client, svc)
		return health, detail, Observed{}
	}
	if svc.Connected() {
		health, detail, obs = checkConnectedHealth(client, svc)
	} else {
		health, detail, obs = checkComposeHealth(client, svc)
	}
	// The server answered, it just runs nothing of this stack: that's not
	// "unreachable". Never deployed, or down (taken down, or gone).
	if health == "unreachable" && detail == "no containers found" {
		switch {
		case svc.Status == StatusPending && !svc.Connected():
			return "not deployed", "never deployed — nothing runs on the server yet", obs
		case svc.Status == StatusDown:
			return "down", "taken down — no containers", obs
		default:
			return "down", "no containers — nothing of it runs on the server", obs
		}
	}
	probePorts(host, obs.Ports)
	return health, detail, obs
}

// checkConnectedHealth checks a stack the panel didn't deploy. Its compose
// file may have any name and live anywhere, so containers are found by
// their compose project label (or, outside Compose, by the pinned
// container) rather than by running compose in a folder.
func checkConnectedHealth(client *sshexec.Client, svc *Service) (string, string, Observed) {
	filter := "name=^" + svc.Container + "$"
	if svc.ComposeProject != "" {
		filter = "label=com.docker.compose.project=" + svc.ComposeProject
	}
	var out strings.Builder
	cmd := sshexec.AsRoot("docker ps -a --filter " + sshexec.ShellQuote(filter) + " --format '{{.Names}}\t{{.State}}\t{{.Status}}\t{{.Image}}'")
	if err := client.Run(cmd, &out); err != nil {
		return "unreachable", "docker ps failed: " + err.Error(), Observed{}
	}
	var entries []composePsEntry
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		health := ""
		switch {
		case strings.Contains(parts[2], "(unhealthy)"):
			health = "unhealthy"
		case strings.Contains(parts[2], "(health: starting)"):
			health = "starting"
		}
		entries = append(entries, composePsEntry{Service: parts[0], Name: parts[0], State: parts[1], Health: health, Status: parts[2], Image: parts[3]})
	}
	health, detail, stats := summarizeContainers(client, entries)
	if health != "healthy" {
		return health, detail, stats
	}
	if svc.Type == "nginx" {
		// Serving isn't enough: the config on disk must also pass nginx -t,
		// or the next reload or restart would take the site down.
		out, errOut, err := client.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(svc.Container) + " nginx -t"))
		if err != nil {
			lines := strings.Split(strings.TrimSpace(out+errOut), "\n")
			return "degraded", detail + " — nginx -t fails: " + lines[0], stats
		}
		return "healthy", detail + ", config test passes", stats
	}
	if svc.Type == "haproxy" {
		// A connected HAProxy's config is arbitrary — there's no fixed
		// /healthz to assume — so this reads it fresh and checks whatever
		// probe it actually determines (a monitor-uri, else its stats
		// page, else just a bound port).
		ct, err := dockerx.Inspect(client, svc.Container)
		if err != nil {
			return "degraded", detail + " — could not inspect the HAProxy container: " + err.Error(), stats
		}
		ok, why := haproxyctl.CheckHealth(client, ct)
		if !ok {
			return "degraded", detail + " — " + why, stats
		}
		return "healthy", detail + ", " + why, stats
	}
	if svc.Type == "envoy" {
		// Envoy's own verdict (its admin API's state) and its endpoints'
		// health, read from the config the container runs.
		ct, err := dockerx.Inspect(client, svc.Container)
		if err != nil {
			return "degraded", detail + " — could not inspect the Envoy container: " + err.Error(), stats
		}
		ok, why := envoyctl.CheckHealth(client, ct)
		if !ok {
			return "degraded", detail + " — " + why, stats
		}
		return "healthy", detail + ", " + why, stats
	}
	if svc.Type == "gitlab" {
		// A running container isn't a working GitLab: any of the services
		// inside it (puma, sidekiq, gitaly, postgresql, ...) can be down.
		ok, why := gitlabctl.CheckHealth(client, svc.Container)
		if !ok {
			return "degraded", detail + " — " + why, stats
		}
		return "healthy", detail + ", " + why, stats
	}
	if svc.Type != "prometheus" {
		return health, detail, stats
	}
	// Running containers aren't the whole story: Prometheus itself must be
	// ready to serve, and running the config it was last given.
	inst, err := promctl.DiscoverContainer(client, svc.Container)
	if err != nil {
		return "degraded", detail + " — Prometheus container not found: " + err.Error(), stats
	}
	if ok, why := promctl.Ready(client, inst); !ok {
		return "degraded", detail + " — Prometheus is not ready: " + why, stats
	}
	return "healthy", detail + ", Prometheus ready", stats
}

// noDeployDir marks a deploy folder that isn't on the server.
const noDeployDir = "@@PANEL-NO-DEPLOY-DIR@@"

func checkComposeHealth(client *sshexec.Client, svc *Service) (string, string, Observed) {
	var out strings.Builder
	dir := sshexec.ShellQuote(svc.ComposeDir())
	cmd := sshexec.AsRootPrelude + fmt.Sprintf("if ! $AS_ROOT test -d %s; then echo %s; exit 0; fi; ", dir, noDeployDir) + sshexec.ComposeIn(svc.ComposeDir()) + " ps --all --format json"
	stdout, errOut, err := client.Output(cmd)
	out.WriteString(stdout)
	if err != nil {
		// The command's own last error line says why (no such file, a bad
		// compose file, permission denied), where the exit status doesn't.
		lines := strings.Split(strings.TrimSpace(errOut), "\n")
		return "unreachable", "docker compose ps failed: " + strings.TrimSpace(lines[len(lines)-1]), Observed{}
	}
	if strings.Contains(out.String(), noDeployDir) {
		if svc.Status == StatusPending {
			return "unreachable", "no containers found", Observed{}
		}
		return "unreachable", "its deploy folder " + svc.ComposeDir() + " doesn't exist on the server", Observed{}
	}

	var entries []composePsEntry
	scanner := bufio.NewScanner(strings.NewReader(out.String()))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry composePsEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // a stray non-JSON line (warning banner etc.) — skip it
		}
		entries = append(entries, entry)
	}

	return summarizeContainers(client, entries)
}

// summarizeContainers turns container states into a health verdict and a
// one-line detail, and records every container's state. Verified against
// real stacks: a paused stack, a stopped one ("docker compose stop"), and a
// mix of running, paused and stopped containers each read for what they
// are — rather than every non-running container counting as a crash.
func summarizeContainers(client *sshexec.Client, entries []composePsEntry) (string, string, Observed) {
	if len(entries) == 0 {
		return "unreachable", "no containers found", Observed{}
	}

	var runningNames, names []string
	for _, e := range entries {
		names = append(names, e.Name)
		if e.State == "running" {
			runningNames = append(runningNames, e.Name)
		}
	}
	// Only ever passed names just observed running: docker stats prints a
	// meaningless all-zero line for a stopped-but-existing container, and
	// fails the whole batch outright for one that's gone entirely.
	obs := Observed{Stats: dockerx.Stats(client, runningNames), Ports: containerPorts(client, names)}
	awaited := setupJobServices(client, names)

	var running, starting, completed int
	var paused, stopped, failing []string
	for _, e := range entries {
		cs := ContainerState{Name: e.Name, Service: e.Service, State: e.State, Status: e.Status, Image: e.Image}
		switch {
		case e.State == "running" && strings.EqualFold(e.Health, "unhealthy"):
			cs.Role = "unhealthy"
			reason, _ := crashReason(client, e.Name, false)
			cs.Note = reason
			failing = append(failing, fmt.Sprintf("%s: unhealthy%s", e.Service, prefixed("; ", reason)))
		case e.State == "running":
			running++
			cs.Role = "running"
			if strings.EqualFold(e.Health, "starting") {
				starting++
				cs.Role = "starting"
			}
		case e.State == "paused":
			cs.Role = "paused"
			paused = append(paused, e.Service)
		default: // exited, created, restarting, dead
			reason, done := crashReason(client, e.Name, awaited[e.Service])
			switch {
			case done:
				completed++
				cs.Role = "job"
			case e.State == "exited" && cleanStop(reason) || e.State == "created":
				// Just the exit code: a stopped service's last log line is
				// its shutdown message, not news.
				code, _, _ := strings.Cut(reason, ";")
				cs.Role, cs.Note = "stopped", strings.TrimSpace(code)
				stopped = append(stopped, e.Service+prefixed(" (", cs.Note, ")"))
			default:
				cs.Role, cs.Note = "failing", reason
				failing = append(failing, e.Service+prefixed(": ", reason))
			}
		}
		obs.Containers = append(obs.Containers, cs)
	}

	total := len(entries) - completed
	detail := fmt.Sprintf("%d/%d running", running, total)
	if completed > 0 {
		detail += fmt.Sprintf(", %d setup job%s completed", completed, map[bool]string{true: "", false: "s"}[completed == 1])
	}
	if len(paused) > 0 {
		detail += " · paused: " + strings.Join(paused, ", ")
	}
	if len(stopped) > 0 {
		detail += " · stopped: " + strings.Join(stopped, ", ")
	}
	if len(failing) > 0 {
		detail += " — " + strings.Join(failing, "; ")
	}

	switch {
	case total > 0 && len(paused) == total:
		return "paused", fmt.Sprintf("all %d container%s paused — unpause to resume", total, plural(total)), obs
	case running == 0 && len(paused) == 0 && len(failing) == 0 && len(stopped) > 0:
		return "stopped", fmt.Sprintf("stopped — %d container%s not running: %s", len(stopped), plural(len(stopped)), stoppedList(obs.Containers)), obs
	case len(failing) > 0 || len(paused) > 0 || len(stopped) > 0 || running == 0:
		return "degraded", detail, obs
	// A container whose own healthcheck hasn't passed yet isn't healthy —
	// ClickHouse, GitLab and Nexus take minutes to be ready after they're
	// "running". Verified on SigNoz: calling the stack healthy at this point
	// was wrong, as ClickHouse only became ready a minute later.
	case starting > 0:
		return "starting", detail + fmt.Sprintf(", %d still starting", starting), obs
	}
	return "healthy", detail, obs
}

// stoppedList names the stopped containers, giving the exit once when they
// all stopped the same way: "api, web, worker (exited with code 0)".
func stoppedList(cs []ContainerState) string {
	var names []string
	notes := map[string]bool{}
	for _, c := range cs {
		if c.Role == "stopped" {
			names = append(names, c.Service)
			notes[c.Note] = true
		}
	}
	if len(notes) == 1 {
		for n := range notes {
			return strings.Join(names, ", ") + prefixed(" (", n, ")")
		}
	}
	var parts []string
	for _, c := range cs {
		if c.Role == "stopped" {
			parts = append(parts, c.Service+prefixed(" (", c.Note, ")"))
		}
	}
	return strings.Join(parts, ", ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// prefixed wraps a non-empty s in before/after, for optional detail.
func prefixed(before, s string, after ...string) string {
	if s == "" {
		return ""
	}
	return before + s + strings.Join(after, "")
}

// cleanStop reports whether an exited container's reason is a plain stop:
// exit code 0, or the 143/137 of SIGTERM/SIGKILL that "docker stop" sends —
// stopped, not crashed.
func cleanStop(reason string) bool {
	code, _, _ := strings.Cut(reason, ";")
	switch strings.TrimSpace(code) {
	case "", "exited with code 0", "exited with code 143", "exited with code 137":
		return true
	}
	return false
}

// setupJobServices finds the services other services wait to finish
// ("depends_on: x: condition: service_completed_successfully"), from the
// label compose puts on each dependent container — verified format:
// "migrate:service_completed_successfully:false", comma-separated.
func setupJobServices(client *sshexec.Client, names []string) map[string]bool {
	out := map[string]bool{}
	if len(names) == 0 {
		return out
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = sshexec.ShellQuote(n)
	}
	res, _, _ := client.Output(sshexec.AsRoot("docker inspect --format " + sshexec.ShellQuote(`{{index .Config.Labels "com.docker.compose.depends_on"}}`) + " " + strings.Join(quoted, " ")))
	for _, line := range strings.Split(res, "\n") {
		for _, dep := range strings.Split(strings.TrimSpace(line), ",") {
			parts := strings.Split(dep, ":")
			if len(parts) >= 2 && parts[1] == "service_completed_successfully" {
				out[parts[0]] = true
			}
		}
	}
	return out
}

// crashReason inspects containerName's own crash signal (a non-zero exit,
// an OOM kill) and its recent logs, in one SSH round trip, and folds them
// into a short human-readable explanation. Docker's own state often just
// says "exited 1"; the real cause — a missing file, a permission error, an
// unhandled exception — is usually the one line in the logs. Returns ""
// on any failure; callers treat that as "no extra detail available", not
// an error worth surfacing on its own.
func crashReason(client *sshexec.Client, containerName string, awaited bool) (reason string, completed bool) {
	quoted := sshexec.ShellQuote(containerName)
	cmd := sshexec.AsRootPrelude + "$AS_ROOT docker inspect " + quoted +
		" --format '{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.HostConfig.RestartPolicy.Name}}|{{.State.Error}}'" +
		"; echo '@@LOGS@@'; $AS_ROOT docker logs --tail 40 " + quoted + " 2>&1"

	var out strings.Builder
	if err := client.Run(cmd, &out); err != nil {
		return "", false
	}

	inspectPart, logPart, _ := strings.Cut(out.String(), "@@LOGS@@")

	var parts []string
	fields := strings.SplitN(strings.TrimSpace(inspectPart), "|", 5)
	if len(fields) == 5 {
		status, exitCode, oomKilled, restart, stateErr := fields[0], fields[1], fields[2], fields[3], fields[4]
		if completedJob(status, exitCode, restart, awaited) {
			return "", true
		}
		switch {
		case oomKilled == "true":
			parts = append(parts, "OOM-killed")
		case status == "restarting":
			parts = append(parts, "crash-looping (last exit code "+exitCode+")")
		case status == "exited":
			parts = append(parts, "exited with code "+exitCode)
		}
		if stateErr != "" {
			parts = append(parts, stateErr)
		}
	}

	if line := lastErrorLikeLine(logPart); line != "" {
		parts = append(parts, line)
	}

	return strings.Join(parts, "; "), false
}

// completedJob reports whether a stopped container is a setup job that
// finished (a migrator, an init step) rather than a service that went down:
// it exited cleanly, isn't meant to stay up ("always"/"unless-stopped"),
// and is a job by its setup — another service waits for it to complete
// (awaited), or it restarts only on failure, as one-shot jobs do (SigNoz's
// migrator). A plain service stopped with "docker compose stop" (restart
// "no", nothing waiting on it) is stopped, not a finished job.
func completedJob(status, exitCode, restartPolicy string, awaited bool) bool {
	if status != "exited" || exitCode != "0" || restartPolicy == "always" || restartPolicy == "unless-stopped" {
		return false
	}
	return awaited || restartPolicy == "on-failure"
}

// errorLineKeywords are substrings that tend to mark the actual cause line
// in a crash log, as opposed to the surrounding noise (stack frame
// continuations, timestamps, informational lines).
var errorLineKeywords = []string{"Exception", "Error", "error", "FATAL", "fatal", "panic:", "Caused by:"}

// lastErrorLikeLine scans logs for the last line containing one of
// errorLineKeywords — in a nested stack trace, later matches are usually
// closer to the root cause than the first exception thrown — falling back
// to the last non-empty line if nothing matches. Truncated so one giant
// stack trace line doesn't dominate the health detail.
func lastErrorLikeLine(logs string) string {
	lines := strings.Split(logs, "\n")

	var lastMatch, lastNonEmpty string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lastNonEmpty = trimmed
		for _, kw := range errorLineKeywords {
			if strings.Contains(trimmed, kw) {
				lastMatch = trimmed
				break
			}
		}
	}

	result := lastMatch
	if result == "" {
		result = lastNonEmpty
	}
	const maxLen = 300
	if len(result) > maxLen {
		result = result[:maxLen] + "…"
	}
	return result
}

// checkKubernetesHealth is deliberately lighter-weight than the compose
// check: it confirms the manifest's resources still exist on the cluster,
// not that every pod is Ready. A real readiness check would need
// kind-specific logic per resource type in an arbitrary user-pasted
// manifest, which is real scope beyond "does this still exist."
func checkKubernetesHealth(client *sshexec.Client, svc *Service) (string, string) {
	var out strings.Builder
	manifestPath := svc.DeployDir + "/manifest.yaml"
	cmd := "kubectl get -f " + sshexec.ShellQuote(manifestPath) + " --no-headers"
	if err := client.Run(cmd, &out); err != nil {
		return "unreachable", "kubectl get failed: " + err.Error()
	}

	count := 0
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count == 0 {
		return "unreachable", "no resources found"
	}
	return "healthy", fmt.Sprintf("%d resource(s) found (existence only, not readiness)", count)
}
