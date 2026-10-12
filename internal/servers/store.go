// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"devops-wrapper/internal/randid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Server is one watched server and its last check.
type Server struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Host              string     `json:"host"`
	Port              int        `json:"port"`
	Username          string     `json:"username"`
	EncryptedPassword string     `json:"-"`
	HasSavedPassword  bool       `json:"hasSavedPassword"`
	Snapshot          *Snapshot  `json:"snapshot,omitempty"`
	CheckedAt         *time.Time `json:"checkedAt,omitempty"`
	CheckError        string     `json:"checkError,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	// Alerts are its open alerts, filled in when read through the API.
	Alerts []Alert `json:"alerts"`
}

// Alert is one alert raised on a server.
type Alert struct {
	ID             string     `json:"id"`
	ServerID       string     `json:"serverId"`
	ServerName     string     `json:"serverName,omitempty"`
	RuleID         string     `json:"ruleId"`
	RuleName       string     `json:"ruleName,omitempty"`
	Kind           string     `json:"kind,omitempty"`
	Subject        string     `json:"subject,omitempty"`
	Severity       string     `json:"severity"`
	Message        string     `json:"message"`
	Value          float64    `json:"value"`
	StartedAt      time.Time  `json:"startedAt"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
}

// Store persists servers, rules and alerts in Postgres.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const serverColumns = `id, name, host, port, username, encrypted_password, snapshot, checked_at, check_error, created_at`

func scanServer(row pgx.Row) (*Server, error) {
	var s Server
	var snap []byte
	if err := row.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.Username, &s.EncryptedPassword, &snap, &s.CheckedAt, &s.CheckError, &s.CreatedAt); err != nil {
		return nil, err
	}
	if len(snap) > 0 && string(snap) != "null" {
		s.Snapshot = &Snapshot{}
		if err := json.Unmarshal(snap, s.Snapshot); err != nil {
			s.Snapshot = nil
		}
	}
	s.HasSavedPassword = s.EncryptedPassword != ""
	s.Alerts = []Alert{}
	return &s, nil
}

func (st *Store) List(ctx context.Context) ([]*Server, error) {
	rows, err := st.pool.Query(ctx, "SELECT "+serverColumns+" FROM servers WHERE NOT removed ORDER BY name, host")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Server{}
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (st *Store) Get(ctx context.Context, id string) (*Server, bool) {
	s, err := scanServer(st.pool.QueryRow(ctx, "SELECT "+serverColumns+" FROM servers WHERE id = $1 AND NOT removed", id))
	if err != nil {
		return nil, false
	}
	return s, true
}

// Add adds a server, or brings back one removed earlier at that address.
func (st *Store) Add(ctx context.Context, s *Server) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	s.ID, s.CreatedAt = id, time.Now()
	err = st.pool.QueryRow(ctx, `
		INSERT INTO servers (id, name, host, port, username, encrypted_password, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (host, port) DO UPDATE SET name = $2, username = $5, encrypted_password = $6, removed = false
		WHERE servers.removed
		RETURNING id`, s.ID, s.Name, s.Host, s.Port, s.Username, s.EncryptedPassword, s.CreatedAt).Scan(&s.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errAlreadyWatched
	}
	return err
}

var errAlreadyWatched = errors.New("that server is already in the list")

func (st *Store) Update(ctx context.Context, s *Server) error {
	_, err := st.pool.Exec(ctx, `UPDATE servers SET name=$2, username=$3, encrypted_password=$4 WHERE id=$1`,
		s.ID, s.Name, s.Username, s.EncryptedPassword)
	return err
}

// Remove stops watching a server: it leaves the list (and isn't added back
// from its services), and its open alerts are resolved.
func (st *Store) Remove(ctx context.Context, id string) error {
	if _, err := st.pool.Exec(ctx, `UPDATE servers SET removed = true, snapshot = NULL, checked_at = NULL WHERE id = $1`, id); err != nil {
		return err
	}
	_, err := st.pool.Exec(ctx, `UPDATE server_alerts SET resolved_at = now() WHERE server_id = $1 AND resolved_at IS NULL`, id)
	return err
}

func (st *Store) SaveCheck(ctx context.Context, id string, snap *Snapshot, checkErr string) error {
	var raw any
	if snap != nil {
		b, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		raw = b
	}
	// A failed check keeps the last snapshot, so the page still shows the
	// last known state beside the error.
	_, err := st.pool.Exec(ctx, `
		UPDATE servers SET snapshot = COALESCE($2::jsonb, snapshot), checked_at = now(), check_error = $3 WHERE id = $1`,
		id, raw, checkErr)
	return err
}

