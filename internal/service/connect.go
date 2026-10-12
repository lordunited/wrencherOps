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
	"devops-wrapper/internal/envoyctl"
	"devops-wrapper/internal/gitlabctl"
	"devops-wrapper/internal/haproxyctl"
	"devops-wrapper/internal/nginxctl"
	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// Connecting an existing Prometheus: the panel finds the Prometheus
// containers on a server however they were set up (any compose file name,
// any folder, config mounted from anywhere, custom port or URL prefix),
// and attaches to the one the user picks. A connected stack is inspected
// and configured, never deployed, stopped or removed.

type connectTarget struct {
	Kind     string `json:"kind"` // "prometheus" (default), "nginx", "haproxy", "envoy" or "gitlab"
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type discoverResponse struct {
	Candidates any `json:"candidates"`
	// AlreadyConnected maps container name to the panel service already
	// connected to it on this host.
	AlreadyConnected map[string]string `json:"alreadyConnected"`
}

func (h *Handler) handleConnectDiscover(w http.ResponseWriter, r *http.Request) {
	var req connectTarget
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()

	var candidates any
	switch req.Kind {
	case "nginx":
		candidates, err = nginxctl.FindAll(client)
	case "haproxy":
		candidates, err = haproxyctl.FindAll(client)
	case "envoy":
		candidates, err = envoyctl.FindAll(client)
	case "gitlab":
		candidates, err = gitlabctl.FindAll(client)
	default:
		candidates, err = promctl.FindAll(client)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	resp := discoverResponse{Candidates: candidates, AlreadyConnected: h.connectedOn(r, req.Host)}
	writeJSON(w, resp, nil)
}

// connectedOn maps container name to service ID for services already
// connected on host.
func (h *Handler) connectedOn(r *http.Request, host string) map[string]string {
	out := map[string]string{}
	all, err := h.services.List(r.Context())
	if err != nil {
		return out
	}
	for _, s := range all {
		if s.Host == host && s.Container != "" {
			out[s.Container] = s.ID
		}
	}
	return out
}

type connectRequest struct {
	connectTarget
	Name      string `json:"name"`
	Container string `json:"container"`
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	var req connectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Container) == "" {
		http.Error(w, "pick a container to connect", http.StatusBadRequest)
		return
	}
	client, err := h.dialTarget(r.Context(), &req.connectTarget)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer client.Close()

	if id, ok := h.connectedOn(r, req.Host)[req.Container]; ok {
		http.Error(w, fmt.Sprintf("this %s is already connected (service %s)", req.Kind, id), http.StatusConflict)
		return
	}
	if req.Kind == "nginx" {
		h.connectNginx(w, r, req, client)
		return
	}
	if req.Kind == "haproxy" {
		h.connectHAProxy(w, r, req, client)
		return
	}
	if req.Kind == "envoy" {
		h.connectEnvoy(w, r, req, client)
		return
	}
	if req.Kind == "gitlab" {
		h.connectGitLab(w, r, req, client)
		return
	}
	inst, err := promctl.DiscoverContainer(client, req.Container)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	project := inst.Labels["com.docker.compose.project"]
	workingDir := inst.Labels["com.docker.compose.project.working_dir"]
	manifest := "# This Prometheus was not started with Docker Compose.\n"
	if files := inst.Labels["com.docker.compose.project.config_files"]; files != "" {
		if content := promctl.ReadHostFiles(client, strings.Split(files, ",")); content != "" {
			manifest = content
		} else {
			manifest = "# The compose file(s) could not be read: " + files + "\n"
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
		name = inst.Container
	}
	svc := &Service{
		Name:              name,
		Type:              "prometheus",
		Host:              req.Host,
		Port:              req.Port,
		Username:          req.Username,
		DeployDir:         workingDir,
		Manifest:          manifest,
		Status:            StatusConnected,
		EncryptedPassword: encrypted,
		Origin:            OriginConnected,
		Container:         inst.Container,
		ComposeProject:    project,
		ComposeFiles:      inst.Labels["com.docker.compose.project.config_files"],
	}
	started := time.Now()
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	svc.HasSavedPassword = true

	var log strings.Builder
	fmt.Fprintf(&log, "Connected to Prometheus container %s (%s) on %s\n", inst.Container, inst.Image, req.Host)
	if project != "" {
		fmt.Fprintf(&log, "Compose project: %s in %s\n", project, workingDir)
	}
	fmt.Fprintf(&log, "Config file: %s\n", inst.ConfigPath)
	if host, ok := inst.HostPath(inst.ConfigPath); ok {
		fmt.Fprintf(&log, "  on the server at %s\n", host)
	} else {
		fmt.Fprintln(&log, "  inside the container only; edits would be lost when the container is recreated")
	}
	fmt.Fprintf(&log, "API: port %s%s\n", inst.Port, inst.RoutePrefix)
	for _, m := range inst.Mounts {
		fmt.Fprintf(&log, "Mount: %s -> %s\n", m.Source, m.Destination)
	}
	fmt.Fprintln(&log, "The panel never deploys or deletes this stack. Start, Stop and Down unlock once the whole stack has been healthy for 5 minutes in a row.")
	h.recordAction(r.Context(), svc, "connect", "success", log.String(), started)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// connectNginx attaches to an existing Nginx container.
func (h *Handler) connectNginx(w http.ResponseWriter, r *http.Request, req connectRequest, client *sshexec.Client) {
	ct, err := dockerx.Inspect(client, req.Container)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !nginxctl.IsNginxImage(ct.Image) {
		http.Error(w, ct.Name+" doesn't run an Nginx image", http.StatusBadRequest)
		return
	}
	project, workingDir, files := ct.Compose()
	manifest := "# This Nginx was not started with Docker Compose.\n"
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
		Name: name, Type: "nginx", Host: req.Host, Port: req.Port, Username: req.Username,
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
	fmt.Fprintf(&log, "Connected to Nginx container %s (%s) on %s\n", ct.Name, ct.Image, req.Host)
	if project != "" {
		fmt.Fprintf(&log, "Compose project: %s in %s\n", project, workingDir)
	}
	for _, p := range []string{"/etc/nginx/nginx.conf", "/etc/nginx/conf.d"} {
		if host, ok := ct.HostPath(p); ok {
			fmt.Fprintf(&log, "%s is on the server at %s\n", p, host)
		} else {
			fmt.Fprintf(&log, "%s is inside the container only; edits there are lost when the container is recreated\n", p)
		}
	}
	fmt.Fprintln(&log, "The panel never deploys or deletes this stack. Start, Stop and Down unlock once the whole stack has been healthy for 5 minutes in a row.")
	h.recordAction(r.Context(), svc, "connect", "success", log.String(), started)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// connectHAProxy attaches to an existing HAProxy container. Unlike the
// panel's own generated stacks, its config is arbitrary — there's no fixed
// /healthz to rely on — so this reads it right away and logs the health
// probe determined from it (a monitor-uri, else its stats page, else just
// a bound port), the same one CheckHealth uses on every future Sync.
func (h *Handler) connectHAProxy(w http.ResponseWriter, r *http.Request, req connectRequest, client *sshexec.Client) {
	ct, err := dockerx.Inspect(client, req.Container)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !haproxyctl.IsHAProxyImage(ct.Image) {
		http.Error(w, ct.Name+" doesn't run an HAProxy image", http.StatusBadRequest)
		return
	}
	project, workingDir, files := ct.Compose()
	manifest := "# This HAProxy was not started with Docker Compose.\n"
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
		Name: name, Type: "haproxy", Host: req.Host, Port: req.Port, Username: req.Username,
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
	fmt.Fprintf(&log, "Connected to HAProxy container %s (%s) on %s\n", ct.Name, ct.Image, req.Host)
	if project != "" {
		fmt.Fprintf(&log, "Compose project: %s in %s\n", project, workingDir)
	}
	_, detail := haproxyctl.CheckHealth(client, ct)
	fmt.Fprintf(&log, "Health probe: %s\n", detail)
	fmt.Fprintln(&log, "The panel never deploys or deletes this stack. Start, Stop and Down unlock once the whole stack has been healthy for 5 minutes in a row.")
	h.recordAction(r.Context(), svc, "connect", "success", log.String(), started)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// connectGitLab attaches to an existing GitLab Omnibus container. Its web
// server may listen on any port, over HTTP or HTTPS, depending on its
// external_url, so health comes from "gitlab-ctl status" inside the
// container rather than an HTTP probe that would have to guess.
func (h *Handler) connectGitLab(w http.ResponseWriter, r *http.Request, req connectRequest, client *sshexec.Client) {
	ct, err := dockerx.Inspect(client, req.Container)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !gitlabctl.IsGitLabImage(ct.Image) {
		http.Error(w, ct.Name+" doesn't run a GitLab image", http.StatusBadRequest)
		return
	}
	project, workingDir, files := ct.Compose()
	manifest := "# This GitLab was not started with Docker Compose.\n"
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
		Name: name, Type: "gitlab", Host: req.Host, Port: req.Port, Username: req.Username,
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
	fmt.Fprintf(&log, "Connected to GitLab container %s (%s) on %s\n", ct.Name, ct.Image, req.Host)
	if project != "" {
		fmt.Fprintf(&log, "Compose project: %s in %s\n", project, workingDir)
	}
	for _, m := range []struct{ label, dir string }{{"Config", "/etc/gitlab"}, {"Data", "/var/opt/gitlab"}} {
		if host, ok := ct.HostPath(m.dir); ok {
			fmt.Fprintf(&log, "%s: %s on the server\n", m.label, host)
		} else {
			fmt.Fprintf(&log, "%s: %s is not mounted from the server (a Docker volume, or inside the container only)\n", m.label, m.dir)
		}
	}
	_, detail := gitlabctl.CheckHealth(client, ct.Name)
	fmt.Fprintf(&log, "Health: %s\n", detail)
	fmt.Fprintln(&log, "The panel never deploys or deletes this stack. Start, Stop and Down unlock once the whole stack has been healthy for 5 minutes in a row.")
	h.recordAction(r.Context(), svc, "connect", "success", log.String(), started)

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}
