// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package terraform runs Terraform workspaces kept in the panel, with
// OpenTofu (Terraform's open-source fork: the same .tf language and
// providers; HashiCorp's own provider registry is blocked in this region).
// A run is a plan — validated, saved — and an apply applies exactly a saved
// plan, so nothing that wasn't reviewed is ever changed. Unless the code
// declares its own backend, the panel keeps the state, encrypted, with
// every version.
package terraform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// File is one file of a workspace: .tf, .tfvars, a module's files...
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Files       []File    `json:"files"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// StateSerial is the managed state's serial (0: no state yet);
	// OwnBackend is the backend the code declares, "" when the panel keeps
	// the state.
	StateSerial    int        `json:"stateSerial"`
	StateUpdatedAt *time.Time `json:"stateUpdatedAt,omitempty"`
	OwnBackend     string     `json:"ownBackend,omitempty"`
	stateEnc       string
	LastRun        *Run `json:"lastRun,omitempty"`
}

// Variable is a workspace variable: a Terraform input variable (Kind
// "terraform", passed as TF_VAR_<key>) or an environment variable ("env",
// e.g. a provider's credentials). A sensitive one's value is never sent back.
type Variable struct {
	Key       string `json:"key"`
	Kind      string `json:"kind"`
	Value     string `json:"value,omitempty"`
	Sensitive bool   `json:"sensitive"`
	HCL       bool   `json:"hcl"`
	// Keep, on a save, keeps a sensitive variable's stored value.
	Keep     bool `json:"keep,omitempty"`
	valueEnc string
}

// Summary is what a plan would do, or an apply did.
type Summary struct {
	Add     int      `json:"add"`
	Change  int      `json:"change"`
	Destroy int      `json:"destroy"`
	Replace int      `json:"replace"`
	Import  int      `json:"import,omitempty"`
	Changes []Change `json:"changes,omitempty"`
}

// RunOptions are what a plan is asked for beyond the code.
type RunOptions struct {
	// Replace are resources planned for replacement (-replace): the
	// successor of terraform taint.
	Replace []string `json:"replace,omitempty"`
}

// Change is one resource a plan touches.
type Change struct {
	Address string `json:"address"`
	Action  string `json:"action"` // create, update, delete, replace, read, import
}

type Run struct {
	ID            string  `json:"id"`
	WorkspaceID   string  `json:"workspaceId"`
	WorkspaceName string  `json:"workspaceName"`
	Kind          string  `json:"kind"` // "plan", "destroy-plan", "apply"
	Status        string  `json:"status"`
	ContentSHA    string  `json:"contentSha"`
	Log           string  `json:"log,omitempty"`
	Error         string  `json:"error,omitempty"`
	Summary       Summary `json:"summary"`
	// StartedBy is the panel user who started the run.
	StartedBy string `json:"startedBy,omitempty"`
	// Options are what the plan was asked for beyond the code.
	Options RunOptions `json:"options"`
	// Checks are the standard's findings when the plan was made (only
	// advice: an enforced one stops the plan before it's a run).
	Checks       []Finding  `json:"checks,omitempty"`
	AppliesRunID string     `json:"appliesRunId,omitempty"`
	AppliedByRun string     `json:"appliedByRunId,omitempty"`
	StateSerial  int        `json:"stateSerial"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	planEnc      string
}

// Limits on a workspace's files.
const (
	maxFiles     = 300
	maxFileBytes = 1 << 20
	maxBytes     = 8 << 20
)

var pathRe = regexp.MustCompile(`^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$`)

// generated are files a run makes, never part of a workspace: providers,
// state (the panel keeps it), plans, logs.
func generated(p string) bool {
	base := path.Base(p)
	switch {
	case strings.HasPrefix(p, ".terraform/") || strings.Contains(p, "/.terraform/"),
		strings.HasSuffix(base, ".tfstate"), strings.HasSuffix(base, ".tfstate.backup"),
		strings.HasSuffix(base, ".tfplan"), base == "crash.log", base == "plan.panel":
		return true
	}
	return false
}

