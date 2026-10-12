// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/nginxctl"
	"devops-wrapper/internal/sshexec"
)

// Nginx configuration management: read the running configuration, add
// server blocks, edit or remove config files, and apply every change with
// nginx -t and a graceful reload.

const snapshotNginx = "nginx"

func (h *Handler) registerNginxRoutes(mux routeMux) {
	mux.HandleFunc("POST /api/services/{id}/nginx/inspect", h.handleNginxInspect)
	mux.HandleFunc("POST /api/services/{id}/nginx/preview", h.handleNginxPreview)
	mux.HandleFunc("POST /api/services/{id}/nginx/apply", h.handleNginxApply)
	mux.HandleFunc("GET /api/services/{id}/nginx/snapshot", h.handlePromSnapshot)
}

// nginxSession resolves the service and credentials, connects, and finds
// its Nginx container. Callers must Close the client.
func (h *Handler) nginxSession(w http.ResponseWriter, r *http.Request, password string) (*Service, *sshexec.Client, *dockerx.Container, bool) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	if svc.Type != "nginx" {
		http.Error(w, "this service is not an Nginx", http.StatusBadRequest)
		return nil, nil, nil, false
	}
	pw, err := h.resolvePassword(r.Context(), svc, password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, nil, false
	}
	client, err := sshexec.Dial(sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: pw})
	if err != nil {
		http.Error(w, "SSH connection failed: "+err.Error(), http.StatusBadGateway)
		return nil, nil, nil, false
	}
	var ct *dockerx.Container
	if svc.Container != "" {
		ct, err = nginxctl.Running(client, svc.Container)
	} else {
		ct, err = nginxctl.Discover(client, svc.DeployDir)
	}
	if err != nil {
		client.Close()
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, false
	}
	return svc, client, ct, true
}

func (h *Handler) handleNginxInspect(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.nginxSession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	report, err := nginxctl.Inspect(client, ct)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	h.snapshots.Save(r.Context(), svc.ID, snapshotNginx, report)
	writeJSON(w, report, nil)
}

type nginxPreviewRequest struct {
	Server        nginxctl.ServerSpec `json:"server"`
	NewFileDir    string              `json:"newFileDir"`
	ExistingPaths []string            `json:"existingPaths"`
}

type nginxPreviewResponse struct {
	Changes []previewChange `json:"changes"`
	Summary string          `json:"summary"`
}

// handleNginxPreview renders a new server block as a file, without
// touching the server. Apply re-checks everything before writing.
func (h *Handler) handleNginxPreview(w http.ResponseWriter, r *http.Request) {
	var req nginxPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !path.IsAbs(req.NewFileDir) {
		http.Error(w, "inspect the Nginx first", http.StatusBadRequest)
		return
	}
	spec := req.Server
	if err := spec.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file := path.Join(req.NewFileDir, nginxctl.FileName(spec.Names))
	for _, p := range req.ExistingPaths {
		if p == file {
			http.Error(w, fmt.Sprintf("%s already exists; edit that file instead, or use other domain names", file), http.StatusConflict)
			return
		}
	}
	kind := map[string]string{"proxy": "reverse proxy", "loadbalance": "load balancer", "redirect": "redirect"}[spec.Kind]
	writeJSON(w, nginxPreviewResponse{
		Changes: []previewChange{{Path: file, After: spec.Render(), IsNew: true}},
		Summary: fmt.Sprintf("Add %s for %s", kind, strings.Join(spec.Names, ", ")),
	}, nil)
}

type nginxApplyRequest struct {
	Password string            `json:"password"`
	Summary  string            `json:"summary"`
	Changes  []nginxctl.Change `json:"changes"`
}

func (h *Handler) handleNginxApply(w http.ResponseWriter, r *http.Request) {
	var req nginxApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.nginxSession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()

	started := time.Now()
	var out strings.Builder
	if req.Summary != "" {
		fmt.Fprintf(&out, "%s\n", req.Summary)
	}
	fmt.Fprintf(&out, "Nginx container: %s\n", ct.Name)
	err := nginxctl.Apply(client, ct, req.Changes, &out)

	resp := applyResponse{OK: err == nil}
	status := "success"
	if err != nil {
		resp.Error = err.Error()
		fmt.Fprintf(&out, "Failed: %v\n", err)
		status = "failed"
	}
	resp.Log = out.String()
	h.recordAction(r.Context(), svc, "nginx", status, resp.Log, started)
	writeJSON(w, resp, nil)
}
