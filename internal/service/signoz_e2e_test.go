// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/sshexec"
)

// TestSigNozEndToEnd deploys SigNoz through Foundry to a real throwaway
// server, checks the panel reads it as healthy (its two one-shot setup jobs
// counted as completed, not crashed) and its UI answers, redeploys without
// changes (nothing recreated), then deletes it. Heavy — it pulls ClickHouse,
// Keeper, Postgres, SigNoz and the collector — so it runs only with
// E2E_SIGNOZ=1, plus E2E_SSH_TARGET (host:port), E2E_SSH_PASSWORD and
// optionally E2E_SSH_USER. Never point it at a real server.
func TestSigNozEndToEnd(t *testing.T) {
	target, password := os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if os.Getenv("E2E_SIGNOZ") != "1" || target == "" || password == "" {
		t.Skip("set E2E_SIGNOZ=1, E2E_SSH_TARGET and E2E_SSH_PASSWORD to a throwaway server to run")
	}
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	ssh := sshTarget(host, port, password)
	const dir = "/opt/signoz-e2e"
	svc := &Service{Type: "signoz", DeployDir: dir, Manifest: deploy.SigNozDefaultCasting}

	jobs := deploy.NewStore()
	run := func(t *testing.T, job *deploy.Job, within time.Duration) string {
		t.Helper()
		deadline := time.Now().Add(within)
		for job.Snapshot().Status == deploy.StatusRunning {
			if time.Now().After(deadline) {
				t.Fatalf("timed out:\n%s", strings.Join(job.Snapshot().Log, ""))
			}
			time.Sleep(2 * time.Second)
		}
		log := strings.Join(job.Snapshot().Log, "")
		if job.Snapshot().Status != deploy.StatusSuccess {
			t.Fatalf("job failed:\n%s", log)
		}
		return log
	}

	log := run(t, jobs.RunSigNoz(ssh, dir, svc.Manifest), 20*time.Minute)
	if !strings.Contains(log, "Installed foundryctl "+deploy.FoundryVersion+" (SHA-256 verified)") {
		t.Errorf("foundryctl wasn't installed with a verified checksum:\n%s", log)
	}

	// Checking as the panel's Sync does: "starting" while ClickHouse and SigNoz
	// boot (their healthchecks haven't passed), then healthy once they have
	// and both setup jobs (migrator, scripts) have completed.
	// The migrator may restart (on-failure) until ClickHouse is ready, so the
	// stack can look degraded for a moment while it converges; only the end
	// state is judged, and every state seen is logged.
	var health, detail string
	sawStarting := false
	seen := map[string]bool{}
	for i := 0; i < 120; i++ {
		health, detail, _ = CheckHealth(ssh, svc)
		if !seen[health] {
			seen[health] = true
			t.Logf("health went %s: %s", health, detail)
		}
		if health == "starting" {
			sawStarting = true
		}
		if health == "healthy" && strings.Contains(detail, "2 setup jobs completed") {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if health != "healthy" || !strings.Contains(detail, "2 setup jobs completed") {
		t.Fatalf("health = %s %q; want healthy with both setup jobs completed", health, detail)
	}
	if !sawStarting {
		t.Error("never reported \"starting\" while the stack booted")
	}
	t.Logf("health: %s — %s", health, detail)

	c, err := sshexec.Dial(ssh)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out, errOut, err := c.Output(sshexec.AsRoot("sh -c 'wget -qO- http://127.0.0.1:8080/api/v1/health'"))
	if err != nil {
		t.Errorf("SigNoz UI health: %v %s", err, errOut)
	} else {
		t.Logf("SigNoz /api/v1/health: %s", strings.TrimSpace(out))
	}

	// Same casting again: nothing changed, so nothing is recreated.
	log = run(t, jobs.RunSigNoz(ssh, dir, svc.Manifest), 10*time.Minute)
	if !strings.Contains(log, "is already installed") || strings.Contains(log, "--force-recreate") {
		t.Errorf("an unchanged redeploy reinstalled or recreated:\n%s", log)
	}

	// Delete: the stack, its volumes and the whole folder go.
	run(t, jobs.RunComposeDestroy(ssh, svc.ComposeDir(), dir), 10*time.Minute)
	left, _, _ := c.Output(sshexec.AsRoot("sh -c 'ls -d " + dir + " 2>/dev/null; docker ps -aq --filter label=com.docker.compose.project=signoz; docker volume ls -q --filter label=com.docker.compose.project=signoz'"))
	if strings.TrimSpace(left) != "" {
		t.Errorf("after Delete, still there:\n%s", left)
	}
}
