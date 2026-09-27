// Package db opens the Postgres connection pool and applies this backend's
// schema on startup. No migration framework — the schema is small enough
// that plain idempotent DDL is the simpler, correct choice.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := migrate(ctx, pool); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return pool, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS services (
	id text PRIMARY KEY,
	name text NOT NULL,
	type text NOT NULL,
	host text NOT NULL,
	port integer NOT NULL,
	username text NOT NULL,
	deploy_dir text NOT NULL,
	manifest text NOT NULL,
	status text NOT NULL,
	last_job_id text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	token text PRIMARY KEY,
	username text NOT NULL,
	expires_at timestamptz NOT NULL
);

ALTER TABLE services ADD COLUMN IF NOT EXISTS health text NOT NULL DEFAULT 'unknown';
ALTER TABLE services ADD COLUMN IF NOT EXISTS health_detail text NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS last_synced_at timestamptz;

-- AES-256-GCM ciphertext (base64), never plaintext. Empty means no saved password yet.
ALTER TABLE services ADD COLUMN IF NOT EXISTS encrypted_password text NOT NULL DEFAULT '';

-- Saved history of every action run against a service (deploy, start, stop,
-- down, destroy, sync) and its output. Deliberately no foreign key to
-- services: the record of a Delete should outlive the service it deleted.
CREATE TABLE IF NOT EXISTS service_logs (
	id text PRIMARY KEY,
	service_id text NOT NULL,
	service_name text NOT NULL,
	host text NOT NULL,
	action text NOT NULL,
	status text NOT NULL,
	log text NOT NULL DEFAULT '',
	started_at timestamptz NOT NULL,
	finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS service_logs_by_service ON service_logs (service_id, started_at DESC);

-- "panel": deployed and fully managed by the panel. "connected": an existing
-- stack the panel only inspects and configures; it never deploys, stops or
-- removes anything of it. Container pins which container a connected
-- service is; compose_project groups its stack's containers for Sync.
ALTER TABLE services ADD COLUMN IF NOT EXISTS origin text NOT NULL DEFAULT 'panel';
ALTER TABLE services ADD COLUMN IF NOT EXISTS container text NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS compose_project text NOT NULL DEFAULT '';

-- Connected stacks: their compose files (comma-separated, any name), and
-- the health streak that unlocks Start/Stop/Down. healthy_since is when the
-- current unbroken run of healthy checks began; controls_unlocked_at is set
-- once that run first reaches five minutes, and stays set.
ALTER TABLE services ADD COLUMN IF NOT EXISTS compose_files text NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS healthy_since timestamptz;
ALTER TABLE services ADD COLUMN IF NOT EXISTS controls_unlocked_at timestamptz;

-- The last successful Prometheus reading of each kind ("inspect" for Rules &
-- targets, "status" for Status), shown instantly while a fresh one loads.
CREATE TABLE IF NOT EXISTS prometheus_snapshots (
	service_id text NOT NULL,
	kind text NOT NULL,
	data jsonb NOT NULL,
	fetched_at timestamptz NOT NULL,
	PRIMARY KEY (service_id, kind)
);
`

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}
