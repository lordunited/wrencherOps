// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxStoredLogBytes caps how much output is kept per entry. A long image
// pull can print a lot; the end of the log is where failures show up, so
// the tail is what's kept.
const maxStoredLogBytes = 1 << 20

// LogEntry is one saved action against a service and its output.
//
// Status is "running", "success" or "failed" for jobs. For a sync it's the
// health result instead: "healthy", "degraded" or "unreachable".
type LogEntry struct {
	ID          string     `json:"id"`
	ServiceID   string     `json:"serviceId"`
	ServiceName string     `json:"serviceName"`
	Host        string     `json:"host"`
	Action      string     `json:"action"`
	Status      string     `json:"status"`
	Log         string     `json:"log,omitempty"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// LogStore persists LogEntry records in Postgres, so a service's history
// survives page reloads and backend restarts.
type LogStore struct {
	pool *pgxpool.Pool
}

func NewLogStore(pool *pgxpool.Pool) *LogStore {
	return &LogStore{pool: pool}
}

// Start records a newly started action as running.
func (s *LogStore) Start(ctx context.Context, id string, svc *Service, action string, startedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO service_logs (id, service_id, service_name, host, action, status, started_at)
		VALUES ($1, $2, $3, $4, $5, 'running', $6)`,
		id, svc.ID, svc.Name, svc.Host, action, startedAt)
	return err
}

// Save writes an entry's current status and output. finishedAt is nil
// while the action is still running.
func (s *LogStore) Save(ctx context.Context, id, status, log string, finishedAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		"UPDATE service_logs SET status = $1, log = $2, finished_at = $3 WHERE id = $4",
		status, capLog(log), finishedAt, id)
	return err
}

// Record inserts an action that has already finished, such as a sync.
func (s *LogStore) Record(ctx context.Context, e LogEntry) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO service_logs (id, service_id, service_name, host, action, status, log, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		e.ID, e.ServiceID, e.ServiceName, e.Host, e.Action, e.Status, capLog(e.Log), e.StartedAt, e.FinishedAt)
	return err
}

// List returns a service's most recent entries, newest first, without
// their output (fetch one with Get for that).
func (s *LogStore) List(ctx context.Context, serviceID string, limit int) ([]LogEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, service_id, service_name, host, action, status, started_at, finished_at
		FROM service_logs WHERE service_id = $1
		ORDER BY started_at DESC LIMIT $2`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LogEntry, 0)
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.ID, &e.ServiceID, &e.ServiceName, &e.Host, &e.Action, &e.Status,
			&e.StartedAt, &e.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Get returns one entry, including its output.
func (s *LogStore) Get(ctx context.Context, serviceID, id string) (*LogEntry, bool, error) {
	var e LogEntry
	err := s.pool.QueryRow(ctx, `
		SELECT id, service_id, service_name, host, action, status, log, started_at, finished_at
		FROM service_logs WHERE service_id = $1 AND id = $2`, serviceID, id).
		Scan(&e.ID, &e.ServiceID, &e.ServiceName, &e.Host, &e.Action, &e.Status, &e.Log,
			&e.StartedAt, &e.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &e, true, nil
}

// MarkInterrupted closes out entries left "running" by a previous backend
// process. Jobs only run inside the process that started them, so after a
// restart nothing will ever finish those entries.
func (s *LogStore) MarkInterrupted(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE service_logs
		SET status = 'failed',
		    log = log || E'\n[Interrupted: the panel restarted before this action finished.]\n',
		    finished_at = now()
		WHERE status = 'running'`)
	return tag.RowsAffected(), err
}

// capLog makes output safe to store: Postgres text rejects NUL bytes and
// invalid UTF-8, both of which remote command output can contain. It then
// keeps only the tail if the output is over maxStoredLogBytes.
func capLog(log string) string {
	log = strings.ToValidUTF8(strings.ReplaceAll(log, "\x00", ""), "\uFFFD")
	if len(log) <= maxStoredLogBytes {
		return log
	}
	const marker = "[Earlier output trimmed.]\n"
	tail := log[len(log)-(maxStoredLogBytes-len(marker)):]
	// The cut may land mid-character; drop the partial bytes.
	return marker + strings.ToValidUTF8(tail, "")
}

// joinLog flattens a job's streamed output chunks into one string.
func joinLog(chunks []string) string {
	return strings.Join(chunks, "")
}
