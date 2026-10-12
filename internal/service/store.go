// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package service tracks deployable services (deployed or pending), each
// holding its own editable compose/manifest YAML. Persisted to Postgres —
// these are meaningful user records, not a cache.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/randid"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusDeploying Status = "deploying"
	StatusDeployed  Status = "deployed"
	StatusStarting  Status = "starting"
	StatusStopping  Status = "stopping"
	StatusStopped   Status = "stopped"
	StatusRemoving  Status = "removing"
	StatusDown      Status = "down"
	StatusFailed    Status = "failed"
	// StatusConnected is the standing status of a connected service: the
	// panel didn't deploy it and has no deploy state to report.
	StatusConnected Status = "connected"
)

const (
	// OriginPanel services were created and deployed by the panel.
	OriginPanel = "panel"
	// OriginConnected services are existing stacks the panel attached to.
	// The panel inspects and configures them but never deploys, stops or
	// removes anything of theirs.
	OriginConnected = "connected"
)

// Connected reports whether the panel only attached to this service.
func (s *Service) Connected() bool { return s.Origin == OriginConnected }

// ControlsUnlockAfter is how long a connected stack must stay healthy,
// without a single failed check, before the panel lets anyone start, stop
// or take it down.
const ControlsUnlockAfter = 5 * time.Minute

// ControlsUnlocked reports whether Start/Stop/Down may be used. Panel
// stacks always may; connected stacks only after their first
// ControlsUnlockAfter of continuous health.
func (s *Service) ControlsUnlocked() bool { return !s.Connected() || s.ControlsUnlockedAt != nil }

// ComposeDir is the folder holding the service's compose file. It's the
// deploy folder itself, except for SigNoz, whose installer generates the
// stack in a subfolder.
func (s *Service) ComposeDir() string {
	if s.Type == "signoz" {
		return strings.TrimRight(s.DeployDir, "/") + "/" + deploy.SigNozComposeDir
	}
	return s.DeployDir
}

// Project is how to address the service's Compose project on the server.
func (s *Service) Project() deploy.Project {
	if !s.Connected() {
		return deploy.Project{Dir: s.ComposeDir()}
	}
	p := deploy.Project{Dir: s.DeployDir, Name: s.ComposeProject}
	for _, f := range strings.Split(s.ComposeFiles, ",") {
		if f = strings.TrimSpace(f); f != "" {
			p.Files = append(p.Files, f)
		}
	}
	return p
}

