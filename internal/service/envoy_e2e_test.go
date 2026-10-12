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
	"devops-wrapper/internal/envoyctl"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

// TestEnvoyEndToEnd deploys an Envoy through the panel's API to a throwaway
// server (needs envoyproxy/envoy:v1.39-latest and nginx:alpine there), adds
// a route to a backend through preview → test → apply, checks traffic and
// endpoint health, and that a broken or stale change is refused without
// touching anything. Runs with E2E_ENVOY=1, TEST_DATABASE_URL,
// E2E_SSH_TARGET (host:port), E2E_SSH_PASSWORD and E2E_SSH_USER.
func TestEnvoyEndToEnd(t *testing.T) {
	dbURL, target, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if os.Getenv("E2E_ENVOY") != "1" || dbURL == "" || target == "" || password == "" {
		t.Skip("set E2E_ENVOY=1, TEST_DATABASE_URL, E2E_SSH_TARGET and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	jobs := deploy.NewStore()
	store := NewStore(pool)
	h := NewHandler(store, NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), jobs, box)
	store.onCreate = nil
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	do := func(path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", path, &buf))
		return rec
	}

	rec := do("/api/services/envoy", map[string]any{"name": "edge", "host": host, "port": port, "username": e2eUser(), "httpPort": 18200})
	var svc Service
	json.Unmarshal(rec.Body.Bytes(), &svc)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	rec = do("/api/services/"+svc.ID+"/deploy", map[string]any{"password": password})
	var started struct{ JobID string }
	json.Unmarshal(rec.Body.Bytes(), &started)
	job, ok := jobs.Get(started.JobID)
	if !ok {
		t.Fatalf("deploy: %d %s", rec.Code, rec.Body)
	}
	for job.Snapshot().Status == deploy.StatusRunning {
		time.Sleep(time.Second)
	}
	if job.Snapshot().Status != deploy.StatusSuccess {
		t.Fatalf("deploy failed:\n%s", strings.Join(job.Snapshot().Log, ""))
	}

	ssh := sshTarget(host, port, password)
	c, err := sshexec.Dial(ssh)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	run := func(cmd string) string {
		out, errOut, err := c.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(cmd)))
		if err != nil {
			t.Fatalf("%s: %v %s", cmd, err, errOut)
		}
		return strings.TrimSpace(out)
	}
	// A backend on the stack's network, as "web".
	run("docker run -d --name web --network envoy_default nginx:alpine >/dev/null")
	through := func(path string) string {
		out, _, _ := c.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote("wget -q -O - -S http://127.0.0.1:18200"+path+" 2>&1 | head -1")))
		return strings.TrimSpace(out)
	}

	inspect := func() envoyctl.Report {
		var r envoyctl.Report
		var rec *httptest.ResponseRecorder
		for i := 0; i < 20; i++ { // it may still be starting
			rec = do("/api/services/"+svc.ID+"/envoy/inspect", map[string]any{})
			json.Unmarshal(rec.Body.Bytes(), &r)
			if rec.Code == http.StatusOK && r.AdminReachable {
				return r
			}
			time.Sleep(time.Second)
		}
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body)
		return r
	}
	r := inspect()
	t.Logf("inspect: %s · state %s · admin %s · validate ok=%v · health %v %q", r.Version[:min(40, len(r.Version))], r.State, r.Admin, r.TestOK, r.Healthy, r.HealthDetail)
	if !r.Healthy || r.State != "LIVE" || !r.TestOK || len(r.Files) != 1 || r.Files[0].Path != "/etc/envoy/envoy.yaml" || len(r.Config.Listeners) != 1 {
		t.Fatalf("report: %+v", r)
	}
	if got := through("/"); !strings.Contains(got, "404") {
		t.Errorf("before the route, / = %q, want 404", got)
	}

	// Add a route: preview, test, apply.
	rec = do("/api/services/"+svc.ID+"/envoy/preview", map[string]any{
		"file":  r.Files[0],
		"route": map[string]any{"domains": []string{"*"}, "pathPrefix": "/", "upstreams": []string{"web:80"}, "clusterName": "web", "healthPath": "/"},
	})
	var preview previewResponse
	json.Unmarshal(rec.Body.Bytes(), &preview)
	if rec.Code != http.StatusOK || len(preview.Changes) != 1 {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	t.Logf("preview: %s", preview.Summary)
	ch := map[string]any{"path": preview.Changes[0].Path, "content": preview.Changes[0].After, "baseSha": preview.Changes[0].BaseSHA}
	var test envoyTestResponse
	json.Unmarshal(do("/api/services/"+svc.ID+"/envoy/test", map[string]any{"change": ch}).Body.Bytes(), &test)
	if !test.OK {
		t.Fatalf("test: %s", test.Output)
	}
	var applied applyResponse
	json.Unmarshal(do("/api/services/"+svc.ID+"/envoy/apply", map[string]any{"summary": preview.Summary, "change": ch}).Body.Bytes(), &applied)
	if !applied.OK {
		t.Fatalf("apply:\n%s", applied.Log)
	}
	t.Logf("apply log:\n%s", applied.Log)
	if got := through("/"); !strings.Contains(got, "200") {
		t.Errorf("after the route, / = %q, want 200 from nginx", got)
	}

	// Endpoint health from the admin API (the check runs every 10s).
	var healthy bool
	for i := 0; i < 20 && !healthy; i++ {
		r = inspect()
		for _, lc := range r.LiveClusters {
			if lc.Name == "web" && len(lc.Endpoints) == 1 && lc.Endpoints[0].Healthy {
				healthy = true
			}
		}
		time.Sleep(time.Second)
	}
	if !healthy || !r.Healthy {
		t.Errorf("web endpoint healthy=%v, report healthy=%v %q", healthy, r.Healthy, r.HealthDetail)
	}

	// A broken config: rejected by validation, nothing written, Envoy unchanged.
	broken := strings.Replace(r.Files[0].Content, "cluster: web\n", "cluster: nope\n", 1)
	if broken == r.Files[0].Content {
		t.Fatalf("couldn't make the broken edit in:\n%s", r.Files[0].Content)
	}
	json.Unmarshal(do("/api/services/"+svc.ID+"/envoy/apply", map[string]any{"change": map[string]any{"path": r.Files[0].Path, "content": broken, "baseSha": r.Files[0].SHA}}).Body.Bytes(), &applied)
	if applied.OK || !strings.Contains(applied.Error, "unknown cluster 'nope'") {
		t.Errorf("broken config: ok=%v %q", applied.OK, applied.Error)
	}
	if again := inspect(); again.Files[0].SHA != r.Files[0].SHA || through("/") == "" || !strings.Contains(through("/"), "200") {
		t.Error("a rejected config changed the file or broke traffic")
	}
	// A stale base: refused.
	json.Unmarshal(do("/api/services/"+svc.ID+"/envoy/apply", map[string]any{"change": map[string]any{"path": r.Files[0].Path, "content": r.Files[0].Content, "baseSha": "stale"}}).Body.Bytes(), &applied)
	if applied.OK || !strings.Contains(applied.Error, "changed on the server") {
		t.Errorf("stale apply: %q", applied.Error)
	}

	// Connect existing finds it.
	rec = do("/api/services/connect/discover", map[string]any{"kind": "envoy", "host": host, "port": port, "username": e2eUser(), "password": password})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"container":"envoy"`) || !strings.Contains(rec.Body.String(), `"/etc/envoy/envoy.yaml"`) {
		t.Errorf("discover: %d %s", rec.Code, rec.Body)
	}
}
