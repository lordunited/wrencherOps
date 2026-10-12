// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/haproxyctl"
	"devops-wrapper/internal/sshexec"
)

// HAProxy configuration management: read the running configuration
// (sections, probe, health, live files), and apply edits with a
// pre-flight "haproxy -c" test and a graceful SIGUSR2 reload — the same
// investigate/probe/edit shape the Nginx panel has, adapted to HAProxy's
// flat config format and its own reload mechanism.

const snapshotHAProxy = "haproxy"

func (h *Handler) registerHAProxyRoutes(mux routeMux) {
	mux.HandleFunc("POST /api/services/{id}/haproxy/inspect", h.handleHAProxyInspect)
	mux.HandleFunc("POST /api/services/{id}/haproxy/preview", h.handleHAProxyPreview)
	mux.HandleFunc("POST /api/services/{id}/haproxy/test", h.handleHAProxyTest)
	mux.HandleFunc("POST /api/services/{id}/haproxy/apply", h.handleHAProxyApply)
	mux.HandleFunc("GET /api/services/{id}/haproxy/snapshot", h.handlePromSnapshot)
}

// haproxySession resolves the service and credentials, connects, and finds
// its HAProxy container. Callers must Close the client.
func (h *Handler) haproxySession(w http.ResponseWriter, r *http.Request, password string) (*Service, *sshexec.Client, *dockerx.Container, bool) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	if svc.Type != "haproxy" {
		http.Error(w, "this service is not an HAProxy", http.StatusBadRequest)
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
		ct, err = haproxyctl.Running(client, svc.Container)
	} else {
		ct, err = haproxyctl.Discover(client, svc.DeployDir)
	}
	if err != nil {
		client.Close()
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, false
	}
	return svc, client, ct, true
}

func (h *Handler) handleHAProxyInspect(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.haproxySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	report, err := haproxyctl.Inspect(client, ct)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	h.snapshots.Save(r.Context(), svc.ID, snapshotHAProxy, report)
	writeJSON(w, report, nil)
}

type haproxyPreviewRequest struct {
	Site        haproxyctl.SiteSpec `json:"site"`
	Files       []dockerx.File      `json:"files"`
	ConfigPaths []string            `json:"configPaths"`
}

// handleHAProxyPreview renders a new site against the files the client got
// from inspect, without touching the server. Apply re-checks that each
// file is unchanged, and dry-runs the result, before writing anything.
func (h *Handler) handleHAProxyPreview(w http.ResponseWriter, r *http.Request) {
	var req haproxyPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "inspect the HAProxy first", http.StatusBadRequest)
		return
	}
	spec := req.Site
	if err := spec.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	changes, summary, err := spec.Render(req.Files, haproxyctl.MainFile(req.Files, req.ConfigPaths))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	resp := previewResponse{Summary: summary, Changes: []previewChange{}}
	for _, c := range changes {
		resp.Changes = append(resp.Changes, previewChange{Path: c.Path, Before: c.Before, After: c.After, BaseSHA: c.BaseSHA})
	}
	writeJSON(w, resp, nil)
}

type haproxyTestRequest struct {
	Password string              `json:"password"`
	Changes  []haproxyctl.Change `json:"changes"`
}

type haproxyTestResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

// handleHAProxyTest dry-runs changes against a scratch copy of the config
// inside the container (haproxyctl.Validate) without writing anything real
// — lets the panel check syntax before committing to Apply.
func (h *Handler) handleHAProxyTest(w http.ResponseWriter, r *http.Request) {
	var req haproxyTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Changes) == 0 {
		http.Error(w, "nothing to test", http.StatusBadRequest)
		return
	}
	_, client, ct, ok := h.haproxySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	ok2, output, err := haproxyctl.Validate(client, ct, req.Changes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, haproxyTestResponse{OK: ok2, Output: output}, nil)
}

type haproxyApplyRequest struct {
	Password string              `json:"password"`
	Summary  string              `json:"summary"`
	Changes  []haproxyctl.Change `json:"changes"`
}

func (h *Handler) handleHAProxyApply(w http.ResponseWriter, r *http.Request) {
	var req haproxyApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.haproxySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()

	started := time.Now()
	var out strings.Builder
	if req.Summary != "" {
		fmt.Fprintf(&out, "%s\n", req.Summary)
	}
	fmt.Fprintf(&out, "HAProxy container: %s\n", ct.Name)
	err := haproxyctl.Apply(client, ct, req.Changes, &out)

	resp := applyResponse{OK: err == nil}
	status := "success"
	if err != nil {
		resp.Error = err.Error()
		fmt.Fprintf(&out, "Failed: %v\n", err)
		status = "failed"
	}
	resp.Log = out.String()
	h.recordAction(r.Context(), svc, "haproxy", status, resp.Log, started)
	writeJSON(w, resp, nil)
}
