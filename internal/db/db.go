// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

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

-- Every currently-running container's CPU/memory/etc. from the last health
-- check, for every service type alike (not just Nginx/HAProxy, which also
-- show this in their own live-inspect tabs) — a JSON array of
-- dockerx.ContainerStats, empty until the first successful check.
ALTER TABLE services ADD COLUMN IF NOT EXISTS health_stats jsonb NOT NULL DEFAULT '[]';
-- Every port the stack publishes from the last health check, and whether
-- the panel could connect to it: a JSON array of service.PortStatus.
ALTER TABLE services ADD COLUMN IF NOT EXISTS health_ports jsonb NOT NULL DEFAULT '[]';
-- Every container of the stack at the last check, and its state: running,
-- paused, stopped, a finished setup job, failing (service.ContainerState).
ALTER TABLE services ADD COLUMN IF NOT EXISTS health_containers jsonb NOT NULL DEFAULT '[]';

-- A custom compose app: one docker-compose.yml deployed, identically, to
-- several servers. Each server is an ordinary services row (type "docker")
-- whose app_id points here, so deploy, start/stop/down, sync, health and
-- history all work per server unchanged; the app holds the compose and
-- deploy folder they share, and keeps every server's copy in step.
CREATE TABLE IF NOT EXISTS compose_apps (
	id text PRIMARY KEY,
	name text NOT NULL,
	manifest text NOT NULL,
	deploy_dir text NOT NULL,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);
ALTER TABLE services ADD COLUMN IF NOT EXISTS app_id text NOT NULL DEFAULT '';
-- The app's .env, and its other files (configs, scripts, ...) as a JSON
-- array of {path, content}: all written into the deploy folder on deploy.
ALTER TABLE compose_apps ADD COLUMN IF NOT EXISTS env text NOT NULL DEFAULT '';
ALTER TABLE compose_apps ADD COLUMN IF NOT EXISTS files jsonb NOT NULL DEFAULT '[]';
-- The app's compose file name: docker-compose.yml for apps made in the
-- panel; an imported project keeps the name it already had on its server.
ALTER TABLE compose_apps ADD COLUMN IF NOT EXISTS compose_file text NOT NULL DEFAULT 'docker-compose.yml';
-- The template an app was created from ("signoz"), so its own tab can list
-- it; empty for apps written from scratch or imported.
ALTER TABLE compose_apps ADD COLUMN IF NOT EXISTS template text NOT NULL DEFAULT '';

-- SSH passwords are saved per server login (host, SSH port, user): a service
-- without one takes the one another service on the same login has. Only
-- fills empty ones (encrypted values copied as they are); never overwrites.
UPDATE services s SET encrypted_password = o.encrypted_password
FROM (
	SELECT DISTINCT ON (trim(host), port, trim(username)) trim(host) AS host, port, trim(username) AS username, encrypted_password
	FROM services WHERE encrypted_password <> ''
	ORDER BY trim(host), port, trim(username), updated_at DESC
) o
WHERE s.encrypted_password = '' AND trim(s.host) = o.host AND s.port = o.port AND trim(s.username) = o.username;

-- Servers watched for their own health (logins, disks, kernel errors,
-- connections, load): every server the panel deploys to is added once, and
-- more can be added by hand. Removing one only stops watching it (removed),
-- so it isn't added back from its services.
CREATE TABLE IF NOT EXISTS servers (
	id text PRIMARY KEY,
	name text NOT NULL,
	host text NOT NULL,
	port integer NOT NULL,
	username text NOT NULL,
	encrypted_password text NOT NULL DEFAULT '',
	snapshot jsonb,
	checked_at timestamptz,
	check_error text NOT NULL DEFAULT '',
	removed boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL,
	UNIQUE (host, port)
);

-- Alert rules for servers; server_id '' applies to every server.
CREATE TABLE IF NOT EXISTS server_alert_rules (
	id text PRIMARY KEY,
	kind text NOT NULL,
	name text NOT NULL,
	threshold double precision NOT NULL,
	severity text NOT NULL,
	enabled boolean NOT NULL DEFAULT true,
	server_id text NOT NULL DEFAULT '',
	builtin boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL
);

