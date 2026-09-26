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
`

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}
