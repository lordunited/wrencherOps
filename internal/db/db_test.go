// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package db

import (
	"context"
	"os"
	"testing"
)

// The schema is applied on every start: applying it again must work and
// keep everything (TEST_DATABASE_URL, a throwaway Postgres).
func TestMigrateTwiceE2E(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("needs TEST_DATABASE_URL to a throwaway Postgres")
	}
	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate(ctx, pool); err != nil {
		t.Fatalf("migrating a second time: %v", err)
	}
	for _, table := range []string{"services", "sessions", "compose_apps", "servers", "server_alerts", "ansible_runs",
		"tf_workspaces", "tf_runs", "panel_users", "panel_groups", "panel_group_members", "panel_grants"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", "public."+table).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s is missing (%v)", table, err)
		}
	}
	var cols int
	pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'tf_runs' AND column_name IN ('started_by', 'options', 'checks')`).Scan(&cols)
	if cols != 3 {
		t.Errorf("tf_runs has %d of the 3 newer columns", cols)
	}
}

func TestOpenRefusesABadURL(t *testing.T) {
	if _, err := Open(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1"); err == nil {
		t.Error("opened a database that isn't there")
	}
}