// Service is a deployable unit: connection details for its target server
// plus the YAML that defines it — a docker-compose file for type "gitlab"
// or "docker", a Kubernetes manifest for type "kubernetes".
//
// Status reflects the last deploy/stop/down attempt. Health reflects the last sync
// (a live check of whether the thing is actually running right now) — the
// two are tracked separately since a service can be "deployed" (last
// deploy succeeded) but currently "unreachable" (it's since gone down).
//
// EncryptedPassword holds the SSH password, AES-GCM encrypted, so a deploy
// or sync can be re-run (including on an unattended schedule) without
// re-prompting. It's never serialized to JSON — HasSavedPassword is the
// safe derived signal the frontend gets instead.
type Service struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "gitlab" | "nexus3" | "docker" | "kubernetes"
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	DeployDir    string `json:"deployDir"`
	Manifest     string `json:"manifest"`
	Status       Status `json:"status"`
	LastJobID    string `json:"lastJobId,omitempty"`
	Health       string `json:"health"`       // "unknown" | "healthy" | "starting" | "degraded" | "unreachable"
	HealthDetail string `json:"healthDetail"` // e.g. "2/2 running", or an error message
	// HealthStats is every currently-running container's resource use from
	// the last check — empty until the first successful one, and for
	// Kubernetes services, which have no fixed notion of "the containers".
	HealthStats []dockerx.ContainerStats `json:"healthStats,omitempty"`
	// HealthPorts is every port the stack published at the last check,
	// with whether the panel could connect to it.
	HealthPorts []PortStatus `json:"healthPorts,omitempty"`
	// HealthContainers is every container of the stack at the last check,
	// with its state — paused and stopped ones included.
	HealthContainers  []ContainerState `json:"healthContainers,omitempty"`
	LastSyncedAt      *time.Time       `json:"lastSyncedAt,omitempty"`
	EncryptedPassword string           `json:"-"`
	HasSavedPassword  bool             `json:"hasSavedPassword"`
	Origin            string           `json:"origin"`         // "panel" or "connected"
	Container         string           `json:"container"`      // pinned container, for connected services
	ComposeProject    string           `json:"composeProject"` // for connected services
	ComposeFiles      string           `json:"composeFiles"`   // comma-separated, for connected services
	// HealthySince is when the current unbroken run of healthy checks
	// began; nil while unhealthy or not yet checked.
	HealthySince *time.Time `json:"healthySince,omitempty"`
	// ControlsUnlockedAt is when a connected stack first stayed healthy for
	// ControlsUnlockAfter; Start/Stop/Down are refused until then.
	ControlsUnlockedAt *time.Time `json:"controlsUnlockedAt,omitempty"`
	// AppID is the custom compose app this server belongs to, if any: its
	// compose and deploy folder are the app's, edited there, not here.
	AppID     string    `json:"appId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const serviceColumns = `id, name, type, host, port, username, deploy_dir, manifest, status,
	last_job_id, health, health_detail, last_synced_at, encrypted_password, created_at, updated_at,
	origin, container, compose_project, compose_files, healthy_since, controls_unlocked_at, health_stats, app_id, health_ports, health_containers`

func scanService(row pgx.Row) (*Service, error) {
	var svc Service
	var healthStats, healthPorts, healthContainers []byte
	err := row.Scan(&svc.ID, &svc.Name, &svc.Type, &svc.Host, &svc.Port, &svc.Username,
		&svc.DeployDir, &svc.Manifest, &svc.Status, &svc.LastJobID,
		&svc.Health, &svc.HealthDetail, &svc.LastSyncedAt, &svc.EncryptedPassword,
		&svc.CreatedAt, &svc.UpdatedAt, &svc.Origin, &svc.Container, &svc.ComposeProject,
		&svc.ComposeFiles, &svc.HealthySince, &svc.ControlsUnlockedAt, &healthStats, &svc.AppID, &healthPorts, &healthContainers)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(healthStats, &svc.HealthStats)
	json.Unmarshal(healthPorts, &svc.HealthPorts)
	json.Unmarshal(healthContainers, &svc.HealthContainers)
	svc.HasSavedPassword = svc.EncryptedPassword != ""
	return &svc, nil
}

// Store persists services in Postgres.
type Store struct {
	pool *pgxpool.Pool
	// onCreate, when set, is told about every service just created.
	onCreate func(id string)
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) List(ctx context.Context) ([]*Service, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+serviceColumns+" FROM services ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*Service, 0)
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, svc)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id string) (*Service, bool) {
	row := s.pool.QueryRow(ctx, "SELECT "+serviceColumns+" FROM services WHERE id = $1", id)
	svc, err := scanService(row)
	if err != nil {
		return nil, false
	}
	return svc, true
}

// Create assigns an ID and timestamps, then persists svc.
func (s *Store) Create(ctx context.Context, svc *Service) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	svc.ID = id
	svc.Host, svc.Username = strings.TrimSpace(svc.Host), strings.TrimSpace(svc.Username)
	if svc.EncryptedPassword == "" {
		// A new service on a server whose login is already saved gets it.
		if enc, err := s.LoginPassword(ctx, svc.Host, svc.Port, svc.Username); err == nil {
			svc.EncryptedPassword = enc
		}
	}
	svc.CreatedAt = time.Now()
	svc.UpdatedAt = time.Now()
	if svc.Health == "" {
		svc.Health = "unknown"
	}
	if svc.Origin == "" {
		svc.Origin = OriginPanel
	}

	healthStats, err := json.Marshal(svc.HealthStats)
	if err != nil {
		return err
	}
	healthPorts, err := json.Marshal(svc.HealthPorts)
	if err != nil {
		return err
	}
	healthContainers, err := json.Marshal(svc.HealthContainers)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO services (`+serviceColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`,
		svc.ID, svc.Name, svc.Type, svc.Host, svc.Port, svc.Username,
		svc.DeployDir, svc.Manifest, svc.Status, svc.LastJobID,
		svc.Health, svc.HealthDetail, svc.LastSyncedAt, svc.EncryptedPassword,
		svc.CreatedAt, svc.UpdatedAt, svc.Origin, svc.Container, svc.ComposeProject,
		svc.ComposeFiles, svc.HealthySince, svc.ControlsUnlockedAt, healthStats, svc.AppID, healthPorts, healthContainers)
	if err == nil && s.onCreate != nil {
		s.onCreate(svc.ID)
	}
	return err
}

// Update reads the service under id, applies mutate, and writes the result
// back inside a transaction so the read-modify-write is atomic.
func (s *Store) Update(ctx context.Context, id string, mutate func(*Service)) (*Service, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, "SELECT "+serviceColumns+" FROM services WHERE id = $1 FOR UPDATE", id)
	svc, err := scanService(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("service %s not found", id)
		}
		return nil, err
	}

	mutate(svc)
	// Stray whitespace in an address can't be dialed; a record saved before
	// this was trimmed gets cleaned up here, on its next update.
	svc.Host, svc.Username = strings.TrimSpace(svc.Host), strings.TrimSpace(svc.Username)
	svc.UpdatedAt = time.Now()

	healthStats, err := json.Marshal(svc.HealthStats)
	if err != nil {
		return nil, err
	}
	healthPorts, err := json.Marshal(svc.HealthPorts)
	if err != nil {
		return nil, err
	}
	healthContainers, err := json.Marshal(svc.HealthContainers)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE services
		SET name=$1, host=$2, port=$3, username=$4, deploy_dir=$5, manifest=$6, status=$7, last_job_id=$8,
			health=$9, health_detail=$10, last_synced_at=$11, encrypted_password=$12, updated_at=$13,
			container=$14, compose_project=$15, compose_files=$16, healthy_since=$17, controls_unlocked_at=$18,
			health_stats=$19, app_id=$20, health_ports=$21, health_containers=$22
		WHERE id=$23`,
		svc.Name, svc.Host, svc.Port, svc.Username, svc.DeployDir, svc.Manifest, svc.Status, svc.LastJobID,
		svc.Health, svc.HealthDetail, svc.LastSyncedAt, svc.EncryptedPassword, svc.UpdatedAt,
		svc.Container, svc.ComposeProject, svc.ComposeFiles, svc.HealthySince, svc.ControlsUnlockedAt,
		healthStats, svc.AppID, healthPorts, healthContainers, svc.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	svc.HasSavedPassword = svc.EncryptedPassword != ""
	return svc, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM services WHERE id = $1", id)
	return err
}

