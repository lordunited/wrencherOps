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
)

// TestHealthWithoutSyncEndToEnd: services get a real health on their own —
// projects added by a scan, and a new app server — with no Sync pressed, and
// one never deployed reads "not deployed", not "unknown" or "unreachable".
// Same throwaway systems and projects as TestDetectImportWatchEndToEnd.
func TestHealthWithoutSyncEndToEnd(t *testing.T) {
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
	post := func(path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", path, &buf))
		return rec
	}

	if rec := post("/api/apps/detect/add", map[string]any{"host": host, "port": port, "username": e2eUser(), "password": password}); rec.Code != http.StatusOK {
		t.Fatalf("scan: %d %s", rec.Code, rec.Body)
	}
	rec := post("/api/apps", map[string]any{"name": "never", "deployDir": "/opt/never", "manifest": "services:\n  web:\n    image: nginx:alpine\n",
		"servers": []map[string]any{{"host": host, "port": port, "username": e2eUser()}}})
	if rec.Code != http.StatusCreated {
		t.Fatalf("app: %d %s", rec.Code, rec.Body)
	}

	want := map[string]string{"/opt/shop": "healthy", "/srv/custom": "healthy", "/opt/never": "not deployed"}
	deadline := time.Now().Add(30 * time.Second)
	for {
		services, _ := h.services.List(ctx)
		got := map[string]string{}
		for _, s := range services {
			got[s.DeployDir] = s.Health + " (" + s.HealthDetail + ")"
		}
		done := true
		for dir, health := range want {
			if !strings.HasPrefix(got[dir], health+" ") {
				done = false
			}
		}
		if done {
			t.Logf("health without Sync: %v", got)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("after 30s, health = %v; want %v", got, want)
		}
		time.Sleep(time.Second)
	}
}
