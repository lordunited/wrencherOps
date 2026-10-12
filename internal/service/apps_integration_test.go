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
	"strings"
	"testing"

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/secretbox"
)

// TestAppsAgainstPostgres drives the custom compose app API end to end
// against a real database. It needs a throwaway Postgres — never point it
// at real data — given as TEST_DATABASE_URL; without one it's skipped.
func TestAppsAgainstPostgres(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a throwaway Postgres to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	services := NewStore(pool)
	h := NewHandler(services, NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), deploy.NewStore(), box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}
	server := func(host, password string) map[string]any {
		return map[string]any{"host": host, "username": "deploy", "password": password}
	}

	// Another stack already lives in /opt/web on 10.0.0.2.
	if err := services.Create(ctx, &Service{Name: "other", Type: "docker", Host: "10.0.0.2", Port: 22, Username: "u", DeployDir: "/opt/web", Manifest: "x", Status: StatusPending}); err != nil {
		t.Fatal(err)
	}
	if rec := do("POST", "/api/apps", map[string]any{"name": "web", "deployDir": "/opt/web", "manifest": "services: {}",
		"servers": []any{server("10.0.0.1", ""), server("10.0.0.2", "")}}); rec.Code != http.StatusConflict {
		t.Fatalf("creating into another stack's folder: %d %s", rec.Code, rec.Body)
	}
	if rec := do("POST", "/api/apps", map[string]any{"name": "dup", "manifest": "services: {}",
		"servers": []any{server("10.0.0.1", ""), server("10.0.0.1", "")}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("the same server twice: %d %s", rec.Code, rec.Body)
	}

	rec := do("POST", "/api/apps", map[string]any{"name": "Web App", "manifest": "services:\n  web:\n    image: nginx\n",
		"servers": []any{server("10.0.0.1", ""), server("10.0.0.3", "")}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var app App
	json.Unmarshal(rec.Body.Bytes(), &app)
	if app.DeployDir != "/opt/web-app" || len(app.Servers) != 2 {
		t.Fatalf("app = %+v", app)
	}
	if app.Servers[0].HasSavedPassword || app.Servers[1].HasSavedPassword {
		t.Errorf("saved passwords = %v, %v; none was given", app.Servers[0].HasSavedPassword, app.Servers[1].HasSavedPassword)
	}
	// A password is saved only once it has logged in (logins_e2e_test
	// covers that against a real SSH server): one for a server that can't
	// be reached isn't kept, and the app is still created.
	rec = do("POST", "/api/apps", map[string]any{"name": "unverified", "manifest": "services: {}",
		"servers": []any{map[string]any{"host": "127.0.0.1", "port": 1, "username": "deploy", "password": "pw"}}})
	var unverified App
	json.Unmarshal(rec.Body.Bytes(), &unverified)
	if rec.Code != http.StatusCreated || len(unverified.Servers) != 1 || unverified.Servers[0].HasSavedPassword {
		t.Errorf("an unverifiable password: %d %s", rec.Code, rec.Body)
	}
	for _, s := range unverified.Servers {
		do("DELETE", "/api/services/"+s.ID, nil)
	}
	do("DELETE", "/api/apps/"+unverified.ID, nil)
	for _, s := range app.Servers {
		if s.AppID != app.ID || s.Type != "docker" || s.DeployDir != app.DeployDir || s.Manifest != app.Manifest {
			t.Errorf("server %s doesn't mirror its app: %+v", s.Host, s)
		}
	}

	// Saving the app's compose updates every server's copy.
	rec = do("PUT", "/api/apps/"+app.ID, map[string]any{"name": "Web", "manifest": "services:\n  web:\n    image: nginx:alpine\n"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	for _, s := range app.Servers {
		got, _ := services.Get(ctx, s.ID)
		if got.Name != "Web" || got.Manifest != "services:\n  web:\n    image: nginx:alpine\n" {
			t.Errorf("server %s not updated: name %q manifest %q", s.Host, got.Name, got.Manifest)
		}
	}
	// ...and a server can't change its own copy.
	if rec := do("PUT", "/api/services/"+app.Servers[0].ID, map[string]any{"manifest": "something else"}); rec.Code != http.StatusBadRequest {
		t.Errorf("editing a server's own compose: %d %s", rec.Code, rec.Body)
	}

	// .env and project files are stored, read back, and analyzed.
	rec = do("PUT", "/api/apps/"+app.ID, map[string]any{
		"manifest": "services:\n  web:\n    image: nginx:${TAG}\n    volumes:\n      - ./conf/app.conf:/etc/app.conf\n      - data:/data\nvolumes:\n  data:\n",
		"env":      "TAG=alpine\n",
		"files":    []map[string]string{{"path": "conf/app.conf", "content": "x=1\n"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update with files: %d %s", rec.Code, rec.Body)
	}
	rec = do("GET", "/api/apps/"+app.ID, nil)
	var withFiles App
	json.Unmarshal(rec.Body.Bytes(), &withFiles)
	if withFiles.Env != "TAG=alpine\n" || len(withFiles.Files) != 1 || withFiles.Files[0].Path != "conf/app.conf" {
		t.Errorf("env/files not stored: env %q files %+v", withFiles.Env, withFiles.Files)
	}
	if a := withFiles.Analysis; a == nil || len(a.Mounts) != 2 || !a.Mounts[0].InProject || a.Mounts[1].Volume != "web-app_data" || len(a.Warnings) != 0 {
		t.Errorf("analysis = %+v", withFiles.Analysis)
	}
	for name, files := range map[string][]map[string]string{
		"path escaping the folder": {{"path": "../etc/x", "content": "x"}},
		".env as a file":           {{"path": ".env", "content": "A=1"}},
		"binary content":           {{"path": "a.bin", "content": "a\x00b"}},
	} {
		if rec := do("PUT", "/api/apps/"+app.ID, map[string]any{"files": files}); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}

	// A known server can be added by reusing its saved login.
	known := &Service{Name: "known", Type: "docker", Host: "10.0.0.9", Port: 2222, Username: "ops", DeployDir: "/opt/known", Manifest: "x", Status: StatusPending}
	known.EncryptedPassword, _ = box.Encrypt("saved-pw")
	if err := services.Create(ctx, known); err != nil {
		t.Fatal(err)
	}
	rec = do("POST", "/api/apps", map[string]any{"name": "reuse", "manifest": "services: {}", "servers": []any{map[string]any{"copyLoginFrom": known.ID}}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with a copied login: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), known.EncryptedPassword) {
		t.Error("the response carries the encrypted password")
	}
	var reused App
	json.Unmarshal(rec.Body.Bytes(), &reused)
	got, _ := services.Get(ctx, reused.Servers[0].ID)
	if got.Host != "10.0.0.9" || got.Port != 2222 || got.Username != "ops" || !got.HasSavedPassword {
		t.Errorf("copied server = %+v", got)
	}
	if pw, _ := box.Decrypt(got.EncryptedPassword); pw != "saved-pw" {
		t.Errorf("copied password decrypts to %q", pw)
	}
	if rec := do("POST", "/api/apps/"+app.ID+"/servers", map[string]any{"copyLoginFrom": "no-such-service"}); rec.Code != http.StatusBadRequest {
		t.Errorf("copying from an unknown service: %d %s", rec.Code, rec.Body)
	}
	do("DELETE", "/api/services/"+got.ID, nil)
	do("DELETE", "/api/apps/"+reused.ID, nil)

	if rec := do("POST", "/api/apps/"+app.ID+"/servers", server("10.0.0.4", "")); rec.Code != http.StatusCreated {
		t.Fatalf("add server: %d %s", rec.Code, rec.Body)
	}
	if rec := do("POST", "/api/apps/"+app.ID+"/servers", server("10.0.0.1", "")); rec.Code != http.StatusConflict {
		t.Errorf("adding a server already in the app: %d %s", rec.Code, rec.Body)
	}

	rec = do("GET", "/api/apps/"+app.ID, nil)
	json.Unmarshal(rec.Body.Bytes(), &app)
	if len(app.Servers) != 3 {
		t.Fatalf("servers = %d, want 3", len(app.Servers))
	}
	if rec := do("DELETE", "/api/apps/"+app.ID, nil); rec.Code != http.StatusConflict {
		t.Errorf("deleting an app that still has servers: %d", rec.Code)
	}
	for _, s := range app.Servers {
		do("DELETE", "/api/services/"+s.ID, nil)
	}
	if rec := do("DELETE", "/api/apps/"+app.ID, nil); rec.Code != http.StatusNoContent {
		t.Errorf("deleting an emptied app: %d %s", rec.Code, rec.Body)
	}
}