// LoginPassword is the saved (encrypted) SSH password of a server login —
// host, SSH port and user — from whichever of its services was saved most
// recently; "" if none has one.
func (s *Store) LoginPassword(ctx context.Context, host string, port int, username string) (string, error) {
	var enc string
	err := s.pool.QueryRow(ctx, `
		SELECT encrypted_password FROM services
		WHERE trim(host) = $1 AND port = $2 AND trim(username) = $3 AND encrypted_password <> ''
		ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(host), port, strings.TrimSpace(username)).Scan(&enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return enc, err
}

// SetLoginPassword saves enc (an encrypted SSH password that just logged
// in) for every service on that server login.
func (s *Store) SetLoginPassword(ctx context.Context, host string, port int, username, enc string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE services SET encrypted_password = $4
		WHERE trim(host) = $1 AND port = $2 AND trim(username) = $3`, strings.TrimSpace(host), port, strings.TrimSpace(username), enc)
	return err
}

// LoginPasswords is every distinct saved (encrypted) password on a server
// login, newest first.
func (s *Store) LoginPasswords(ctx context.Context, host string, port int, username string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT encrypted_password FROM services
		WHERE trim(host) = $1 AND port = $2 AND trim(username) = $3 AND encrypted_password <> ''
		GROUP BY encrypted_password ORDER BY max(updated_at) DESC`, strings.TrimSpace(host), port, strings.TrimSpace(username))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var enc string
		if err := rows.Scan(&enc); err != nil {
			return nil, err
		}
		out = append(out, enc)
	}
	return out, rows.Err()
}
