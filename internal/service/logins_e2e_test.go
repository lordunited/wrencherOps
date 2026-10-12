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

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/secretbox"
)

// TestSavedLoginsEndToEnd checks SSH passwords are saved per server login,
// and only once they've logged in, against a throwaway Postgres
// (TEST_DATABASE_URL) and SSH server (E2E_SSH_TARGET, E2E_SSH_PASSWORD).
func TestSavedLoginsEndToEnd(t *testing.T) {
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
	user := e2eUser()

	do := func(path string, body map[string]any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", path, &buf))
		return rec
	}
	newService := func(name string) *Service {
		s := &Service{Name: name, Type: "docker", Host: host, Port: port, Username: user, DeployDir: "/opt/" + name, Status: StatusPending}
		if err := h.services.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	saved := func(id string) string {
		s, _ := h.services.Get(ctx, id)
		return s.EncryptedPassword
	}

	a, b := newService("login-a"), newService("login-b")
	if a.EncryptedPassword != "" || b.EncryptedPassword != "" {
		t.Fatal("nothing should be saved yet")
	}

	// A wrong password is refused and saves nothing.
	if rec := do("/api/services/"+a.ID+"/sync", map[string]any{"password": "wrong"}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("wrong password: %d %s", rec.Code, rec.Body)
	}
	if saved(a.ID) != "" {
		t.Error("a refused password was saved")
	}

	// The right one is saved — for every service on that login.
	if rec := do("/api/services/"+a.ID+"/sync", map[string]any{"password": password}); rec.Code != http.StatusOK {
		t.Fatalf("right password: %d %s", rec.Code, rec.Body)
	}
	good := saved(a.ID)
	if good == "" || saved(b.ID) != good {
		t.Fatalf("saved on a=%v, on b=%v; want both", good != "", saved(b.ID) != "")
	}
	if rec := do("/api/services/"+b.ID+"/sync", map[string]any{}); rec.Code != http.StatusOK {
		t.Errorf("b with the saved password: %d %s", rec.Code, rec.Body)
	}

	// A typo later doesn't replace the saved one.
	if rec := do("/api/services/"+a.ID+"/sync", map[string]any{"password": "typo"}); rec.Code != http.StatusBadRequest {
		t.Errorf("typo: %d %s", rec.Code, rec.Body)
	}
	if saved(a.ID) != good || saved(b.ID) != good {
		t.Error("a refused password replaced the saved one")
	}

	// A new service on the same server starts with it.
	if c := newService("login-c"); c.EncryptedPassword != good {
		t.Error("a new service on a saved server didn't get its password")
	}

	// Detect with no password typed uses the saved one.
	if rec := do("/api/apps/detect", map[string]any{"host": host, "port": port, "username": user}); rec.Code != http.StatusOK {
		t.Errorf("detect with the saved password: %d %s", rec.Code, rec.Body)
	}
	// An app server added with a wrong password is refused.
	rec := do("/api/apps", map[string]any{"name": "pw-app", "manifest": "services: {}\n",
		"servers": []map[string]any{{"host": host, "port": port, "username": user, "password": "nope"}}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "refused") {
		t.Errorf("app with a wrong password: %d %s", rec.Code, rec.Body)
	}
}