-- Alerts raised by those rules: open while resolved_at is null, at most one
-- open per server, rule and subject (a disk's mount point, or '').
CREATE TABLE IF NOT EXISTS server_alerts (
	id text PRIMARY KEY,
	server_id text NOT NULL,
	rule_id text NOT NULL,
	subject text NOT NULL DEFAULT '',
	severity text NOT NULL,
	message text NOT NULL,
	value double precision NOT NULL,
	started_at timestamptz NOT NULL,
	last_seen_at timestamptz NOT NULL,
	resolved_at timestamptz,
	acknowledged_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS server_alerts_open ON server_alerts (server_id, rule_id, subject) WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS server_alerts_recent ON server_alerts (started_at DESC);

-- Ansible playbooks kept in the panel: files is a JSON array of
-- {path, content}, always with the entry point playbook.yml.
CREATE TABLE IF NOT EXISTS ansible_playbooks (
	id text PRIMARY KEY,
	name text NOT NULL,
	description text NOT NULL DEFAULT '',
	files jsonb NOT NULL DEFAULT '[]',
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

-- Every run of a playbook: a dry run ("check") or for real ("apply"), on
-- the servers it targeted, with its whole log and per-host recap.
-- content_sha is the playbook's files and extra vars as run, so an apply can
-- require a passed dry run of exactly that.
CREATE TABLE IF NOT EXISTS ansible_runs (
	id text PRIMARY KEY,
	playbook_id text NOT NULL,
	playbook_name text NOT NULL,
	mode text NOT NULL,
	content_sha text NOT NULL,
	targets jsonb NOT NULL,
	extra_vars text NOT NULL DEFAULT '',
	status text NOT NULL,
	log text NOT NULL DEFAULT '',
	recap jsonb NOT NULL DEFAULT '[]',
	error text NOT NULL DEFAULT '',
	started_at timestamptz NOT NULL,
	finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS ansible_runs_by_playbook ON ansible_runs (playbook_id, started_at DESC);
-- The file ansible-playbook runs (an uploaded project's site.yml, ...).
ALTER TABLE ansible_playbooks ADD COLUMN IF NOT EXISTS entry text NOT NULL DEFAULT 'playbook.yml';

-- Terraform (OpenTofu) workspaces: their .tf files (a JSON array of
-- {path, content}) and, unless the code declares its own backend, their
-- state — encrypted, as state holds secrets in plain text.
CREATE TABLE IF NOT EXISTS tf_workspaces (
	id text PRIMARY KEY,
	name text NOT NULL,
	description text NOT NULL DEFAULT '',
	files jsonb NOT NULL DEFAULT '[]',
	state_enc text NOT NULL DEFAULT '',
	state_serial integer NOT NULL DEFAULT 0,
	state_updated_at timestamptz,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

-- A workspace's variables: Terraform input variables (kind 'terraform',
-- passed as TF_VAR_<key>) or environment variables (kind 'env', e.g. a
-- provider's credentials). Values are encrypted; sensitive ones are never
-- shown again and are masked in run logs.
CREATE TABLE IF NOT EXISTS tf_variables (
	workspace_id text NOT NULL,
	key text NOT NULL,
	kind text NOT NULL,
	value_enc text NOT NULL,
	sensitive boolean NOT NULL DEFAULT false,
	hcl boolean NOT NULL DEFAULT false,
	PRIMARY KEY (workspace_id, kind, key)
);

-- Every run: a plan (kind 'plan' or 'destroy-plan', with its saved plan
-- file, encrypted) or an apply of one plan run's file (applies_run_id).
CREATE TABLE IF NOT EXISTS tf_runs (
	id text PRIMARY KEY,
	workspace_id text NOT NULL,
	workspace_name text NOT NULL,
	kind text NOT NULL,
	status text NOT NULL,
	content_sha text NOT NULL,
	log text NOT NULL DEFAULT '',
	error text NOT NULL DEFAULT '',
	plan_enc text NOT NULL DEFAULT '',
	summary jsonb NOT NULL DEFAULT '{}',
	applies_run_id text NOT NULL DEFAULT '',
	applied_by_run_id text NOT NULL DEFAULT '',
	state_serial integer NOT NULL DEFAULT 0,
	started_at timestamptz NOT NULL,
	finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS tf_runs_by_workspace ON tf_runs (workspace_id, started_at DESC);
-- Who started a run, the options it was planned with (resources to
-- replace), and the standard's findings when it was planned.
ALTER TABLE tf_runs ADD COLUMN IF NOT EXISTS started_by text NOT NULL DEFAULT '';
ALTER TABLE tf_runs ADD COLUMN IF NOT EXISTS options jsonb NOT NULL DEFAULT '{}';
ALTER TABLE tf_runs ADD COLUMN IF NOT EXISTS checks jsonb NOT NULL DEFAULT '[]';

-- Every state a run left behind, newest first: to see what changed, and to
-- go back to.
CREATE TABLE IF NOT EXISTS tf_state_versions (
	workspace_id text NOT NULL,
	serial integer NOT NULL,
	state_enc text NOT NULL,
	run_id text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL,
	PRIMARY KEY (workspace_id, serial)
);

-- Panel users besides AUTH_USERS (who are built-in admins), each with a
-- role: admin, writer or viewer.
CREATE TABLE IF NOT EXISTS panel_users (
	username text PRIMARY KEY,
	password_hash text NOT NULL,
	role text NOT NULL,
	disabled boolean NOT NULL DEFAULT false,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_groups (
	id text PRIMARY KEY,
	name text NOT NULL UNIQUE,
	description text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS panel_group_members (
	group_id text NOT NULL REFERENCES panel_groups (id) ON DELETE CASCADE,
	username text NOT NULL REFERENCES panel_users (username) ON DELETE CASCADE,
	PRIMARY KEY (group_id, username)
);

-- What a user or a group can do in each tab of the panel: view or write.
CREATE TABLE IF NOT EXISTS panel_grants (
	subject_kind text NOT NULL, -- 'user' or 'group'
	subject text NOT NULL,      -- the username, or the group's id
	tab text NOT NULL,
	level text NOT NULL,
	PRIMARY KEY (subject_kind, subject, tab)
);

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
