// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package ansible runs Ansible playbooks kept in the panel on the servers
// it knows: the inventory is built from the Servers list, logging in (and
// becoming root) with each server's saved password. A run is a dry run
// (--check --diff) or for real; an apply needs a passed dry run of exactly
// the same playbook, extra vars and servers first.
package ansible

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"devops-wrapper/internal/randid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EntryPoint is the default file ansible-playbook runs; a playbook's Entry
// can name another (an uploaded project's site.yml, say).
const EntryPoint = "playbook.yml"

// File is one file of a playbook: the playbook itself, a role, a template,
// vars...
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Playbook struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Files       []File    `json:"files"`
	Entry       string    `json:"entry"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// LastRun is its newest run, filled in when listed.
	LastRun *Run `json:"lastRun,omitempty"`
}

// Target is a server a run targets, as it was when the run started.
type Target struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"` // its inventory name
	Host     string `json:"host"` // address
	User     string `json:"user"` // SSH user
}

// HostRecap is one host's line of Ansible's PLAY RECAP.
type HostRecap struct {
	Host        string `json:"host"`
	OK          int    `json:"ok"`
	Changed     int    `json:"changed"`
	Unreachable int    `json:"unreachable"`
	Failed      int    `json:"failed"`
	Skipped     int    `json:"skipped"`
	Rescued     int    `json:"rescued"`
	Ignored     int    `json:"ignored"`
}

type Run struct {
	ID           string      `json:"id"`
	PlaybookID   string      `json:"playbookId"`
	PlaybookName string      `json:"playbookName"`
	Mode         string      `json:"mode"` // "check" | "apply"
	ContentSHA   string      `json:"contentSha"`
	Targets      []Target    `json:"targets"`
	ExtraVars    string      `json:"extraVars"`
	Status       string      `json:"status"` // "running" | "success" | "failed" | "canceled"
	Log          string      `json:"log,omitempty"`
	Recap        []HostRecap `json:"recap"`
	Error        string      `json:"error,omitempty"`
	StartedAt    time.Time   `json:"startedAt"`
	FinishedAt   *time.Time  `json:"finishedAt,omitempty"`
}

// Limits on a playbook's files: they're stored with it and written for each
// run, so this is for playbooks, roles and templates, not data.
const (
	maxFiles     = 200
	maxFileBytes = 1 << 20
	maxBytes     = 8 << 20
)

var pathRe = regexp.MustCompile(`^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$`)

// reserved are files the panel writes itself for a run.
var reserved = map[string]bool{"inventory.ini": true, "ansible.cfg": true, "extra_vars.yml": true}

// ValidateFiles checks a playbook's files: safe relative paths, text, the
// limits, and a non-empty entry point among them.
func ValidateFiles(files []File, entry string) error {
	if len(files) > maxFiles {
		return fmt.Errorf("a playbook can have at most %d files", maxFiles)
	}
	seen := map[string]bool{}
	total, hasEntry := 0, false
	for _, f := range files {
		p := f.Path
		switch {
		case !pathRe.MatchString(p) || strings.Contains(p, "//") || strings.HasSuffix(p, "/"):
			return fmt.Errorf("%q isn't a valid file path (letters, digits, . _ - @ + and / between folders)", p)
		case path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../"):
			return fmt.Errorf("%q must stay inside the playbook's folder", p)
		case reserved[p]:
			return fmt.Errorf("%s is written by the panel for each run; it can't be a playbook file", p)
		case seen[p]:
			return fmt.Errorf("%s is listed twice", p)
		case len(f.Content) > maxFileBytes:
			return fmt.Errorf("%s is larger than %d KiB", p, maxFileBytes>>10)
		case !utf8.ValidString(f.Content) || strings.ContainsRune(f.Content, 0):
			return fmt.Errorf("%s isn't a text file", p)
		}
		seen[p] = true
		total += len(f.Content)
		if p == entry {
			hasEntry = strings.TrimSpace(f.Content) != ""
		}
	}
	if !hasEntry {
		return fmt.Errorf("the entry point %s is missing or empty", entry)
	}
	if total > maxBytes {
		return fmt.Errorf("the playbook's files add up to more than %d MiB", maxBytes>>20)
	}
	return nil
}

// ContentSHA identifies exactly what a run ran: every file, the entry point
// and the extra vars. A dry run and an apply with the same SHA ran the same
// thing.
func ContentSHA(files []File, entry, extraVars string) string {
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s\x00%d\x00%s\x00", f.Path, len(f.Content), f.Content)
	}
	fmt.Fprintf(h, "entry\x00%s\x00extra\x00%s", entry, strings.TrimSpace(extraVars))
	return hex.EncodeToString(h.Sum(nil))
}