// ValidateFiles checks a workspace's files: safe relative paths, text, the
// limits, and at least one .tf file at the root.
func ValidateFiles(files []File) error {
	if len(files) > maxFiles {
		return fmt.Errorf("a workspace can have at most %d files", maxFiles)
	}
	seen := map[string]bool{}
	total, rootTF := 0, false
	for _, f := range files {
		p := f.Path
		switch {
		case !pathRe.MatchString(p) || strings.Contains(p, "//") || strings.HasSuffix(p, "/"):
			return fmt.Errorf("%q isn't a valid file path (letters, digits, . _ - @ + and / between folders)", p)
		case path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../"):
			return fmt.Errorf("%q must stay inside the workspace", p)
		case generated(p):
			return fmt.Errorf("%s is made by a run (providers, state, plans); it can't be a workspace file", p)
		case seen[p]:
			return fmt.Errorf("%s is listed twice", p)
		case len(f.Content) > maxFileBytes:
			return fmt.Errorf("%s is larger than %d KiB", p, maxFileBytes>>10)
		case !utf8.ValidString(f.Content) || strings.ContainsRune(f.Content, 0):
			return fmt.Errorf("%s isn't a text file", p)
		}
		seen[p] = true
		total += len(f.Content)
		if !strings.Contains(p, "/") && (strings.HasSuffix(p, ".tf") || strings.HasSuffix(p, ".tf.json")) {
			rootTF = true
		}
	}
	if !rootTF {
		return fmt.Errorf("a workspace needs at least one .tf file at its root")
	}
	if total > maxBytes {
		return fmt.Errorf("the workspace's files add up to more than %d MiB", maxBytes>>20)
	}
	return nil
}

// LockFile is the dependency lock file a run keeps up to date.
const LockFile = ".terraform.lock.hcl"

