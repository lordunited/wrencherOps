// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

// TestSigNozComposeEndToEnd creates SigNoz from the compose template as the
// SigNoz tab does — template, then a custom compose app — deploys it through
// the API to a real throwaway server, and checks the panel reads it as
// healthy (both one-shot setup jobs completed), the UI answers, and SigNoz
// is using the generated Postgres password. With E2E_NO_GITHUB=1 the server
// is expected to have no DNS (images pulled beforehand), as a server that
// can't reach GitHub. Heavy, so it runs only with
// E2E_SIGNOZ=1 plus TEST_DATABASE_URL, E2E_SSH_TARGET (host:port),
// E2E_SSH_PASSWORD and optionally E2E_SSH_USER. Never point it at real systems.
func TestSigNozComposeEndToEnd(t *testing.T) {
	dbURL, target, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if os.Getenv("E2E_SIGNOZ") != "1" || dbURL == "" || target == "" || password == "" {
		t.Skip("set E2E_SIGNOZ=1, TEST_DATABASE_URL, E2E_SSH_TARGET and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	jobs := deploy.NewStore()
	h := NewHandler(NewStore(pool), NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), jobs, box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}

	rec := do("GET", "/api/apps/templates/signoz", nil)
	var tmpl deploy.ComposeTemplate
	json.Unmarshal(rec.Body.Bytes(), &tmpl)
	if rec.Code != http.StatusOK || tmpl.Manifest == "" {
		t.Fatalf("template: %d %s", rec.Code, rec.Body)
	}
	pw := ""
	for _, line := range strings.Split(tmpl.Env, "\n") {
		if v, ok := strings.CutPrefix(line, "SIGNOZ_POSTGRES_PASSWORD="); ok {
			pw = v
		}
	}

	rec = do("POST", "/api/apps", map[string]any{
		"name": tmpl.Name, "deployDir": tmpl.DeployDir, "manifest": tmpl.Manifest, "env": tmpl.Env, "files": tmpl.Files,
		"composeFile": tmpl.ComposeFile, "template": tmpl.Template,
		"servers": []map[string]any{{"host": host, "port": port, "username": e2eUser(), "password": password}},
	})
	var app App
	json.Unmarshal(rec.Body.Bytes(), &app)
	if rec.Code != http.StatusCreated || app.Template != "signoz" || app.ComposeFile != "compose.yaml" || len(app.Servers) != 1 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if got, ok := h.apps.Get(ctx, app.ID); !ok || got.Template != "signoz" {
		t.Fatalf("template not stored: %+v", got)
	}
	svc := app.Servers[0]

	rec = do("POST", "/api/services/"+svc.ID+"/deploy", map[string]any{})
	var started struct{ JobID string }
	json.Unmarshal(rec.Body.Bytes(), &started)
	job, ok := jobs.Get(started.JobID)
	if rec.Code != http.StatusOK || !ok {
		t.Fatalf("deploy: %d %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(20 * time.Minute)
	for job.Snapshot().Status == deploy.StatusRunning {
		if time.Now().After(deadline) {
			t.Fatalf("deploy timed out:\n%s", strings.Join(job.Snapshot().Log, ""))
		}
		time.Sleep(2 * time.Second)
	}
	if log := strings.Join(job.Snapshot().Log, ""); job.Snapshot().Status != deploy.StatusSuccess {
		t.Fatalf("deploy failed:\n%s", log)
	}

	ssh := sshTarget(host, port, password)
	var health, detail string
	seen := map[string]bool{}
	for i := 0; i < 120; i++ {
		health, detail, _ = CheckHealth(ssh, svc)
		if !seen[health] {
			seen[health] = true
			t.Logf("health went %s: %s", health, detail)
		}
		if health == "healthy" && strings.Contains(detail, "2 setup jobs completed") {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if health != "healthy" || !strings.Contains(detail, "2 setup jobs completed") {
		t.Fatalf("health = %s %q; want healthy with both setup jobs completed", health, detail)
	}

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
	// histogramQuantile came from the panel's bundled copy, not a download.
	scripts, _, _ := c.Output(sshexec.AsRoot("sh -c 'docker logs signoz-telemetrystore-clickhouse-user-scripts 2>&1; docker exec signoz-telemetrystore-clickhouse-0-0 ls -l /var/lib/clickhouse/user_scripts/histogramQuantile'"))
	if !strings.Contains(scripts, "bundled by the panel, no download") || !strings.Contains(scripts, "histogramQuantile") {
		t.Errorf("histogram-quantile setup:\n%s", scripts)
	}
	// The pinned images run, and Postgres took the generated password.
	images, _, _ := c.Output(sshexec.AsRoot("docker ps --format '{{.Image}}' --filter label=com.docker.compose.project=signoz"))
	if !strings.Contains(images, "signoz/signoz:"+deploy.SigNozVersion) || !strings.Contains(images, "signoz/signoz-otel-collector:"+deploy.SigNozCollectorVersion) {
		t.Errorf("running images:\n%s", images)
	}
	if _, errOut, err := c.Output(sshexec.AsRoot("docker exec -e PGPASSWORD=" + sshexec.ShellQuote(pw) + " signoz-metastore-postgres-0 psql -h 127.0.0.1 -U signoz -d signoz -c 'select 1'")); err != nil {
		t.Errorf("Postgres doesn't take the generated password: %v %s", err, errOut)
	}
}