func (st *Store) SetPassword(ctx context.Context, id, enc string) error {
	_, err := st.pool.Exec(ctx, `UPDATE servers SET encrypted_password = $2 WHERE id = $1`, id, enc)
	return err
}

// DiscoverFromServices adds every server the panel's services run on that
// isn't watched yet (nor removed), with the login whose password is saved —
// root's when there is one.
func (st *Store) DiscoverFromServices(ctx context.Context) (int, error) {
	rows, err := st.pool.Query(ctx, `
		SELECT DISTINCT ON (trim(s.host), s.port) trim(s.host), s.port, trim(s.username), s.encrypted_password
		FROM services s
		WHERE s.encrypted_password <> '' AND trim(s.host) <> ''
		  AND NOT EXISTS (SELECT 1 FROM servers v WHERE v.host = trim(s.host) AND v.port = s.port)
		ORDER BY trim(s.host), s.port, (trim(s.username) = 'root') DESC, s.updated_at DESC`)
	if err != nil {
		return 0, err
	}
	type login struct {
		host     string
		port     int
		username string
		enc      string
	}
	var found []login
	for rows.Next() {
		var l login
		if err := rows.Scan(&l.host, &l.port, &l.username, &l.enc); err != nil {
			rows.Close()
			return 0, err
		}
		found = append(found, l)
	}
	rows.Close()
	added := 0
	for _, l := range found {
		s := &Server{Name: l.host, Host: l.host, Port: l.port, Username: l.username, EncryptedPassword: l.enc}
		if err := st.Add(ctx, s); err == nil {
			added++
		}
	}
	return added, nil
}