// ContentSHA identifies a workspace's code — every file but the lock file,
// which init itself keeps current — so an apply can require that a plan was
// made from exactly the code there is now.
func ContentSHA(files []File) string {
	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, f := range sorted {
		if f.Path == LockFile {
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00%s\x00", f.Path, len(f.Content), f.Content)
	}
	return hex.EncodeToString(h.Sum(nil))
}

var backendRe = regexp.MustCompile(`(?s)\bterraform\s*\{.*?\bbackend\s+"([A-Za-z0-9_]+)"`)

// OwnBackend is the backend the root module declares ("s3", "pg", ...), or
// "" — then the panel keeps the state.
func OwnBackend(files []File) string {
	for _, f := range files {
		if strings.Contains(f.Path, "/") || !strings.HasSuffix(f.Path, ".tf") {
			continue
		}
		if m := backendRe.FindStringSubmatch(stripComments(f.Content)); m != nil && m[1] != "local" {
			return m[1]
		}
	}
	return ""
}

var commentRe = regexp.MustCompile(`(?m)(#|//).*$|(?s)/\*.*?\*/`)

func stripComments(s string) string { return commentRe.ReplaceAllString(s, "") }

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const wsColumns = `id, name, description, files, state_enc, state_serial, state_updated_at, created_at, updated_at`

func scanWorkspace(row pgx.Row) (*Workspace, error) {
	var w Workspace
	var files []byte
	if err := row.Scan(&w.ID, &w.Name, &w.Description, &files, &w.stateEnc, &w.StateSerial, &w.StateUpdatedAt, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return nil, err
	}
	if json.Unmarshal(files, &w.Files) != nil || w.Files == nil {
		w.Files = []File{}
	}
	w.OwnBackend = OwnBackend(w.Files)
	return &w, nil
}

func (s *Store) Workspaces(ctx context.Context) ([]*Workspace, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+wsColumns+" FROM tf_workspaces ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Workspace{}
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) Workspace(ctx context.Context, id string) (*Workspace, bool) {
	w, err := scanWorkspace(s.pool.QueryRow(ctx, "SELECT "+wsColumns+" FROM tf_workspaces WHERE id = $1", id))
	return w, err == nil
}

func (s *Store) CreateWorkspace(ctx context.Context, w *Workspace) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	w.ID, w.CreatedAt = id, time.Now()
	w.UpdatedAt = w.CreatedAt
	files, _ := json.Marshal(w.Files)
	_, err = s.pool.Exec(ctx, `INSERT INTO tf_workspaces (id, name, description, files, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		w.ID, w.Name, w.Description, files, w.CreatedAt, w.UpdatedAt)
	return err
}

func (s *Store) UpdateWorkspace(ctx context.Context, w *Workspace) error {
	w.UpdatedAt = time.Now()
	files, _ := json.Marshal(w.Files)
	_, err := s.pool.Exec(ctx, `UPDATE tf_workspaces SET name=$2, description=$3, files=$4, updated_at=$5 WHERE id=$1`,
		w.ID, w.Name, w.Description, files, w.UpdatedAt)
	return err
}

// SaveLockFile keeps the lock file init wrote, without touching updated_at
// (it isn't an edit of the code).
func (s *Store) SaveLockFile(ctx context.Context, id, content string) error {
	w, ok := s.Workspace(ctx, id)
	if !ok {
		return fmt.Errorf("workspace gone")
	}
	found := false
	for i := range w.Files {
		if w.Files[i].Path == LockFile {
			if w.Files[i].Content == content {
				return nil
			}
			w.Files[i].Content, found = content, true
		}
	}
	if !found {
		w.Files = append(w.Files, File{Path: LockFile, Content: content})
	}
	files, _ := json.Marshal(w.Files)
	_, err := s.pool.Exec(ctx, `UPDATE tf_workspaces SET files=$2 WHERE id=$1`, id, files)
	return err
}

// DeleteWorkspace deletes a workspace, its variables and its state history
// (its runs stay, as history).
func (s *Store) DeleteWorkspace(ctx context.Context, id string) error {
	for _, q := range []string{"DELETE FROM tf_variables WHERE workspace_id = $1", "DELETE FROM tf_state_versions WHERE workspace_id = $1", "DELETE FROM tf_workspaces WHERE id = $1"} {
		if _, err := s.pool.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return nil
}

// SaveState stores a new state (encrypted) and keeps it as a version.
func (s *Store) SaveState(ctx context.Context, id string, serial int, enc, runID string) error {
	now := time.Now()
	if _, err := s.pool.Exec(ctx, `UPDATE tf_workspaces SET state_enc=$2, state_serial=$3, state_updated_at=$4 WHERE id=$1`, id, enc, serial, now); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO tf_state_versions (workspace_id, serial, state_enc, run_id, created_at) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (workspace_id, serial) DO UPDATE SET state_enc = $3, run_id = $4, created_at = $5`, id, serial, enc, runID, now)
	return err
}

