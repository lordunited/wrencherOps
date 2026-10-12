// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Reading a Prometheus over SSH takes seconds. The last successful reading
// of each kind is kept so a page can show it at once, marked with its age,
// while a fresh one loads.

const (
	snapshotInspect = "inspect" // Rules & targets
	snapshotStatus  = "status"  // Status
)

// SnapshotStore persists the last reading per service and kind.
type SnapshotStore struct {
	pool *pgxpool.Pool
}

func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{pool: pool}
}

func (s *SnapshotStore) Save(ctx context.Context, serviceID, kind string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO prometheus_snapshots (service_id, kind, data, fetched_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (service_id, kind) DO UPDATE SET data = EXCLUDED.data, fetched_at = EXCLUDED.fetched_at`,
		serviceID, kind, data)
	if err != nil {
		log.Printf("snapshots: save %s/%s: %v", serviceID, kind, err)
	}
}

func (s *SnapshotStore) Get(ctx context.Context, serviceID, kind string) (json.RawMessage, time.Time, bool) {
	var data []byte
	var at time.Time
	err := s.pool.QueryRow(ctx, "SELECT data, fetched_at FROM prometheus_snapshots WHERE service_id = $1 AND kind = $2",
		serviceID, kind).Scan(&data, &at)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return nil, time.Time{}, false
	}
	return data, at, true
}

func (s *SnapshotStore) DeleteService(ctx context.Context, serviceID string) {
	s.pool.Exec(ctx, "DELETE FROM prometheus_snapshots WHERE service_id = $1", serviceID)
}

type snapshotResponse struct {
	Data      json.RawMessage `json:"data"`
	FetchedAt time.Time       `json:"fetchedAt"`
}

// handlePromSnapshot returns the last saved reading of ?kind=inspect|status,
// or 404 when there is none yet. It never touches the server.
func (h *Handler) handlePromSnapshot(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind != snapshotInspect && kind != snapshotStatus && kind != snapshotNginx && kind != snapshotHAProxy && kind != snapshotEnvoy {
		http.Error(w, "kind must be inspect, status, nginx, haproxy or envoy", http.StatusBadRequest)
		return
	}
	data, at, ok := h.snapshots.Get(r.Context(), r.PathValue("id"), kind)
	if !ok {
		http.Error(w, "no saved reading yet", http.StatusNotFound)
		return
	}
	writeJSON(w, snapshotResponse{Data: data, FetchedAt: at}, nil)
}