// targetKey identifies a set of servers, whatever their order.
func targetKey(ts []Target) string {
	ids := make([]string, len(ts))
	for i, t := range ts {
		ids[i] = t.ServerID
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const playbookColumns = `id, name, description, files, created_at, updated_at, entry`

func scanPlaybook(row pgx.Row) (*Playbook, error) {
	var p Playbook
	var files []byte
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &files, &p.CreatedAt, &p.UpdatedAt, &p.Entry); err != nil {
		return nil, err
	}
	if json.Unmarshal(files, &p.Files) != nil || p.Files == nil {
		p.Files = []File{}
	}
	return &p, nil
}

func (s *Store) Playbooks(ctx context.Context) ([]*Playbook, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+playbookColumns+" FROM ansible_playbooks ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Playbook{}
	for rows.Next() {
		p, err := scanPlaybook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Playbook(ctx context.Context, id string) (*Playbook, bool) {
	p, err := scanPlaybook(s.pool.QueryRow(ctx, "SELECT "+playbookColumns+" FROM ansible_playbooks WHERE id = $1", id))
	return p, err == nil
}

func (s *Store) CreatePlaybook(ctx context.Context, p *Playbook) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	p.ID, p.CreatedAt = id, time.Now()
	p.UpdatedAt = p.CreatedAt
	files, _ := json.Marshal(p.Files)
	if p.Entry == "" {
		p.Entry = EntryPoint
	}
	_, err = s.pool.Exec(ctx, "INSERT INTO ansible_playbooks ("+playbookColumns+") VALUES ($1,$2,$3,$4,$5,$6,$7)",
		p.ID, p.Name, p.Description, files, p.CreatedAt, p.UpdatedAt, p.Entry)
	return err
}

func (s *Store) UpdatePlaybook(ctx context.Context, p *Playbook) error {
	p.UpdatedAt = time.Now()
	files, _ := json.Marshal(p.Files)
	_, err := s.pool.Exec(ctx, "UPDATE ansible_playbooks SET name=$2, description=$3, files=$4, updated_at=$5, entry=$6 WHERE id=$1",
		p.ID, p.Name, p.Description, files, p.UpdatedAt, p.Entry)
	return err
}

// DeletePlaybook deletes a playbook; its runs stay in the history.
func (s *Store) DeletePlaybook(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM ansible_playbooks WHERE id = $1", id)
	return err
}

const runColumns = `id, playbook_id, playbook_name, mode, content_sha, targets, extra_vars, status, log, recap, error, started_at, finished_at`

func scanRun(row pgx.Row, withLog bool) (*Run, error) {
	var r Run
	var targets, recap []byte
	if err := row.Scan(&r.ID, &r.PlaybookID, &r.PlaybookName, &r.Mode, &r.ContentSHA, &targets, &r.ExtraVars, &r.Status, &r.Log, &recap, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
		return nil, err
	}
	json.Unmarshal(targets, &r.Targets)
	if json.Unmarshal(recap, &r.Recap) != nil || r.Recap == nil {
		r.Recap = []HostRecap{}
	}
	if !withLog {
		r.Log = ""
	}
	return &r, nil
}

func (s *Store) CreateRun(ctx context.Context, r *Run) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	r.ID, r.StartedAt, r.Status = id, time.Now(), "running"
	if r.Recap == nil {
		r.Recap = []HostRecap{}
	}
	targets, _ := json.Marshal(r.Targets)
	recap, _ := json.Marshal(r.Recap)
	_, err = s.pool.Exec(ctx, "INSERT INTO ansible_runs ("+runColumns+") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULL)",
		r.ID, r.PlaybookID, r.PlaybookName, r.Mode, r.ContentSHA, targets, r.ExtraVars, r.Status, r.Log, recap, r.Error, r.StartedAt)
	return err
}

// SaveLog stores a running run's log so far.
func (s *Store) SaveLog(ctx context.Context, id, log string) {
	s.pool.Exec(ctx, "UPDATE ansible_runs SET log = $2 WHERE id = $1 AND status = 'running'", id, log)
}

func (s *Store) FinishRun(ctx context.Context, r *Run) error {
	now := time.Now()
	r.FinishedAt = &now
	recap, _ := json.Marshal(r.Recap)
	_, err := s.pool.Exec(ctx, "UPDATE ansible_runs SET status=$2, log=$3, recap=$4, error=$5, finished_at=$6 WHERE id=$1",
		r.ID, r.Status, r.Log, recap, r.Error, now)
	return err
}

func (s *Store) Run(ctx context.Context, id string) (*Run, bool) {
	r, err := scanRun(s.pool.QueryRow(ctx, "SELECT "+runColumns+" FROM ansible_runs WHERE id = $1", id), true)
	return r, err == nil
}

// Runs lists runs, newest first, without their logs: all of them, or one
// playbook's.
func (s *Store) Runs(ctx context.Context, playbookID string, limit int) ([]*Run, error) {
	q, args := "SELECT "+runColumns+" FROM ansible_runs", []any{}
	if playbookID != "" {
		q += " WHERE playbook_id = $1"
		args = append(args, playbookID)
	}
	q += fmt.Sprintf(" ORDER BY started_at DESC LIMIT %d", limit)
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Run{}
	for rows.Next() {
		r, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PassedDryRun finds a successful dry run of exactly this content on exactly
// these servers in the last day — what an apply needs first.
func (s *Store) PassedDryRun(ctx context.Context, playbookID, contentSHA string, targets []Target) (*Run, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+runColumns+` FROM ansible_runs
		WHERE playbook_id = $1 AND content_sha = $2 AND mode = 'check' AND status = 'success' AND started_at > now() - interval '1 day'
		ORDER BY started_at DESC LIMIT 20`, playbookID, contentSHA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	want := targetKey(targets)
	for rows.Next() {
		r, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		if targetKey(r.Targets) == want {
			return r, nil
		}
	}
	return nil, errNoDryRun
}

var errNoDryRun = errors.New("no passed dry run")

// MarkInterrupted fails runs left "running" by a restart of the panel.
func (s *Store) MarkInterrupted(ctx context.Context) {
	s.pool.Exec(ctx, `UPDATE ansible_runs SET status = 'failed', error = 'the panel restarted while it ran', finished_at = now() WHERE status = 'running'`)
}
