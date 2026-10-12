// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"devops-wrapper/internal/access"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/randid"
	"devops-wrapper/internal/sshexec"
)

// A custom compose app is one docker-compose.yml deployed, identically, to
// several servers. Each server is an ordinary service (type "docker") whose
// AppID points at the app, so deploy jobs, start/stop/down, sync, health
// and history work per server with no app-specific machinery; the app holds
// the compose and deploy folder they share, and pushes every change to
// each server's copy so they can't drift.

// App is a custom compose app and, when read through the API, its servers.
type App struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Manifest string `json:"manifest"`
	// ComposeFile is the name the compose file is written under in the
	// deploy folder: docker-compose.yml, or an imported project's own
	// compose.yaml / compose.yml / docker-compose.yaml.
	ComposeFile string `json:"composeFile"`
	// Template is what the app was created from ("signoz"), or "".
	Template  string               `json:"template,omitempty"`
	Env       string               `json:"env"`
	Files     []deploy.ProjectFile `json:"files"`
	DeployDir string               `json:"deployDir"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
	Servers   []*Service           `json:"servers"`
	// Analysis describes the project's volumes and what's likely to break;
	// computed when the app is read, never stored.
	Analysis *deploy.Analysis `json:"analysis,omitempty"`
}

// projectFiles is what a deploy writes besides docker-compose.yml: the
// .env (when there is one) and every other file of the project.
func (a *App) projectFiles() []deploy.ProjectFile {
	var files []deploy.ProjectFile
	if strings.TrimSpace(a.Env) != "" {
		files = append(files, deploy.ProjectFile{Path: ".env", Content: a.Env})
	}
	return append(files, a.Files...)
}

// deployFiles is everything a deploy writes besides the compose file: the
// project's files plus, for SigNoz, the binaries the panel bundles for it.
func (a *App) deployFiles() []deploy.ProjectFile {
	files := a.projectFiles()
	if a.Template == deploy.SigNozTemplate {
		files = append(files, deploy.SigNozBundledFiles()...)
	}
	return files
}

func (a *App) analyze() {
	files := a.Files
	if a.Template == deploy.SigNozTemplate {
		files = append(append([]deploy.ProjectFile{}, files...), deploy.SigNozBundledFiles()...)
	}
	an := deploy.AnalyzeProject(a.Manifest, a.Env, files, a.DeployDir)
	a.Analysis = &an
}

// validateFiles checks a project's files; .env has its own field, so it
// isn't accepted among them.
func validateFiles(files []deploy.ProjectFile) error {
	for _, f := range files {
		if f.Path == ".env" {
			return fmt.Errorf(".env has its own editor; it can't also be a project file")
		}
	}
	return deploy.ValidateProjectFiles(files)
}

// AppStore persists apps in Postgres.
type AppStore struct {
	pool *pgxpool.Pool
}

func NewAppStore(pool *pgxpool.Pool) *AppStore {
	return &AppStore{pool: pool}
}

const appColumns = `id, name, manifest, deploy_dir, created_at, updated_at, env, files, compose_file, template`

func scanApp(row pgx.Row) (*App, error) {
	var a App
	var files []byte
	if err := row.Scan(&a.ID, &a.Name, &a.Manifest, &a.DeployDir, &a.CreatedAt, &a.UpdatedAt, &a.Env, &files, &a.ComposeFile, &a.Template); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(files, &a.Files); err != nil || a.Files == nil {
		a.Files = []deploy.ProjectFile{}
	}
	a.Servers = []*Service{}
	return &a, nil
}

func (s *AppStore) List(ctx context.Context) ([]*App, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+appColumns+" FROM compose_apps ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AppStore) Get(ctx context.Context, id string) (*App, bool) {
	a, err := scanApp(s.pool.QueryRow(ctx, "SELECT "+appColumns+" FROM compose_apps WHERE id = $1", id))
	if err != nil {
		return nil, false
	}
	return a, true
}

func (s *AppStore) Create(ctx context.Context, a *App) error {
	id, err := randid.New(16)
	if err != nil {
		return err
	}
	a.ID = id
	a.CreatedAt = time.Now()
	a.UpdatedAt = a.CreatedAt
	if a.Files == nil {
		a.Files = []deploy.ProjectFile{}
	}
	if a.ComposeFile == "" {
		a.ComposeFile = "docker-compose.yml"
	}
	files, err := json.Marshal(a.Files)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, "INSERT INTO compose_apps ("+appColumns+") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
		a.ID, a.Name, a.Manifest, a.DeployDir, a.CreatedAt, a.UpdatedAt, a.Env, files, a.ComposeFile, a.Template)
	return err
}

// Update reads the app under id, applies mutate, and writes it back in one
// transaction.
func (s *AppStore) Update(ctx context.Context, id string, mutate func(*App)) (*App, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	a, err := scanApp(tx.QueryRow(ctx, "SELECT "+appColumns+" FROM compose_apps WHERE id = $1 FOR UPDATE", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("app %s not found", id)
		}
		return nil, err
	}
	mutate(a)
	a.UpdatedAt = time.Now()
	if a.Files == nil {
		a.Files = []deploy.ProjectFile{}
	}
	files, err := json.Marshal(a.Files)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE compose_apps SET name=$1, manifest=$2, deploy_dir=$3, updated_at=$4, env=$5, files=$6 WHERE id=$7",
		a.Name, a.Manifest, a.DeployDir, a.UpdatedAt, a.Env, files, a.ID); err != nil {
		return nil, err
	}
	return a, tx.Commit(ctx)
}

func (s *AppStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM compose_apps WHERE id = $1", id)
	return err
}

func (h *Handler) registerAppRoutes(mux routeMux) {
	mux.HandleFunc("GET /api/apps", h.handleListApps)
	mux.HandleFunc("POST /api/apps", h.handleCreateApp)
	mux.HandleFunc("GET /api/apps/{id}", h.handleGetApp)
	mux.HandleFunc("PUT /api/apps/{id}", h.handleUpdateApp)
	mux.HandleFunc("DELETE /api/apps/{id}", h.handleDeleteApp)
	mux.HandleFunc("POST /api/apps/{id}/servers", h.handleAddAppServer)
	mux.HandleFunc("GET /api/apps/templates/signoz", h.handleSigNozTemplate)
	mux.HandleFunc("POST /api/apps/detect", h.handleDetectProjects)
	mux.HandleFunc("POST /api/apps/detect/add", h.handleScanAndAdd)
	mux.HandleFunc("POST /api/apps/import", h.handleImportProject)
	mux.HandleFunc("POST /api/apps/watch", h.handleWatchProject)
	mux.HandleFunc("POST /api/apps/envfile", h.handleReadEnvFile)
	mux.HandleFunc("POST /api/apps/envfile/save", h.handleSaveEnvFile)
}

// withServers fills in each app's servers from one read of all services.
func (h *Handler) withServers(ctx context.Context, apps ...*App) error {
	all, err := h.services.List(ctx)
	if err != nil {
		return err
	}
	byID := map[string]*App{}
	for _, a := range apps {
		byID[a.ID] = a
	}
	for _, s := range all {
		if a, ok := byID[s.AppID]; ok && s.AppID != "" {
			a.Servers = append(a.Servers, s)
		}
	}
	return nil
}

// handleListApps lists the apps in the tabs the user can see.
func (h *Handler) handleListApps(w http.ResponseWriter, r *http.Request) {
	apps, err := h.apps.List(r.Context())
	if err == nil {
		apps = access.Filter(r.Context(), apps, appTab)
		err = h.withServers(r.Context(), apps...)
	}
	writeJSON(w, apps, err)
}

func (h *Handler) handleGetApp(w http.ResponseWriter, r *http.Request) {
	a, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	a.analyze()
	writeJSON(w, a, h.withServers(r.Context(), a))
}

// appServer is one server to deploy an app to. Password is optional: when
// given it's saved (encrypted) so Deploy to all needs no prompt.
type appServer struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	// CopyLoginFrom is a service on a server the panel already knows: its
	// host, port, user and saved (encrypted) password are reused, so adding
	// that server again needs no password typed. The saved password is
	// copied server-side and never sent to the browser.
	CopyLoginFrom string `json:"copyLoginFrom"`
	savedPassword string // from CopyLoginFrom, already encrypted
	verified      bool   // Password just logged in
}

// resolve fills in a server given by CopyLoginFrom, then normalizes it.
func (h *Handler) resolveServer(ctx context.Context, s *appServer) error {
	if s.CopyLoginFrom != "" {
		from, ok := h.services.Get(ctx, s.CopyLoginFrom)
		if !ok {
			return fmt.Errorf("the server to copy the login from no longer exists")
		}
		s.Host, s.Port, s.Username, s.savedPassword = from.Host, from.Port, from.Username, from.EncryptedPassword
	}
	if err := s.normalize(); err != nil {
		return err
	}
	if s.Password != "" {
		// Saved only once it has logged in (for every service on this login);
		// one the server can't be reached to check isn't kept, and the first
		// deploy asks for it again.
		client, err := sshexec.Dial(sshexec.Target{Host: s.Host, Port: s.Port, Username: s.Username, Password: s.Password})
		if isAuthError(err) {
			return fmt.Errorf("%s@%s refused that SSH password; check it, or leave it empty to enter it when deploying", s.Username, s.Host)
		}
		if err != nil {
			s.Password = ""
			return nil
		}
		client.Close()
		s.verified = true
	}
	return nil
}

func (s *appServer) normalize() error {
	s.Host = strings.TrimSpace(s.Host)
	s.Username = strings.TrimSpace(s.Username)
	if s.Port == 0 {
		s.Port = 22
	}
	if s.Host == "" || s.Username == "" {
		return fmt.Errorf("every server needs a host and an SSH user")
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("%d is not a valid SSH port", s.Port)
	}
	return nil
}

// folderTaken reports another panel service that already deploys into dir
// on host — two stacks in one folder would overwrite each other's compose
// file and data. except is a service allowed to be there (the one being
// changed).
func (h *Handler) folderTaken(ctx context.Context, host, dir, except string) (*Service, error) {
	all, err := h.services.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range all {
		if s.ID != except && s.Host == host && strings.TrimRight(s.DeployDir, "/") == strings.TrimRight(dir, "/") {
			return s, nil
		}
	}
	return nil, nil
}

// addServer creates one server of app a.
func (h *Handler) addServer(ctx context.Context, a *App, srv appServer) (*Service, error) {
	if other, err := h.folderTaken(ctx, srv.Host, a.DeployDir, ""); err != nil {
		return nil, err
	} else if other != nil {
		return nil, fmt.Errorf("%s already runs %q in %s; give this app another deploy folder", srv.Host, other.Name, a.DeployDir)
	}
	svc := &Service{
		Name: a.Name, Type: "docker", Host: srv.Host, Port: srv.Port, Username: srv.Username,
		DeployDir: a.DeployDir, Manifest: a.Manifest, Status: StatusPending, AppID: a.ID,
	}
	if srv.Password != "" {
		enc, err := h.secrets.Encrypt(srv.Password)
		if err != nil {
			return nil, err
		}
		svc.EncryptedPassword = enc
	} else {
		svc.EncryptedPassword = srv.savedPassword
	}
	if err := h.services.Create(ctx, svc); err != nil {
		return nil, err
	}
	if srv.verified {
		h.rememberLogin(ctx, srv.Host, srv.Port, srv.Username, srv.Password)
	}
	svc.HasSavedPassword = svc.EncryptedPassword != ""
	return svc, nil
}

type createAppRequest struct {
	Name      string               `json:"name"`
	DeployDir string               `json:"deployDir"`
	Manifest  string               `json:"manifest"`
	Env       string               `json:"env"`
	Files     []deploy.ProjectFile `json:"files"`
	Servers   []appServer          `json:"servers"`
	// Optional: the compose file's name (docker-compose.yml when empty), and
	// the template the app comes from.
	ComposeFile string `json:"composeFile"`
	Template    string `json:"template"`
}

func (h *Handler) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.TrimSpace(req.Manifest) == "" {
		http.Error(w, "the app needs a name and a docker-compose.yml", http.StatusBadRequest)
		return
	}
	if len(req.Servers) == 0 {
		http.Error(w, "add at least one server", http.StatusBadRequest)
		return
	}
	if err := validateFiles(req.Files); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ComposeFile != "" && !deploy.IsComposeFileName(req.ComposeFile) {
		http.Error(w, fmt.Sprintf("%q isn't a compose file name; use one of %s", req.ComposeFile, strings.Join(deploy.ComposeFileNames, ", ")), http.StatusBadRequest)
		return
	}
	if req.Template != "" && req.Template != deploy.SigNozTemplate {
		http.Error(w, fmt.Sprintf("unknown template %q", req.Template), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.DeployDir) == "" {
		req.DeployDir = "/opt/" + slugify(req.Name)
	}
	seen := map[string]bool{}
	for i := range req.Servers {
		if err := h.resolveServer(r.Context(), &req.Servers[i]); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key := fmt.Sprintf("%s:%d", req.Servers[i].Host, req.Servers[i].Port)
		if seen[key] {
			http.Error(w, key+" is listed twice", http.StatusBadRequest)
			return
		}
		seen[key] = true
		if other, err := h.folderTaken(r.Context(), req.Servers[i].Host, req.DeployDir, ""); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if other != nil {
			http.Error(w, fmt.Sprintf("%s already runs %q in %s; give this app another deploy folder", req.Servers[i].Host, other.Name, req.DeployDir), http.StatusConflict)
			return
		}
	}

	a := &App{Name: req.Name, Manifest: req.Manifest, ComposeFile: req.ComposeFile, Template: req.Template, Env: req.Env, Files: req.Files, DeployDir: strings.TrimRight(req.DeployDir, "/")}
	if err := h.apps.Create(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.Servers = []*Service{}
	for _, srv := range req.Servers {
		svc, err := h.addServer(r.Context(), a, srv)
		if err != nil {
			http.Error(w, fmt.Sprintf("the app was created, but adding %s failed: %v", srv.Host, err), http.StatusInternalServerError)
			return
		}
		a.Servers = append(a.Servers, svc)
	}
	a.analyze()
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, a, nil)
}

type updateAppRequest struct {
	Name      *string               `json:"name"`
	Manifest  *string               `json:"manifest"`
	Env       *string               `json:"env"`
	Files     *[]deploy.ProjectFile `json:"files"`
	DeployDir *string               `json:"deployDir"`
}

// handleUpdateApp saves the app and pushes the change to every server's
// copy. Nothing is redeployed: the servers keep running what they have
// until Deploy.
func (h *Handler) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	var req updateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	current, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if err := h.withServers(r.Context(), current); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" || req.Manifest != nil && strings.TrimSpace(*req.Manifest) == "" {
		http.Error(w, "the app needs a name and a docker-compose.yml", http.StatusBadRequest)
		return
	}
	if req.Files != nil {
		if err := validateFiles(*req.Files); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.DeployDir != nil {
		dir := strings.TrimRight(strings.TrimSpace(*req.DeployDir), "/")
		if dir == "" || !strings.HasPrefix(dir, "/") {
			http.Error(w, "the deploy folder must be an absolute path", http.StatusBadRequest)
			return
		}
		req.DeployDir = &dir
		for _, s := range current.Servers {
			if other, err := h.folderTaken(r.Context(), s.Host, dir, s.ID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else if other != nil {
				http.Error(w, fmt.Sprintf("%s already runs %q in %s", s.Host, other.Name, dir), http.StatusConflict)
				return
			}
		}
	}

	a, err := h.apps.Update(r.Context(), current.ID, func(a *App) {
		if req.Name != nil {
			a.Name = strings.TrimSpace(*req.Name)
		}
		if req.Manifest != nil {
			a.Manifest = *req.Manifest
		}
		if req.Env != nil {
			a.Env = *req.Env
		}
		if req.Files != nil {
			a.Files = *req.Files
		}
		if req.DeployDir != nil {
			a.DeployDir = *req.DeployDir
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, s := range current.Servers {
		if _, err := h.services.Update(r.Context(), s.ID, func(s *Service) {
			s.Name, s.Manifest, s.DeployDir = a.Name, a.Manifest, a.DeployDir
		}); err != nil {
			http.Error(w, fmt.Sprintf("saved, but updating %s's copy failed: %v; save again to retry", s.Host, err), http.StatusInternalServerError)
			return
		}
	}
	a.analyze()
	writeJSON(w, a, h.withServers(r.Context(), a))
}

// handleDeleteApp removes an app with no servers left. Servers are removed
// one by one with their own Delete, which takes the stack down and removes
// its folder on that server — never implicitly, from here.
func (h *Handler) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	a, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if err := h.withServers(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n := len(a.Servers); n > 0 {
		http.Error(w, fmt.Sprintf("this app still has %d server(s); delete each one first (that takes its stack down and removes its folder)", n), http.StatusConflict)
		return
	}
	if err := h.apps.Delete(r.Context(), a.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleAddAppServer(w http.ResponseWriter, r *http.Request) {
	var srv appServer
	if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.resolveServer(r.Context(), &srv); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	svc, err := h.addServer(r.Context(), a, srv)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// handleSigNozTemplate returns SigNoz as a ready-to-create app: its compose,
// config files and a .env with a password made for this app alone.
func (h *Handler) handleSigNozTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := deploy.SigNozComposeTemplate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, t, nil)
}
