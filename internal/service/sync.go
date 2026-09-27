package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

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
}

// CheckHealth SSHes to the service's target and inspects whether it's
// actually running right now — not whether the last deploy succeeded.
func CheckHealth(target sshexec.Target, svc *Service) (health, detail string) {
	client, err := sshexec.Dial(target)
	if err != nil {
		return "unreachable", "SSH connection failed: " + err.Error()
	}
	defer client.Close()

	if svc.Connected() {
		return checkConnectedHealth(client, svc)
	}
	if svc.Type == "kubernetes" {
		return checkKubernetesHealth(client, svc)
	}
	return checkComposeHealth(client, svc)
}

// checkConnectedHealth checks a stack the panel didn't deploy. Its compose
// file may have any name and live anywhere, so containers are found by
// their compose project label (or, outside Compose, by the pinned
// container) rather than by running compose in a folder.
func checkConnectedHealth(client *sshexec.Client, svc *Service) (string, string) {
	filter := "name=^" + svc.Container + "$"
	if svc.ComposeProject != "" {
		filter = "label=com.docker.compose.project=" + svc.ComposeProject
	}
	var out strings.Builder
	cmd := "docker ps -a --filter " + sshexec.ShellQuote(filter) + " --format '{{.Names}}\t{{.State}}\t{{.Status}}'"
	if err := client.Run(cmd, &out); err != nil {
		return "unreachable", "docker ps failed: " + err.Error()
	}
	var entries []composePsEntry
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		health := ""
		if strings.Contains(parts[2], "(unhealthy)") {
			health = "unhealthy"
		}
		entries = append(entries, composePsEntry{Service: parts[0], Name: parts[0], State: parts[1], Health: health})
	}
	health, detail := summarizeContainers(client, entries)
	if health != "healthy" {
		return health, detail
	}
	if svc.Type == "nginx" {
		// Serving isn't enough: the config on disk must also pass nginx -t,
		// or the next reload or restart would take the site down.
		out, errOut, err := client.Output("docker exec " + sshexec.ShellQuote(svc.Container) + " nginx -t")
		if err != nil {
			lines := strings.Split(strings.TrimSpace(out+errOut), "\n")
			return "degraded", detail + " — nginx -t fails: " + lines[0]
		}
		return "healthy", detail + ", config test passes"
	}
	if svc.Type != "prometheus" {
		return health, detail
	}
	// Running containers aren't the whole story: Prometheus itself must be
	// ready to serve, and running the config it was last given.
	inst, err := promctl.DiscoverContainer(client, svc.Container)
	if err != nil {
		return "degraded", detail + " — Prometheus container not found: " + err.Error()
	}
	if ok, why := promctl.Ready(client, inst); !ok {
		return "degraded", detail + " — Prometheus is not ready: " + why
	}
	return "healthy", detail + ", Prometheus ready"
}

func checkComposeHealth(client *sshexec.Client, svc *Service) (string, string) {
	var out strings.Builder
	cmd := fmt.Sprintf("cd %s && docker compose ps --format json", sshexec.ShellQuote(svc.DeployDir))
	if err := client.Run(cmd, &out); err != nil {
		return "unreachable", "docker compose ps failed: " + err.Error()
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
// one-line detail, explaining any container that isn't running cleanly.
func summarizeContainers(client *sshexec.Client, entries []composePsEntry) (string, string) {
	if len(entries) == 0 {
		return "unreachable", "no containers found"
	}

	running := 0
	var crashing []composePsEntry
	for _, e := range entries {
		if e.State == "running" {
			running++
		}
		// Not running at all, or running but failing its own healthcheck
		// (e.g. up but never became ready) — either way, worth explaining.
		if e.State != "running" || strings.EqualFold(e.Health, "unhealthy") {
			crashing = append(crashing, e)
		}
	}

	detail := fmt.Sprintf("%d/%d running", running, len(entries))
	if len(crashing) == 0 {
		return "healthy", detail
	}

	var reasons []string
	for _, e := range crashing {
		if reason := crashReason(client, e.Name); reason != "" {
			reasons = append(reasons, fmt.Sprintf("%s: %s", e.Service, reason))
		}
	}
	if len(reasons) > 0 {
		detail += " — " + strings.Join(reasons, "; ")
	}
	return "degraded", detail
}

// crashReason inspects containerName's own crash signal (a non-zero exit,
// an OOM kill) and its recent logs, in one SSH round trip, and folds them
// into a short human-readable explanation. Docker's own state often just
// says "exited 1"; the real cause — a missing file, a permission error, an
// unhandled exception — is usually the one line in the logs. Returns ""
// on any failure; callers treat that as "no extra detail available", not
// an error worth surfacing on its own.
func crashReason(client *sshexec.Client, containerName string) string {
	quoted := sshexec.ShellQuote(containerName)
	cmd := "docker inspect " + quoted +
		" --format '{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.State.Error}}'" +
		"; echo '@@LOGS@@'; docker logs --tail 40 " + quoted + " 2>&1"

	var out strings.Builder
	if err := client.Run(cmd, &out); err != nil {
		return ""
	}

	inspectPart, logPart, _ := strings.Cut(out.String(), "@@LOGS@@")

	var parts []string
	fields := strings.SplitN(strings.TrimSpace(inspectPart), "|", 4)
	if len(fields) == 4 {
		status, exitCode, oomKilled, stateErr := fields[0], fields[1], fields[2], fields[3]
		switch {
		case oomKilled == "true":
			parts = append(parts, "OOM-killed")
		case status == "restarting":
			parts = append(parts, "crash-looping (last exit code "+exitCode+")")
		case status == "exited" && exitCode != "0":
			parts = append(parts, "exited with code "+exitCode)
		}
		if stateErr != "" {
			parts = append(parts, stateErr)
		}
	}

	if line := lastErrorLikeLine(logPart); line != "" {
		parts = append(parts, line)
	}

	return strings.Join(parts, "; ")
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
