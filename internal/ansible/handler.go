// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/servers"
)

type Handler struct {
	store   *Store
	runner  *Runner
	servers *servers.Store
	secrets *secretbox.Box
}

func NewHandler(store *Store, serverStore *servers.Store, secrets *secretbox.Box) *Handler {
	return &Handler{store: store, runner: NewRunner(store), servers: serverStore, secrets: secrets}
}

// PathPrefixes are the URL prefixes RegisterRoutes serves.
func (h *Handler) PathPrefixes() []string {
	return []string{"/api/ansible", "/api/ansible/"}
}

func (h *Handler) RegisterRoutes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("GET /api/ansible/templates", h.handleTemplates)
	mux.HandleFunc("GET /api/ansible/modules", h.handleModules)
	mux.HandleFunc("GET /api/ansible/modules/{name}", h.handleModule)
	mux.HandleFunc("GET /api/ansible/playbooks", h.handleList)
	mux.HandleFunc("POST /api/ansible/playbooks", h.handleCreate)
	mux.HandleFunc("POST /api/ansible/playbooks/upload", h.handleUpload)
	mux.HandleFunc("GET /api/ansible/playbooks/{id}", h.handleGet)
	mux.HandleFunc("PUT /api/ansible/playbooks/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/ansible/playbooks/{id}", h.handleDelete)
	mux.HandleFunc("POST /api/ansible/playbooks/{id}/runs", h.handleStartRun)
	mux.HandleFunc("GET /api/ansible/runs", h.handleRuns)
	mux.HandleFunc("GET /api/ansible/runs/{id}", h.handleRun)
	mux.HandleFunc("POST /api/ansible/runs/{id}/cancel", h.handleCancel)
}

// Start marks runs a restart interrupted as failed, and reads the module
// catalog in the background.
func (h *Handler) Start(ctx context.Context) {
	h.store.MarkInterrupted(ctx)
	WarmModules()
}

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
	}
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) handleTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, Templates, 0)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Playbooks(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs, _ := h.store.Runs(r.Context(), "", 500)
	last := map[string]*Run{}
	for _, run := range runs {
		if _, ok := last[run.PlaybookID]; !ok {
			last[run.PlaybookID] = run
		}
	}
	for _, p := range list {
		p.LastRun = last[p.ID]
		p.Files = nil // the list doesn't need them
	}
	writeJSON(w, list, 0)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	p, ok := h.store.Playbook(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "playbook not found", http.StatusNotFound)
		return
	}
	writeJSON(w, p, 0)
}

type playbookRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Files       *[]File `json:"files"`
	Entry       *string `json:"entry"`
	Template    string  `json:"template"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req playbookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	p := &Playbook{}
	if req.Template != "" {
		for _, t := range Templates {
			if t.Key == req.Template {
				p.Name, p.Description, p.Files = t.Name, t.Description, append([]File(nil), t.Files...)
			}
		}
		if p.Files == nil {
			http.Error(w, "unknown template "+req.Template, http.StatusBadRequest)
			return
		}
	}
	if req.Name != nil {
		p.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		p.Description = strings.TrimSpace(*req.Description)
	}
	if req.Files != nil {
		p.Files = *req.Files
	}
	p.Entry = EntryPoint
	if req.Entry != nil && *req.Entry != "" {
		p.Entry = *req.Entry
	}
	if p.Name == "" {
		http.Error(w, "the playbook needs a name", http.StatusBadRequest)
		return
	}
	if err := ValidateFiles(p.Files, p.Entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.CreatePlaybook(r.Context(), p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, p, http.StatusCreated)
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.store.Playbook(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "playbook not found", http.StatusNotFound)
		return
	}
	var req playbookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name != nil {
		if p.Name = strings.TrimSpace(*req.Name); p.Name == "" {
			http.Error(w, "the playbook needs a name", http.StatusBadRequest)
			return
		}
	}
	if req.Description != nil {
		p.Description = strings.TrimSpace(*req.Description)
	}
	if req.Files != nil {
		p.Files = *req.Files
	}
	if req.Entry != nil && *req.Entry != "" {
		p.Entry = *req.Entry
	}
	if req.Files != nil || req.Entry != nil {
		if err := ValidateFiles(p.Files, p.Entry); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := h.store.UpdatePlaybook(r.Context(), p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, p, 0)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeletePlaybook(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type runRequest struct {
	ServerIDs []string `json:"serverIds"`
	Mode      string   `json:"mode"` // "check" | "apply"
	ExtraVars string   `json:"extraVars"`
	// Approve, the playbook's name typed out, lets an apply go ahead without
	// a passed dry run (some playbooks can't dry-run: a task that needs an
	// earlier one to have really happened).
	Approve string `json:"approve"`
}

// logins resolves the servers to log in to, with their saved passwords: the
// server's own, else one saved for the same login on its services.
func (h *Handler) logins(ctx context.Context, ids []string) ([]Login, error) {
	var out []Login
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		s, ok := h.servers.Get(ctx, id)
		if !ok {
			return nil, fmt.Errorf("server %s isn't in the Servers list", id)
		}
		enc := s.EncryptedPassword
		if enc == "" {
			if more, _ := h.servers.ServicePasswords(ctx, s.Host, s.Port, s.Username); len(more) > 0 {
				enc = more[0]
			}
		}
		if enc == "" {
			return nil, fmt.Errorf("no SSH password is saved for %s (%s@%s); enter it on its Servers page", s.Name, s.Username, s.Host)
		}
		pw, err := h.secrets.Decrypt(enc)
		if err != nil {
			return nil, err
		}
		out = append(out, Login{Target: Target{ServerID: s.ID, Name: s.Name, Host: s.Host, User: s.Username}, Port: s.Port, Password: pw})
	}
	return out, nil
}

func (h *Handler) handleStartRun(w http.ResponseWriter, r *http.Request) {
	pb, ok := h.store.Playbook(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "playbook not found", http.StatusNotFound)
		return
	}
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Mode != "check" && req.Mode != "apply" {
		http.Error(w, "mode must be check (a dry run) or apply", http.StatusBadRequest)
		return
	}
	if len(req.ServerIDs) == 0 {
		http.Error(w, "pick at least one server", http.StatusBadRequest)
		return
	}
	if err := ValidateExtraVars(req.ExtraVars); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := ValidateFiles(pb.Files, pb.Entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	logins, err := h.logins(r.Context(), req.ServerIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Mode == "apply" {
		targets := make([]Target, len(logins))
		for i, l := range logins {
			targets[i] = l.Target
		}
		_, err := h.store.PassedDryRun(r.Context(), pb.ID, ContentSHA(pb.Files, pb.Entry, req.ExtraVars), targets)
		switch {
		case errors.Is(err, errNoDryRun) && strings.TrimSpace(req.Approve) != pb.Name:
			http.Error(w, "run a dry run of this exact playbook, extra vars and servers first — it must pass before applying (or type the playbook's name to apply without one)", http.StatusConflict)
			return
		case err != nil && !errors.Is(err, errNoDryRun):
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	run, err := h.runner.Start(r.Context(), pb, logins, req.Mode, req.ExtraVars)
	if errors.Is(err, ErrBadTarget) {
		http.Error(w, err.Error()+"; fix it on the server's page", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, run, http.StatusCreated)
}

func (h *Handler) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.store.Runs(r.Context(), r.URL.Query().Get("playbook"), 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, runs, 0)
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	run, ok := h.store.Run(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	writeJSON(w, run, 0)
}

func (h *Handler) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !h.runner.Cancel(r.PathValue("id")) {
		http.Error(w, "that run isn't running", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type uploadRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Either an archive (base64, with its file name for the format)…
	Filename string `json:"filename"`
	Archive  []byte `json:"archive"`
	// …or files read in the browser (single files or a folder).
	Files []File `json:"files"`
	// Entry is the file to run; "" detects it.
	Entry string `json:"entry"`
}

type uploadResponse struct {
	Playbook *Playbook `json:"playbook,omitempty"`
	Notes    []string  `json:"notes"`
	// Candidates, when the file to run can't be told, for the uploader to
	// pick one of and send again as Entry.
	Candidates []string `json:"candidates,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// handleUpload creates a playbook from an upload: an archive (.zip, .tar,
// .tar.gz, .tgz) unpacked here, or files read in the browser. Clutter and
// files the panel writes itself are left out (and listed in notes); the
// file to run is detected unless given.
func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*maxArchive+maxBytes)
	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid upload (too big, or not JSON): "+err.Error(), http.StatusBadRequest)
		return
	}
	var files []File
	var notes []string
	if len(req.Archive) > 0 {
		var err error
		if files, notes, err = ExtractArchive(req.Filename, req.Archive); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		files, notes = CleanUpload(req.Files)
	}
	if notes == nil {
		notes = []string{}
	}
	if len(files) == 0 {
		writeJSON(w, uploadResponse{Notes: notes, Error: "the upload has no files that can be part of a playbook"}, http.StatusBadRequest)
		return
	}
	entry := strings.TrimSpace(req.Entry)
	if entry == "" {
		detected, candidates, err := DetectEntry(files)
		if err != nil {
			writeJSON(w, uploadResponse{Notes: notes, Candidates: candidates, Error: err.Error()}, http.StatusConflict)
			return
		}
		entry = detected
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(path.Base(req.Filename), ".gz"), ".tar"), path.Ext(req.Filename))
	}
	if name == "" || name == "." {
		name = "Uploaded playbook"
	}
	p := &Playbook{Name: name, Description: strings.TrimSpace(req.Description), Files: files, Entry: entry}
	if err := ValidateFiles(p.Files, p.Entry); err != nil {
		writeJSON(w, uploadResponse{Notes: notes, Error: err.Error()}, http.StatusBadRequest)
		return
	}
	if err := h.store.CreatePlaybook(r.Context(), p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, uploadResponse{Playbook: p, Notes: notes}, http.StatusCreated)
}
