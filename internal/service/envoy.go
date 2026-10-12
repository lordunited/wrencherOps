// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/envoyctl"
	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// Envoy configuration management: read the bootstrap config and Envoy's
// live state (readiness, routes, clusters, per-endpoint health), and apply
// edits — a full file, or a route added for you — only after "envoy --mode
// validate" accepts them. Envoy can't reload a static config, so applying
// restarts it, and rolls back if it doesn't come up ready.

const snapshotEnvoy = "envoy"

func (h *Handler) registerEnvoyRoutes(mux routeMux) {
	mux.HandleFunc("POST /api/services/{id}/envoy/inspect", h.handleEnvoyInspect)
	mux.HandleFunc("POST /api/services/{id}/envoy/preview", h.handleEnvoyPreview)
	mux.HandleFunc("POST /api/services/{id}/envoy/test", h.handleEnvoyTest)
	mux.HandleFunc("POST /api/services/{id}/envoy/apply", h.handleEnvoyApply)
	mux.HandleFunc("GET /api/services/{id}/envoy/snapshot", h.handlePromSnapshot)
}

// envoySession resolves the service and credentials, connects, and finds
// its Envoy container. Callers must Close the client.
func (h *Handler) envoySession(w http.ResponseWriter, r *http.Request, password string) (*Service, *sshexec.Client, *dockerx.Container, bool) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	if svc.Type != "envoy" {
		http.Error(w, "this service is not an Envoy", http.StatusBadRequest)
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
		ct, err = envoyctl.Running(client, svc.Container)
	} else {
		ct, err = envoyctl.Discover(client, svc.DeployDir)
	}
	if err != nil {
		client.Close()
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, false
	}
	return svc, client, ct, true
}

func (h *Handler) handleEnvoyInspect(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.envoySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	report, err := envoyctl.Inspect(client, ct)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	h.snapshots.Save(r.Context(), svc.ID, snapshotEnvoy, report)
	writeJSON(w, report, nil)
}

type envoyPreviewRequest struct {
	Route envoyctl.RouteSpec `json:"route"`
	File  dockerx.File       `json:"file"`
}

// handleEnvoyPreview renders a new route against the config the client got
// from inspect, without touching the server.
func (h *Handler) handleEnvoyPreview(w http.ResponseWriter, r *http.Request) {
	var req envoyPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.File.Path == "" {
		http.Error(w, "inspect the Envoy first", http.StatusBadRequest)
		return
	}
	spec := req.Route
	if err := spec.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	after, summary, err := spec.Render(req.File.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, previewResponse{Summary: summary, Changes: []previewChange{{Path: req.File.Path, Before: req.File.Content, After: after, BaseSHA: req.File.SHA}}}, nil)
}

type envoyTestRequest struct {
	Password string          `json:"password"`
	Change   envoyctl.Change `json:"change"`
}

type envoyTestResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

// handleEnvoyTest validates a config in a throwaway container, writing
// nothing — so the panel can check it before Apply.
func (h *Handler) handleEnvoyTest(w http.ResponseWriter, r *http.Request) {
	var req envoyTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	_, client, ct, ok := h.envoySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	ok2, output, err := envoyctl.Validate(client, ct, req.Change.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, envoyTestResponse{OK: ok2, Output: output}, nil)
}

type envoyApplyRequest struct {
	Password string          `json:"password"`
	Summary  string          `json:"summary"`
	Change   envoyctl.Change `json:"change"`
}

func (h *Handler) handleEnvoyApply(w http.ResponseWriter, r *http.Request) {
	var req envoyApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, ct, ok := h.envoySession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()

	started := time.Now()
	var out strings.Builder
	if req.Summary != "" {
		fmt.Fprintf(&out, "%s\n", req.Summary)
	}
	fmt.Fprintf(&out, "Envoy container: %s\n", ct.Name)
	err := envoyctl.Apply(client, ct, req.Change, &out)

	resp := applyResponse{OK: err == nil}
	status := "success"
	if err != nil {
		resp.Error = err.Error()
		fmt.Fprintf(&out, "Failed: %v\n", err)
		status = "failed"
	}
	resp.Log = out.String()
	h.recordAction(r.Context(), svc, "envoy", status, resp.Log, started)
	writeJSON(w, resp, nil)
}

type createEnvoyRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`

	HTTPPort      int      `json:"httpPort"`
	ImageTag      string   `json:"imageTag"`
	ContainerName string   `json:"containerName"`
	Networks      []string `json:"networks"`
	ExtraVolumes  []string `json:"extraVolumes"`
	CPULimit      string   `json:"cpuLimit"`
	MemoryLimit   string   `json:"memoryLimit"`
}

func (h *Handler) handleCreateEnvoy(w http.ResponseWriter, r *http.Request) {
	var req createEnvoyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/envoy"
	}
	if req.HTTPPort == 0 {
		req.HTTPPort = 80
	}
	if req.ImageTag == "" {
		req.ImageTag = "v1.39-latest"
	}
	if req.ContainerName == "" {
		req.ContainerName = "envoy"
	}
	if req.Name == "" {
		req.Name = req.Host
	}
	var missing []string
	if req.Host == "" {
		missing = append(missing, "host")
	}
	if req.Username == "" {
		missing = append(missing, "username")
	}
	if len(missing) > 0 {
		http.Error(w, "missing or invalid fields: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}
	manifest := deploy.EnvoyConfig{
		HTTPPort: req.HTTPPort, ImageTag: req.ImageTag, DeployDir: strings.TrimRight(req.DeployDir, "/"),
		ContainerName: req.ContainerName, Networks: req.Networks, ExtraVolumes: req.ExtraVolumes,
		CPULimit: req.CPULimit, MemoryLimit: req.MemoryLimit,
	}.ComposeYAML()
	svc := &Service{
		Name: req.Name, Type: "envoy", Host: req.Host, Port: req.Port, Username: req.Username,
		DeployDir: req.DeployDir, Manifest: manifest, Status: StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// connectEnvoy attaches to an existing Envoy container and logs what its
// health check will read: its admin API's verdict and endpoints' health.
func (h *Handler) connectEnvoy(w http.ResponseWriter, r *http.Request, req connectRequest, client *sshexec.Client) {
	ct, err := dockerx.Inspect(client, req.Container)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !envoyctl.IsEnvoyImage(ct.Image) {
		http.Error(w, ct.Name+" doesn't run an Envoy image", http.StatusBadRequest)
		return
	}
	project, workingDir, files := ct.Compose()
	manifest := "# This Envoy was not started with Docker Compose.\n"
	if len(files) > 0 {
		if content := promctl.ReadHostFiles(client, files); content != "" {
			manifest = content
		} else {
			manifest = "# The compose file(s) could not be read: " + strings.Join(files, ", ") + "\n"
		}
	}
	encrypted, err := h.secrets.Encrypt(req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = project
	}
	if name == "" {
		name = ct.Name
	}
	svc := &Service{
		Name: name, Type: "envoy", Host: req.Host, Port: req.Port, Username: req.Username,
		DeployDir: workingDir, Manifest: manifest, Status: StatusConnected, EncryptedPassword: encrypted,
		Origin: OriginConnected, Container: ct.Name, ComposeProject: project, ComposeFiles: strings.Join(files, ","),
	}
	started := time.Now()
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	svc.HasSavedPassword = true

	var log strings.Builder
	fmt.Fprintf(&log, "Connected to Envoy container %s (%s) on %s\n", ct.Name, ct.Image, req.Host)
	fmt.Fprintf(&log, "Bootstrap config: %s\n", envoyctl.ConfigPath(ct.Args))
	if project != "" {
		fmt.Fprintf(&log, "Compose project: %s in %s\n", project, workingDir)
	}
	_, detail := envoyctl.CheckHealth(client, ct)
	fmt.Fprintf(&log, "Health: %s\n", detail)
	fmt.Fprintln(&log, "The panel never deploys or deletes this stack. Start, Stop and Down unlock once the whole stack has been healthy for 5 minutes in a row.")
	h.recordAction(r.Context(), svc, "connect", "success", log.String(), started)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}
