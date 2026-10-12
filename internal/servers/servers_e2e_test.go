// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

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
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

// TestServersEndToEnd reads a real server's state as a non-root sudo user
// and runs alerts through their life, against a throwaway Postgres
// (TEST_DATABASE_URL) and server (E2E_SSH_TARGET, E2E_SSH_USER,
// E2E_SSH_PASSWORD) that logs sshd to syslog (/var/log/messages).
func TestServersEndToEnd(t *testing.T) {
	dbURL, target, user, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_USER"), os.Getenv("E2E_SSH_PASSWORD")
	if dbURL == "" || target == "" || user == "" || password == "" {
		t.Skip("set TEST_DATABASE_URL, E2E_SSH_TARGET, E2E_SSH_USER and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	store := NewStore(pool)
	h := NewHandler(store, box)
	if err := store.SeedRules(ctx); err != nil {
		t.Fatal(err)
	}
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

	// Three failed logins, as an attacker would make.
	for i := 0; i < 3; i++ {
		if _, err := sshexec.Dial(sshexec.Target{Host: host, Port: port, Username: "root", Password: "guess" + strconv.Itoa(i)}); err == nil {
			t.Fatal("a wrong password logged in")
		}
	}

	// A wrong password is refused, nothing saved.
	if rec := do("POST", "/api/servers", map[string]any{"host": host, "port": port, "username": user, "password": "nope"}); rec.Code != http.StatusBadRequest {
		t.Errorf("wrong password: %d %s", rec.Code, rec.Body)
	}
	rec := do("POST", "/api/servers", map[string]any{"name": "test box", "host": host, "port": port, "username": user, "password": password})
	var srv Server
	json.Unmarshal(rec.Body.Bytes(), &srv)
	if rec.Code != http.StatusCreated || srv.Snapshot == nil {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	s := srv.Snapshot
	t.Logf("host %s · %s · %s · %d CPUs · load %.2f · mem %.0f%% · %d disks · auth from %q · failed %d/h · kernel errors %d (%d in the hour) · %d established · notes %v",
		s.Hostname, s.OS, s.Kernel, s.CPUs, s.Load5, s.Memory.UsedPercent, len(s.Disks), s.AuthSource, s.Failed.LastHour, s.KernelErrors.Total, s.KernelErrors.LastHour, s.Connections.Established, s.Notes)
	if s.Hostname == "" || s.CPUs < 1 || s.Memory.TotalBytes == 0 || len(s.Disks) == 0 {
		t.Errorf("system/disks not read: %+v", s)
	}
	if s.AuthSource != "/var/log/messages" || s.Failed.LastHour < 4 || s.Failed.TopUsers[0].Key != "root" {
		// 3 for root + 1 for the wrong password above.
		t.Errorf("failed logins: source %q, %+v", s.AuthSource, s.Failed)
	}
	if len(s.Logins) == 0 || s.Logins[0].User != user || !s.Logins[0].Panel || s.PanelAddress == "" {
		t.Errorf("successful logins (the panel's own, marked): address %q %+v", s.PanelAddress, s.Logins)
	}
	if !s.Connections.Readable || s.Connections.Established < 1 || !s.KernelErrors.Readable {
		t.Errorf("connections %+v kernel %+v", s.Connections, s.KernelErrors.Readable)
	}

	// Failed-logins alert: lower the threshold, check, it opens.
	rule, _ := store.Rule(ctx, "builtin-failed-logins")
	rule.Threshold = 2
	if rec := do("PUT", "/api/servers/rules/"+rule.ID, rule); rec.Code != http.StatusOK {
		t.Fatalf("update rule: %d %s", rec.Code, rec.Body)
	}
	rec = do("POST", "/api/servers/"+srv.ID+"/check", nil)
	json.Unmarshal(rec.Body.Bytes(), &srv)
	var failedAlert *Alert
	for i, a := range srv.Alerts {
		if a.RuleID == rule.ID {
			failedAlert = &srv.Alerts[i]
		}
	}
	if failedAlert == nil || !strings.Contains(failedAlert.Message, "failed SSH logins in the last hour") {
		t.Fatalf("no failed-logins alert: %+v", srv.Alerts)
	}
	t.Logf("alert: %s", failedAlert.Message)
	// Checking again keeps the same alert open, not a second one.
	rec = do("POST", "/api/servers/"+srv.ID+"/check", nil)
	json.Unmarshal(rec.Body.Bytes(), &srv)
	n := 0
	for _, a := range srv.Alerts {
		if a.RuleID == rule.ID {
			n++
			if a.ID != failedAlert.ID {
				t.Error("a second check opened a new alert instead of keeping the open one")
			}
		}
	}
	if n != 1 {
		t.Errorf("%d failed-logins alerts open", n)
	}
	if rec := do("POST", "/api/servers/alerts/"+failedAlert.ID+"/ack", nil); rec.Code != http.StatusNoContent {
		t.Errorf("ack: %d", rec.Code)
	}
	// Raise the threshold: the condition clears and the alert resolves.
	rule.Threshold = 1000
	do("PUT", "/api/servers/rules/"+rule.ID, rule)
	do("POST", "/api/servers/"+srv.ID+"/check", nil)
	var all []Alert
	json.Unmarshal(do("GET", "/api/servers/alerts?all=1", nil).Body.Bytes(), &all)
	for _, a := range all {
		if a.ID == failedAlert.ID && (a.ResolvedAt == nil || a.AcknowledgedAt == nil || a.ServerName != "test box") {
			t.Errorf("after the condition cleared: %+v", a)
		}
	}

	// A rule just for this server: every disk at least 1% full alerts.
	rec = do("POST", "/api/servers/rules", map[string]any{"kind": "disk", "threshold": 1, "severity": "critical", "enabled": true, "serverId": srv.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add rule: %d %s", rec.Code, rec.Body)
	}
	rec = do("POST", "/api/servers/"+srv.ID+"/check", nil)
	json.Unmarshal(rec.Body.Bytes(), &srv)
	disks := 0
	for _, a := range srv.Alerts {
		if a.Kind == "disk" {
			disks++
		}
	}
	if disks != len(srv.Snapshot.Disks) {
		t.Errorf("%d disk alerts for %d disks", disks, len(srv.Snapshot.Disks))
	}
	if rec := do("DELETE", "/api/servers/rules/builtin-disk-95", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("deleting a built-in rule: %d", rec.Code)
	}

	// A server that can't be reached raises the unreachable alert.
	enc, _ := box.Encrypt(password)
	dead := &Server{Name: "dead", Host: host, Port: 1, Username: user, EncryptedPassword: enc}
	if err := store.Add(ctx, dead); err != nil {
		t.Fatal(err)
	}
	h.check(ctx, dead)
	json.Unmarshal(do("GET", "/api/servers/"+dead.ID, nil).Body.Bytes(), &srv)
	if len(srv.Alerts) != 1 || srv.Alerts[0].Kind != "unreachable" || srv.CheckError == "" {
		t.Errorf("unreachable server: error %q alerts %+v", srv.CheckError, srv.Alerts)
	}
	// Removing it resolves its alerts and drops it from the list.
	do("DELETE", "/api/servers/"+dead.ID, nil)
	var list []Server
	json.Unmarshal(do("GET", "/api/servers", nil).Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("after removing one, %d servers listed", len(list))
	}
}
