// Package service tracks deployable services (deployed or pending), each
// holding its own editable compose/manifest YAML. Persisted to Postgres —
// these are meaningful user records, not a cache.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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
)

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
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Type              string     `json:"type"` // "gitlab" | "nexus3" | "docker" | "kubernetes"
	Host              string     `json:"host"`
	Port              int        `json:"port"`
	Username          string     `json:"username"`
	DeployDir         string     `json:"deployDir"`
	Manifest          string     `json:"manifest"`
	Status            Status     `json:"status"`
	LastJobID         string     `json:"lastJobId,omitempty"`
	Health            string     `json:"health"`       // "unknown" | "healthy" | "degraded" | "unreachable"
	HealthDetail      string     `json:"healthDetail"` // e.g. "2/2 running", or an error message
	LastSyncedAt      *time.Time `json:"lastSyncedAt,omitempty"`
	EncryptedPassword string     `json:"-"`
	HasSavedPassword  bool       `json:"hasSavedPassword"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

const serviceColumns = `id, name, type, host, port, username, deploy_dir, manifest, status,
	last_job_id, health, health_detail, last_synced_at, encrypted_password, created_at, updated_at`

func scanService(row pgx.Row) (*Service, error) {
	var svc Service
	err := row.Scan(&svc.ID, &svc.Name, &svc.Type, &svc.Host, &svc.Port, &svc.Username,
		&svc.DeployDir, &svc.Manifest, &svc.Status, &svc.LastJobID,
		&svc.Health, &svc.HealthDetail, &svc.LastSyncedAt, &svc.EncryptedPassword,
		&svc.CreatedAt, &svc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	svc.HasSavedPassword = svc.EncryptedPassword != ""
	return &svc, nil
}

// Store persists services in Postgres.
type Store struct {
	pool *pgxpool.Pool
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
	svc.CreatedAt = time.Now()
	svc.UpdatedAt = time.Now()
	if svc.Health == "" {
		svc.Health = "unknown"
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO services (`+serviceColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		svc.ID, svc.Name, svc.Type, svc.Host, svc.Port, svc.Username,
		svc.DeployDir, svc.Manifest, svc.Status, svc.LastJobID,
		svc.Health, svc.HealthDetail, svc.LastSyncedAt, svc.EncryptedPassword,
		svc.CreatedAt, svc.UpdatedAt)
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
	svc.UpdatedAt = time.Now()

	_, err = tx.Exec(ctx, `
		UPDATE services
		SET name=$1, host=$2, port=$3, username=$4, deploy_dir=$5, manifest=$6, status=$7, last_job_id=$8,
			health=$9, health_detail=$10, last_synced_at=$11, encrypted_password=$12, updated_at=$13
		WHERE id=$14`,
		svc.Name, svc.Host, svc.Port, svc.Username, svc.DeployDir, svc.Manifest, svc.Status, svc.LastJobID,
		svc.Health, svc.HealthDetail, svc.LastSyncedAt, svc.EncryptedPassword, svc.UpdatedAt, svc.ID)
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
