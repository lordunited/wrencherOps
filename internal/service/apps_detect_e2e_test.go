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

// TestDetectImportWatchEndToEnd scans a real server for compose projects,
// imports one and watches another, against a throwaway Postgres
// (TEST_DATABASE_URL) and a throwaway server with Docker and SSH
// (E2E_SSH_TARGET as host:port, E2E_SSH_PASSWORD, root login) prepared
// with the three projects described below. Skipped without them; never
// point either at real data.
//
//	/opt/shop     docker-compose.yml, running as "shop", with .env,
//	              conf/app.conf (mounted file) and site/ (mounted folder)
//	/srv/custom   compose.yaml, running as "custom-name" (-p)
//	/home/app     docker-compose.yml on disk, never started
func TestDetectImportWatchEndToEnd(t *testing.T) {
	dbURL, target, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if dbURL == "" || target == "" || password == "" {
		t.Skip("set TEST_DATABASE_URL, E2E_SSH_TARGET and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	h := NewHandler(NewStore(pool), NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), deploy.NewStore(), box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	creds := map[string]any{"host": host, "port": port, "username": e2eUser(), "password": password}
	call := func(path string, extra map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body := map[string]any{}
		for k, v := range creds {
			body[k] = v
		}
		for k, v := range extra {
			body[k] = v
		}
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", path, &buf))
		return rec
	}
	detect := func() map[string]DetectedProject {
		t.Helper()
		rec := call("/api/apps/detect", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("detect: %d %s", rec.Code, rec.Body)
		}
		var list []DetectedProject
		json.Unmarshal(rec.Body.Bytes(), &list)
		by := map[string]DetectedProject{}
		for _, p := range list {
			by[p.WorkingDir] = p
		}
		return by
	}

	found := detect()
	shop, custom, onDisk := found["/opt/shop"], found["/srv/custom"], found["/home/app"]
	if shop.Name != "shop" || !strings.HasPrefix(shop.Status, "running") || !shop.Importable || !shop.Watchable {
		t.Errorf("shop = %+v", shop)
	}
	if custom.Name != "custom-name" || custom.Importable || !strings.Contains(custom.ImportProblem, `"custom-name"`) || !custom.Watchable {
		t.Errorf("custom = %+v", custom)
	}
	if onDisk.Name != "app" || onDisk.Status != "" || !onDisk.Importable || onDisk.Watchable {
		t.Errorf("on-disk project = %+v", onDisk)
	}

	// Import shop: compose, .env and the mounted file come along; the
	// mounted folder stays behind, and says so.
	rec := call("/api/apps/import", map[string]any{"workingDir": "/opt/shop"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}
	var imported importResponse
	json.Unmarshal(rec.Body.Bytes(), &imported)
	app := imported.App
	if app.DeployDir != "/opt/shop" || !strings.Contains(app.Manifest, "nginx:${TAG}") || app.Env != "TAG=alpine\n" {
		t.Errorf("imported app = %+v", app)
	}
	if len(app.Files) != 1 || app.Files[0].Path != "conf/app.conf" || app.Files[0].Content != "x=1\n" {
		t.Errorf("imported files = %+v", app.Files)
	}
	if len(app.Servers) != 1 || app.Servers[0].Status != StatusDeployed || !app.Servers[0].HasSavedPassword {
		t.Errorf("imported server = %+v", app.Servers)
	}
	if notes := strings.Join(imported.Notes, "\n"); !strings.Contains(notes, "site is a folder") {
		t.Errorf("notes = %q, want one about the site folder", notes)
	}

	// Redeploying the imported app from the panel updates the same project in
	// place — still one "shop", never a second copy beside it.
	deployRec := httptest.NewRecorder()
	mux.ServeHTTP(deployRec, httptest.NewRequest("POST", "/api/services/"+app.Servers[0].ID+"/deploy", strings.NewReader(`{"password":""}`)))
	if deployRec.Code != http.StatusOK {
		t.Fatalf("deploy imported app: %d %s", deployRec.Code, deployRec.Body)
	}
	var started struct{ JobID string }
	json.Unmarshal(deployRec.Body.Bytes(), &started)
	job, ok := h.jobs.Get(started.JobID)
	if !ok {
		t.Fatal("deploy job not found")
	}
	for i := 0; job.Snapshot().Status == deploy.StatusRunning && i < 180; i++ {
		time.Sleep(time.Second)
	}
	if snap := job.Snapshot(); snap.Status != deploy.StatusSuccess {
		t.Fatalf("redeploy failed:\n%s", strings.Join(snap.Log, ""))
	}
	client, err := sshexec.Dial(sshTarget(host, port, password))
	if err != nil {
		t.Fatal(err)
	}
	ls, _, _ := client.Output(sshexec.AsRoot("docker compose ls --all --format json"))
	client.Close()
	var projects []struct{ Name, Status string }
	json.Unmarshal([]byte(ls), &projects)
	shops := 0
	for _, p := range projects {
		if p.Name == "shop" {
			shops++
			if p.Status != "running(1)" {
				t.Errorf("shop status after redeploy = %q, want running(1)", p.Status)
			}
		}
	}
	if shops != 1 || len(projects) != 2 {
		t.Errorf("after redeploying the imported app, projects = %+v; want shop once, plus custom-name", projects)
	}

	// The custom-named project can't be imported, only watched.
	if rec := call("/api/apps/import", map[string]any{"workingDir": "/srv/custom"}); rec.Code != http.StatusConflict {
		t.Errorf("importing custom-name: %d %s", rec.Code, rec.Body)
	}
	rec = call("/api/apps/watch", map[string]any{"workingDir": "/srv/custom"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("watch: %d %s", rec.Code, rec.Body)
	}
	var watched Service
	json.Unmarshal(rec.Body.Bytes(), &watched)
	if !watched.Connected() || watched.ComposeProject != "custom-name" || watched.ComposeFiles != "/srv/custom/compose.yaml" || !strings.Contains(watched.Manifest, "services:") {
		t.Errorf("watched = %+v", watched)
	}

	// A second scan knows both, and won't import or watch them again.
	found = detect()
	if p := found["/opt/shop"]; p.AppID != app.ID || p.Importable || p.Watchable {
		t.Errorf("shop after import = %+v", p)
	}
	if p := found["/srv/custom"]; p.ServiceID != watched.ID || p.Watchable {
		t.Errorf("custom after watch = %+v", p)
	}
	if rec := call("/api/apps/watch", map[string]any{"workingDir": "/srv/custom"}); rec.Code != http.StatusConflict {
		t.Errorf("watching twice: %d", rec.Code)
	}

	// The watched project's health check finds its containers by label.
	health, detail, _ := CheckHealth(sshTarget(host, port, password), &watched)
	if health != "degraded" && health != "healthy" || !strings.Contains(detail, "running") {
		t.Errorf("watched health = %s %q", health, detail)
	}
}

func sshTarget(host string, port int, password string) sshexec.Target {
	return sshexec.Target{Host: host, Port: port, Username: e2eUser(), Password: password}
}

// e2eUser is the SSH user end-to-end tests log in as: E2E_SSH_USER, else
// root — set it to a non-root user whose sudo asks for a password to test
// that path.
func e2eUser() string {
	if u := os.Getenv("E2E_SSH_USER"); u != "" {
		return u
	}
	return "root"
}

// TestEnvFilesEndToEnd: a project whose compose reads .env (for ${TAG}) and an
// env_file is listed with both; editing the env_file and applying changes the
// running container's environment, keeps a backup, and refuses a save based
// on a version that has since changed. Same throwaway systems as above.
func TestEnvFilesEndToEnd(t *testing.T) {
	dbURL, target, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if dbURL == "" || target == "" || password == "" {
		t.Skip("set TEST_DATABASE_URL, E2E_SSH_TARGET and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	h := NewHandler(NewStore(pool), NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), deploy.NewStore(), box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)

	c, err := sshexec.Dial(sshTarget(host, port, password))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	const dir = "/opt/envapp"
	setup := `mkdir -p /opt/envapp && cd /opt/envapp && printf 'services:\n  web:\n    image: nginx:${TAG}\n    env_file: app.env\n' > docker-compose.yml && printf 'TAG=alpine\n' > .env && printf 'GREETING=hello\n' > app.env && docker compose up -d`
	if _, errOut, err := c.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(setup))); err != nil {
		t.Fatalf("setup: %v %s", err, errOut)
	}
	containerEnv := func() string {
		out, _, _ := c.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote("cd /opt/envapp && docker compose exec -T web printenv GREETING")))
		return strings.TrimSpace(out)
	}

	call := func(path string, extra map[string]any) *httptest.ResponseRecorder {
		body := map[string]any{"host": host, "port": port, "username": e2eUser(), "password": password, "workingDir": dir}
		for k, v := range extra {
			body[k] = v
		}
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", path, &buf))
		return rec
	}

	rec := call("/api/apps/detect", nil)
	var list []DetectedProject
	json.Unmarshal(rec.Body.Bytes(), &list)
	var found *DetectedProject
	for i := range list {
		if list[i].WorkingDir == dir {
			found = &list[i]
		}
	}
	if found == nil || len(found.EnvFiles) != 2 || found.EnvFiles[0].Label != ".env" || found.EnvFiles[1].Path != dir+"/app.env" || !found.EnvFiles[1].Exists {
		t.Fatalf("env files = %+v", found)
	}

	rec = call("/api/apps/envfile", map[string]any{"path": dir + "/app.env"})
	var read envFileContent
	json.Unmarshal(rec.Body.Bytes(), &read)
	if rec.Code != http.StatusOK || read.Content != "GREETING=hello\n" || read.SHA == "" {
		t.Fatalf("read: %d %s", rec.Code, rec.Body)
	}
	if got := containerEnv(); got != "hello" {
		t.Fatalf("container GREETING before = %q", got)
	}

	rec = call("/api/apps/envfile/save", map[string]any{"path": dir + "/app.env", "content": "GREETING=world\n", "baseSha": read.SHA, "apply": true})
	var saved envFileSaved
	json.Unmarshal(rec.Body.Bytes(), &saved)
	if rec.Code != http.StatusOK || !saved.OK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if got := containerEnv(); got != "world" {
		t.Errorf("container GREETING after apply = %q, want world\n%s", got, saved.Log)
	}
	if !strings.Contains(saved.Log, "Previous version kept at /var/backups/devops-panel/opt/envapp/app.env.") {
		t.Errorf("no backup in log:\n%s", saved.Log)
	}

	// The hash a save returns is the base for the next one.
	if rec := call("/api/apps/envfile/save", map[string]any{"path": dir + "/app.env", "content": "GREETING=world\n", "baseSha": saved.SHA}); rec.Code != http.StatusOK {
		t.Errorf("save on the returned hash: %d %s", rec.Code, rec.Body)
	}
	// The same stale base again: the file has changed since, so it's refused.
	if rec := call("/api/apps/envfile/save", map[string]any{"path": dir + "/app.env", "content": "GREETING=x\n", "baseSha": read.SHA}); rec.Code != http.StatusConflict {
		t.Errorf("stale save: %d %s", rec.Code, rec.Body)
	}
	// A file the compose doesn't use can't be edited this way.
	if rec := call("/api/apps/envfile", map[string]any{"path": "/etc/passwd"}); rec.Code != http.StatusBadRequest {
		t.Errorf("reading a non-env file: %d", rec.Code)
	}
}
