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

// TestScanAndAddEndToEnd: scanning adds every compose project with
// containers — /opt/shop as an app, /srv/custom (custom project name, so
// not importable) watched — skips /home/app (files only), and a second scan
// adds nothing twice. Same throwaway systems and projects as
// TestDetectImportWatchEndToEnd.
func TestScanAndAddEndToEnd(t *testing.T) {
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

	scan := func(pw string) scanAddResponse {
		t.Helper()
		var buf bytes.Buffer
		json.NewEncoder(&buf).Encode(map[string]any{"host": host, "port": port, "username": e2eUser(), "password": pw})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/apps/detect/add", &buf))
		if rec.Code != http.StatusOK {
			t.Fatalf("scan: %d %s", rec.Code, rec.Body)
		}
		var resp scanAddResponse
		json.Unmarshal(rec.Body.Bytes(), &resp)
		return resp
	}

	first := scan(password)
	got := map[string]AddedProject{}
	for _, a := range first.Added {
		got[a.WorkingDir] = a
	}
	if a := got["/opt/shop"]; a.As != "app" || a.Error != "" || !strings.HasPrefix(a.Href, "/deploy/compose/") {
		t.Errorf("shop: %+v", a)
	}
	if a := got["/srv/custom"]; a.As != "watched" || a.Error != "" || !strings.HasPrefix(a.Href, "/deploy/compose/watch/") {
		t.Errorf("custom: %+v", a)
	}
	if _, ok := got["/home/app"]; ok || len(first.Added) != 2 {
		t.Errorf("added = %+v; want only the two with containers", first.Added)
	}
	for _, p := range first.Projects {
		inPanel := p.AppID != "" || p.ServiceID != ""
		if inPanel != (p.WorkingDir != "/home/app") {
			t.Errorf("after the scan, %s in panel = %v", p.WorkingDir, inPanel)
		}
	}

	// Again, with the saved password: nothing new to add.
	if second := scan(""); len(second.Added) != 0 {
		t.Errorf("second scan added %+v", second.Added)
	}
	apps, _ := h.apps.List(ctx)
	if len(apps) != 1 || apps[0].Name != "shop" {
		t.Errorf("apps = %d", len(apps))
	}
}
