// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

// TestMonitorOneConnectionPerServerEndToEnd: a monitor round checks every
// service on a server over one SSH login (counted in sshd's own log, which
// E2E_SSHD_LOG names on the server), saves a health for each, and heals a
// service whose saved password is outdated from another on the same login.
func TestMonitorOneConnectionPerServerEndToEnd(t *testing.T) {
	dbURL, target, password, sshdLog := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD"), os.Getenv("E2E_SSHD_LOG")
	if dbURL == "" || target == "" || password == "" || sshdLog == "" {
		t.Skip("set TEST_DATABASE_URL, E2E_SSH_TARGET, E2E_SSH_PASSWORD and E2E_SSHD_LOG to throwaway systems to run")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	store := NewStore(pool)
	h := NewHandler(store, NewAppStore(pool), NewLogStore(pool), NewSnapshotStore(pool), deploy.NewStore(), box)
	store.onCreate = nil // only the monitor round below may log in
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	user := e2eUser()

	good, _ := box.Encrypt(password)
	stale, _ := box.Encrypt("an old password")
	for i, dir := range []string{"/opt/m1", "/opt/m2", "/opt/m3", "/opt/m4"} {
		s := &Service{Name: "m" + strconv.Itoa(i), Type: "docker", Host: host, Port: port, Username: user, DeployDir: dir, Status: StatusPending, EncryptedPassword: good}
		if err := store.Create(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	old := &Service{Name: "stale", Type: "docker", Host: host, Port: port, Username: user, DeployDir: "/opt/m5", Status: StatusPending, EncryptedPassword: stale}
	if err := store.Create(ctx, old); err != nil {
		t.Fatal(err)
	}

	c, err := sshexec.Dial(sshTarget(host, port, password))
	if err != nil {
		t.Fatal(err)
	}
	logins := func() int {
		out, _, _ := c.Output(sshexec.AsRoot("grep -c 'Accepted password' " + sshexec.ShellQuote(sshdLog)))
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		return n
	}
	// The greps run over c, opened before counting, so they add no logins.
	before := logins()
	h.monitorOnce(ctx)
	used := logins() - before
	c.Close()

	// One login for the four services sharing the good password. The stale
	// one is refused (not an accepted login), then healed: one login to
	// verify the sibling's password, one to check it. Once healed, it joins
	// the others — the next round is a single login for all five.
	if used != 3 {
		t.Errorf("a monitor round over 5 services on one server used %d SSH logins; want 3", used)
	}
	before = func() int { c2, _ := sshexec.Dial(sshTarget(host, port, password)); c = c2; return logins() }()
	h.monitorOnce(ctx)
	if used := logins() - before; used != 1 {
		t.Errorf("once healed, a round used %d SSH logins; want 1", used)
	}
	c.Close()
	all, _ := store.List(ctx)
	for _, s := range all {
		if s.Health != "not deployed" {
			t.Errorf("%s: health %s (%s)", s.Name, s.Health, s.HealthDetail)
		}
	}
	if s, _ := store.Get(ctx, old.ID); s.EncryptedPassword != good {
		t.Error("the stale saved password wasn't replaced by the working one")
	}
}