// ServicePasswords is every distinct saved password the panel's services
// hold for this login — a server's password may have been updated there.
func (st *Store) ServicePasswords(ctx context.Context, host string, port int, username string) ([]string, error) {
	rows, err := st.pool.Query(ctx, `
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

// SetServicesPassword saves a password that just logged in for every
// service on the same login too, as passwords are kept per server login.
func (st *Store) SetServicesPassword(ctx context.Context, host string, port int, username, enc string) error {
	_, err := st.pool.Exec(ctx, `UPDATE services SET encrypted_password = $4 WHERE trim(host) = $1 AND port = $2 AND trim(username) = $3`,
		strings.TrimSpace(host), port, strings.TrimSpace(username), enc)
	return err
}

// --- rules

const ruleColumns = `id, kind, name, threshold, severity, enabled, server_id, builtin, created_at`

func scanRule(row pgx.Row) (Rule, error) {
	var r Rule
	err := row.Scan(&r.ID, &r.Kind, &r.Name, &r.Threshold, &r.Severity, &r.Enabled, &r.ServerID, &r.Builtin, &r.CreatedAt)
	return r, err
}

// SeedRules creates the built-in rules that don't exist yet.
func (st *Store) SeedRules(ctx context.Context) error {
	for _, r := range DefaultRules {
		if _, err := st.pool.Exec(ctx, `
			INSERT INTO server_alert_rules (`+ruleColumns+`) VALUES ($1,$2,$3,$4,$5,$6,'',true,now())
			ON CONFLICT (id) DO NOTHING`, r.ID, r.Kind, r.Name, r.Threshold, r.Severity, r.Enabled); err != nil {
			return err
		}
	}
	return nil
}

func (st *Store) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := st.pool.Query(ctx, "SELECT "+ruleColumns+" FROM server_alert_rules ORDER BY builtin DESC, kind, threshold DESC, created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (st *Store) Rule(ctx context.Context, id string) (Rule, bool) {
	r, err := scanRule(st.pool.QueryRow(ctx, "SELECT "+ruleColumns+" FROM server_alert_rules WHERE id = $1", id))
	return r, err == nil
}

func (st *Store) AddRule(ctx context.Context, r *Rule) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	r.ID, r.CreatedAt, r.Builtin = id, time.Now(), false
	_, err = st.pool.Exec(ctx, `INSERT INTO server_alert_rules (`+ruleColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,false,$8)`,
		r.ID, r.Kind, r.Name, r.Threshold, r.Severity, r.Enabled, r.ServerID, r.CreatedAt)
	return err
}

func (st *Store) UpdateRule(ctx context.Context, r Rule) error {
	_, err := st.pool.Exec(ctx, `UPDATE server_alert_rules SET name=$2, threshold=$3, severity=$4, enabled=$5, server_id=$6 WHERE id=$1`,
		r.ID, r.Name, r.Threshold, r.Severity, r.Enabled, r.ServerID)
	return err
}

// DeleteRule deletes a rule you added, and resolves its open alerts.
func (st *Store) DeleteRule(ctx context.Context, id string) error {
	if _, err := st.pool.Exec(ctx, `DELETE FROM server_alert_rules WHERE id = $1 AND NOT builtin`, id); err != nil {
		return err
	}
	_, err := st.pool.Exec(ctx, `UPDATE server_alerts SET resolved_at = now() WHERE rule_id = $1 AND resolved_at IS NULL`, id)
	return err
}

// --- alerts

// ApplyFindings brings a server's alerts in line with one check: each
// finding opens an alert or updates the open one (value, message,
// severity); an open alert from a rule that was judged but no longer finds
// anything is resolved. Alerts from rules not judged are left as they are.
func (st *Store) ApplyFindings(ctx context.Context, serverID string, findings []Finding, judged []string) (opened []Alert, err error) {
	tx, err := st.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	now := time.Now()
	keep := map[string]bool{}
	for _, f := range findings {
		keep[f.RuleID+"\x00"+f.Subject] = true
		tag, err := tx.Exec(ctx, `
			UPDATE server_alerts SET last_seen_at = $4, value = $5, message = $6, severity = $7
			WHERE server_id = $1 AND rule_id = $2 AND subject = $3 AND resolved_at IS NULL`,
			serverID, f.RuleID, f.Subject, now, f.Value, f.Message, f.Severity)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() > 0 {
			continue
		}
		id, err := randid.New(16)
		if err != nil {
			return nil, err
		}
		a := Alert{ID: id, ServerID: serverID, RuleID: f.RuleID, Subject: f.Subject, Severity: f.Severity, Message: f.Message, Value: f.Value, StartedAt: now, LastSeenAt: now}
		if _, err := tx.Exec(ctx, `
			INSERT INTO server_alerts (id, server_id, rule_id, subject, severity, message, value, started_at, last_seen_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, a.ID, serverID, f.RuleID, f.Subject, f.Severity, f.Message, f.Value, now); err != nil {
			return nil, err
		}
		opened = append(opened, a)
	}
	if len(judged) > 0 {
		rows, err := tx.Query(ctx, `SELECT id, rule_id, subject FROM server_alerts WHERE server_id = $1 AND resolved_at IS NULL AND rule_id = ANY($2)`, serverID, judged)
		if err != nil {
			return nil, err
		}
		var resolve []string
		for rows.Next() {
			var id, rule, subject string
			if err := rows.Scan(&id, &rule, &subject); err != nil {
				rows.Close()
				return nil, err
			}
			if !keep[rule+"\x00"+subject] {
				resolve = append(resolve, id)
			}
		}
		rows.Close()
		if len(resolve) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE server_alerts SET resolved_at = $2 WHERE id = ANY($1)`, resolve, now); err != nil {
				return nil, err
			}
		}
	}
	return opened, tx.Commit(ctx)
}

// Alerts lists alerts with their server's and rule's names: the open ones,
// or also those resolved in the last week.
func (st *Store) Alerts(ctx context.Context, includeResolved bool) ([]Alert, error) {
	q := `
		SELECT a.id, a.server_id, coalesce(s.name, ''), a.rule_id, coalesce(r.name, ''), coalesce(r.kind, ''), a.subject, a.severity,
		       a.message, a.value, a.started_at, a.last_seen_at, a.resolved_at, a.acknowledged_at
		FROM server_alerts a
		LEFT JOIN servers s ON s.id = a.server_id
		LEFT JOIN server_alert_rules r ON r.id = a.rule_id
		WHERE (a.resolved_at IS NULL AND NOT coalesce(s.removed, true))`
	if includeResolved {
		q += ` OR a.resolved_at > now() - interval '7 days'`
	}
	q += ` ORDER BY a.resolved_at IS NOT NULL, CASE a.severity WHEN 'critical' THEN 0 ELSE 1 END, a.started_at DESC LIMIT 500`
	rows, err := st.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.ServerID, &a.ServerName, &a.RuleID, &a.RuleName, &a.Kind, &a.Subject, &a.Severity,
			&a.Message, &a.Value, &a.StartedAt, &a.LastSeenAt, &a.ResolvedAt, &a.AcknowledgedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (st *Store) Acknowledge(ctx context.Context, id string) error {
	tag, err := st.pool.Exec(ctx, `UPDATE server_alerts SET acknowledged_at = coalesce(acknowledged_at, now()) WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