type StateVersion struct {
	Serial    int       `json:"serial"`
	RunID     string    `json:"runId"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) StateVersions(ctx context.Context, id string) ([]StateVersion, error) {
	rows, err := s.pool.Query(ctx, `SELECT serial, run_id, created_at FROM tf_state_versions WHERE workspace_id = $1 ORDER BY serial DESC LIMIT 50`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StateVersion{}
	for rows.Next() {
		var v StateVersion
		if err := rows.Scan(&v.Serial, &v.RunID, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// --- variables

func (s *Store) Variables(ctx context.Context, id string) ([]Variable, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, kind, value_enc, sensitive, hcl FROM tf_variables WHERE workspace_id = $1 ORDER BY kind DESC, key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Variable{}
	for rows.Next() {
		var v Variable
		if err := rows.Scan(&v.Key, &v.Kind, &v.valueEnc, &v.Sensitive, &v.HCL); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReplaceVariables stores a workspace's whole variable set.
func (s *Store) ReplaceVariables(ctx context.Context, id string, vars []Variable) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "DELETE FROM tf_variables WHERE workspace_id = $1", id); err != nil {
		return err
	}
	for _, v := range vars {
		if _, err := tx.Exec(ctx, `INSERT INTO tf_variables (workspace_id, key, kind, value_enc, sensitive, hcl) VALUES ($1,$2,$3,$4,$5,$6)`,
			id, v.Key, v.Kind, v.valueEnc, v.Sensitive, v.HCL); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// --- runs

const runColumns = `id, workspace_id, workspace_name, kind, status, content_sha, log, error, plan_enc, summary, applies_run_id, applied_by_run_id, state_serial, started_at, finished_at, started_by, options, checks`

func scanRun(row pgx.Row, withLog bool) (*Run, error) {
	var r Run
	var summary, options, checks []byte
	if err := row.Scan(&r.ID, &r.WorkspaceID, &r.WorkspaceName, &r.Kind, &r.Status, &r.ContentSHA, &r.Log, &r.Error, &r.planEnc, &summary,
		&r.AppliesRunID, &r.AppliedByRun, &r.StateSerial, &r.StartedAt, &r.FinishedAt, &r.StartedBy, &options, &checks); err != nil {
		return nil, err
	}
	json.Unmarshal(summary, &r.Summary)
	json.Unmarshal(options, &r.Options)
	json.Unmarshal(checks, &r.Checks)
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
	summary, _ := json.Marshal(r.Summary)
	options, _ := json.Marshal(r.Options)
	checks, _ := json.Marshal(r.Checks)
	if r.Checks == nil {
		checks = []byte("[]")
	}
	_, err = s.pool.Exec(ctx, "INSERT INTO tf_runs ("+runColumns+") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NULL,$15,$16,$17)",
		r.ID, r.WorkspaceID, r.WorkspaceName, r.Kind, r.Status, r.ContentSHA, r.Log, r.Error, r.planEnc, summary, r.AppliesRunID, r.AppliedByRun, r.StateSerial, r.StartedAt,
		r.StartedBy, options, checks)
	return err
}

func (s *Store) SaveLog(ctx context.Context, id, log string) {
	s.pool.Exec(ctx, "UPDATE tf_runs SET log = $2 WHERE id = $1 AND status = 'running'", id, log)
}

func (s *Store) FinishRun(ctx context.Context, r *Run) error {
	now := time.Now()
	r.FinishedAt = &now
	summary, _ := json.Marshal(r.Summary)
	_, err := s.pool.Exec(ctx, "UPDATE tf_runs SET status=$2, log=$3, error=$4, plan_enc=$5, summary=$6, finished_at=$7 WHERE id=$1",
		r.ID, r.Status, r.Log, r.Error, r.planEnc, summary, now)
	return err
}

// MarkApplied links a plan run to the apply that applied it.
func (s *Store) MarkApplied(ctx context.Context, planID, applyID string) {
	s.pool.Exec(ctx, "UPDATE tf_runs SET applied_by_run_id = $2 WHERE id = $1", planID, applyID)
}

func (s *Store) Run(ctx context.Context, id string) (*Run, bool) {
	r, err := scanRun(s.pool.QueryRow(ctx, "SELECT "+runColumns+" FROM tf_runs WHERE id = $1", id), true)
	return r, err == nil
}

func (s *Store) Runs(ctx context.Context, workspaceID string, limit int) ([]*Run, error) {
	q, args := "SELECT "+runColumns+" FROM tf_runs", []any{}
	if workspaceID != "" {
		q += " WHERE workspace_id = $1"
		args = append(args, workspaceID)
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

func (s *Store) MarkInterrupted(ctx context.Context) {
	s.pool.Exec(ctx, `UPDATE tf_runs SET status = 'failed', error = 'the panel restarted while it ran', finished_at = now() WHERE status = 'running'`)
}
