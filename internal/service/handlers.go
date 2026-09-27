package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/randid"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

type Handler struct {
	services  *Store
	logs      *LogStore
	jobs      *deploy.Store
	secrets   *secretbox.Box
	snapshots *SnapshotStore
	promPool  promConnPool
}

func NewHandler(services *Store, logs *LogStore, snapshots *SnapshotStore, jobs *deploy.Store, secrets *secretbox.Box) *Handler {
	return &Handler{services: services, logs: logs, snapshots: snapshots, jobs: jobs, secrets: secrets}
}

// resolvePassword returns the password to use for this action: the one
// just supplied, if any (also persisting it for next time), otherwise the
// saved one decrypted, if there is one.
func (h *Handler) resolvePassword(ctx context.Context, svc *Service, supplied string) (string, error) {
	if supplied != "" {
		encrypted, err := h.secrets.Encrypt(supplied)
		if err != nil {
			return "", err
		}
		if _, err := h.services.Update(ctx, svc.ID, func(s *Service) {
			s.EncryptedPassword = encrypted
		}); err != nil {
			return "", err
		}
		return supplied, nil
	}
	if svc.EncryptedPassword == "" {
		return "", fmt.Errorf("no password supplied and none saved for this service")
	}
	return h.secrets.Decrypt(svc.EncryptedPassword)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", h.handleList)
	mux.HandleFunc("POST /api/services/gitlab", h.handleCreateGitLab)
	mux.HandleFunc("POST /api/services/nexus3", h.handleCreateNexus3)
	mux.HandleFunc("POST /api/services/prometheus", h.handleCreatePrometheus)
	mux.HandleFunc("POST /api/services/nginx", h.handleCreateNginx)
	mux.HandleFunc("POST /api/services/docker", h.handleCreateDocker)
	mux.HandleFunc("POST /api/services/kubernetes", h.handleCreateKubernetes)
	mux.HandleFunc("POST /api/services/connect/discover", h.handleConnectDiscover)
	mux.HandleFunc("POST /api/services/connect", h.handleConnect)
	mux.HandleFunc("GET /api/services/{id}", h.handleGet)
	mux.HandleFunc("PUT /api/services/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/services/{id}", h.handleDelete)
	mux.HandleFunc("POST /api/services/{id}/deploy", h.handleDeploy)
	mux.HandleFunc("POST /api/services/{id}/start", h.handleStart)
	mux.HandleFunc("POST /api/services/{id}/stop", h.handleStop)
	mux.HandleFunc("POST /api/services/{id}/down", h.handleDown)
	mux.HandleFunc("POST /api/services/{id}/destroy", h.handleDestroy)
	mux.HandleFunc("POST /api/services/{id}/sync", h.handleSync)
	mux.HandleFunc("GET /api/services/{id}/logs", h.handleListLogs)
	mux.HandleFunc("GET /api/services/{id}/logs/{logId}", h.handleGetLog)
	h.registerPrometheusRoutes(mux)
	h.registerNginxRoutes(mux)
}

// maxListedLogs is how many history entries a service's log list returns.
const maxListedLogs = 100

func (h *Handler) handleListLogs(w http.ResponseWriter, r *http.Request) {
	entries, err := h.logs.List(r.Context(), r.PathValue("id"), maxListedLogs)
	writeJSON(w, entries, err)
}

func (h *Handler) handleGetLog(w http.ResponseWriter, r *http.Request) {
	entry, ok, err := h.logs.Get(r.Context(), r.PathValue("id"), r.PathValue("logId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "log not found", http.StatusNotFound)
		return
	}
	writeJSON(w, entry, nil)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	services, err := h.services.List(r.Context())
	writeJSON(w, services, err)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	writeJSON(w, svc, nil)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := h.services.Get(r.Context(), id); !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if err := h.services.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.snapshots.DeleteService(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

type createGitLabRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`

	ExternalURL        string   `json:"externalUrl"`
	HTTPPort           int      `json:"httpPort"`
	HTTPSPort          int      `json:"httpsPort"`
	SSHPort            int      `json:"sshPort"`
	RootPassword       string   `json:"rootPassword"`
	ImageTag           string   `json:"imageTag"`
	ContainerName      string   `json:"containerName"`
	ExtraOmnibusConfig string   `json:"extraOmnibusConfig"`
	Networks           []string `json:"networks"`
	ExtraVolumes       []string `json:"extraVolumes"`
	CPULimit           string   `json:"cpuLimit"`
	MemoryLimit        string   `json:"memoryLimit"`
}

func (h *Handler) handleCreateGitLab(w http.ResponseWriter, r *http.Request) {
	var req createGitLabRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/gitlab"
	}
	if req.HTTPPort == 0 {
		req.HTTPPort = 80
	}
	if req.SSHPort == 0 {
		req.SSHPort = 2222
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	if req.ContainerName == "" {
		req.ContainerName = "gitlab"
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
	if req.ExternalURL == "" {
		missing = append(missing, "externalUrl")
	}
	if len(req.RootPassword) < 8 {
		missing = append(missing, "rootPassword (minimum 8 characters)")
	}
	if len(missing) > 0 {
		http.Error(w, "missing or invalid fields: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}

	manifest := deploy.GitLabConfig{
		ExternalURL:        req.ExternalURL,
		HTTPPort:           req.HTTPPort,
		HTTPSPort:          req.HTTPSPort,
		SSHPort:            req.SSHPort,
		RootPassword:       req.RootPassword,
		ImageTag:           req.ImageTag,
		DeployDir:          req.DeployDir,
		ContainerName:      req.ContainerName,
		ExtraOmnibusConfig: req.ExtraOmnibusConfig,
		Networks:           req.Networks,
		ExtraVolumes:       req.ExtraVolumes,
		CPULimit:           req.CPULimit,
		MemoryLimit:        req.MemoryLimit,
	}.ComposeYAML()

	svc := &Service{
		Name:      req.Name,
		Type:      "gitlab",
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		DeployDir: req.DeployDir,
		Manifest:  manifest,
		Status:    StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

type createNexus3Request struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`

	HTTPPort      int      `json:"httpPort"`
	ExtraPort     int      `json:"extraPort"`
	ImageTag      string   `json:"imageTag"`
	ContainerName string   `json:"containerName"`
	Networks      []string `json:"networks"`
	ExtraVolumes  []string `json:"extraVolumes"`
	CPULimit      string   `json:"cpuLimit"`
	MemoryLimit   string   `json:"memoryLimit"`
	JavaHeap      string   `json:"javaHeap"`
}

func (h *Handler) handleCreateNexus3(w http.ResponseWriter, r *http.Request) {
	var req createNexus3Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/nexus3"
	}
	if req.HTTPPort == 0 {
		req.HTTPPort = 8081
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	if req.ContainerName == "" {
		req.ContainerName = "nexus3"
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

	manifest := deploy.Nexus3Config{
		HTTPPort:      req.HTTPPort,
		ExtraPort:     req.ExtraPort,
		ImageTag:      req.ImageTag,
		DeployDir:     req.DeployDir,
		ContainerName: req.ContainerName,
		Networks:      req.Networks,
		ExtraVolumes:  req.ExtraVolumes,
		CPULimit:      req.CPULimit,
		MemoryLimit:   req.MemoryLimit,
		JavaHeap:      req.JavaHeap,
	}.ComposeYAML()

	svc := &Service{
		Name:      req.Name,
		Type:      "nexus3",
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		DeployDir: req.DeployDir,
		Manifest:  manifest,
		Status:    StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

type createPrometheusRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`

	PrometheusPort        int      `json:"prometheusPort"`
	GrafanaPort           int      `json:"grafanaPort"`
	AlertmanagerPort      int      `json:"alertmanagerPort"`
	NodeExporterPort      int      `json:"nodeExporterPort"`
	ImageTag              string   `json:"imageTag"`
	GrafanaAdminPassword  string   `json:"grafanaAdminPassword"`
	RetentionTime         string   `json:"retentionTime"`
	ContainerPrefix       string   `json:"containerPrefix"`
	Networks              []string `json:"networks"`
	ExtraVolumes          []string `json:"extraVolumes"`
	PrometheusCPULimit    string   `json:"prometheusCpuLimit"`
	PrometheusMemoryLimit string   `json:"prometheusMemoryLimit"`
}

func (h *Handler) handleCreatePrometheus(w http.ResponseWriter, r *http.Request) {
	var req createPrometheusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/prometheus-stack"
	}
	if req.PrometheusPort == 0 {
		req.PrometheusPort = 9090
	}
	if req.GrafanaPort == 0 {
		req.GrafanaPort = 3000
	}
	if req.AlertmanagerPort == 0 {
		req.AlertmanagerPort = 9093
	}
	if req.NodeExporterPort == 0 {
		req.NodeExporterPort = 9100
	}
	if req.ImageTag == "" {
		req.ImageTag = "latest"
	}
	if req.RetentionTime == "" {
		req.RetentionTime = "15d"
	}
	if req.ContainerPrefix == "" {
		req.ContainerPrefix = "monitoring"
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
	if len(req.GrafanaAdminPassword) < 8 {
		missing = append(missing, "grafanaAdminPassword (minimum 8 characters)")
	}
	if len(missing) > 0 {
		http.Error(w, "missing or invalid fields: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}

	manifest := deploy.PrometheusStackConfig{
		PrometheusPort:        req.PrometheusPort,
		GrafanaPort:           req.GrafanaPort,
		AlertmanagerPort:      req.AlertmanagerPort,
		NodeExporterPort:      req.NodeExporterPort,
		ImageTag:              req.ImageTag,
		GrafanaAdminPassword:  req.GrafanaAdminPassword,
		RetentionTime:         req.RetentionTime,
		DeployDir:             req.DeployDir,
		ContainerPrefix:       req.ContainerPrefix,
		Networks:              req.Networks,
		ExtraVolumes:          req.ExtraVolumes,
		PrometheusCPULimit:    req.PrometheusCPULimit,
		PrometheusMemoryLimit: req.PrometheusMemoryLimit,
	}.ComposeYAML()

	svc := &Service{
		Name:      req.Name,
		Type:      "prometheus",
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		DeployDir: req.DeployDir,
		Manifest:  manifest,
		Status:    StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

type createNginxRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`

	HTTPPort      int      `json:"httpPort"`
	HTTPSPort     int      `json:"httpsPort"`
	ImageTag      string   `json:"imageTag"`
	ContainerName string   `json:"containerName"`
	Networks      []string `json:"networks"`
	ExtraVolumes  []string `json:"extraVolumes"`
	CPULimit      string   `json:"cpuLimit"`
	MemoryLimit   string   `json:"memoryLimit"`
}

func (h *Handler) handleCreateNginx(w http.ResponseWriter, r *http.Request) {
	var req createNginxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/nginx"
	}
	if req.HTTPPort == 0 {
		req.HTTPPort = 80
	}
	if req.ImageTag == "" {
		req.ImageTag = "stable-alpine"
	}
	if req.ContainerName == "" {
		req.ContainerName = "nginx"
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

	manifest := deploy.NginxConfig{
		HTTPPort:      req.HTTPPort,
		HTTPSPort:     req.HTTPSPort,
		ImageTag:      req.ImageTag,
		DeployDir:     strings.TrimRight(req.DeployDir, "/"),
		ContainerName: req.ContainerName,
		Networks:      req.Networks,
		ExtraVolumes:  req.ExtraVolumes,
		CPULimit:      req.CPULimit,
		MemoryLimit:   req.MemoryLimit,
	}.ComposeYAML()

	svc := &Service{
		Name:      req.Name,
		Type:      "nginx",
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		DeployDir: req.DeployDir,
		Manifest:  manifest,
		Status:    StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

// createRawRequest is shared by the "bring your own YAML" service types:
// a raw docker-compose file or a raw Kubernetes manifest, applied as-is
// with no generation step.
type createRawRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	DeployDir string `json:"deployDir"`
	Manifest  string `json:"manifest"`
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := strings.Trim(slugNonAlnum.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if slug == "" {
		return "app"
	}
	return slug
}

func (h *Handler) handleCreateRaw(w http.ResponseWriter, r *http.Request, serviceType, defaultDirBase string) {
	var req createRawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.DeployDir == "" {
		req.DeployDir = "/opt/" + slugify(nonEmpty(req.Name, defaultDirBase))
	}

	var missing []string
	if req.Name == "" {
		missing = append(missing, "name")
	}
	if req.Host == "" {
		missing = append(missing, "host")
	}
	if req.Username == "" {
		missing = append(missing, "username")
	}
	if strings.TrimSpace(req.Manifest) == "" {
		missing = append(missing, "manifest")
	}
	if len(missing) > 0 {
		http.Error(w, "missing or invalid fields: "+strings.Join(missing, ", "), http.StatusBadRequest)
		return
	}

	svc := &Service{
		Name:      req.Name,
		Type:      serviceType,
		Host:      req.Host,
		Port:      req.Port,
		Username:  req.Username,
		DeployDir: req.DeployDir,
		Manifest:  req.Manifest,
		Status:    StatusPending,
	}
	if err := h.services.Create(r.Context(), svc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, svc, nil)
}

func nonEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func (h *Handler) handleCreateDocker(w http.ResponseWriter, r *http.Request) {
	h.handleCreateRaw(w, r, "docker", "app")
}

func (h *Handler) handleCreateKubernetes(w http.ResponseWriter, r *http.Request) {
	h.handleCreateRaw(w, r, "kubernetes", "app")
}

type updateRequest struct {
	Name      *string `json:"name"`
	Manifest  *string `json:"manifest"`
	Host      *string `json:"host"`
	Port      *int    `json:"port"`
	Username  *string `json:"username"`
	DeployDir *string `json:"deployDir"`
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if current, ok := h.services.Get(r.Context(), r.PathValue("id")); ok && current.Connected() &&
		(req.Manifest != nil && *req.Manifest != current.Manifest || req.DeployDir != nil && *req.DeployDir != current.DeployDir) {
		http.Error(w, "a connected stack's compose file and folder belong to the server; change them there", http.StatusBadRequest)
		return
	}
	svc, err := h.services.Update(r.Context(), r.PathValue("id"), func(s *Service) {
		if req.Name != nil {
			s.Name = *req.Name
		}
		if req.Manifest != nil {
			s.Manifest = *req.Manifest
		}
		if req.Host != nil {
			s.Host = *req.Host
		}
		if req.Port != nil {
			s.Port = *req.Port
		}
		if req.Username != nil {
			s.Username = *req.Username
		}
		if req.DeployDir != nil {
			s.DeployDir = *req.DeployDir
		}
	})
	if err != nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	writeJSON(w, svc, nil)
}

// connectedRefusal explains why deploy and lifecycle actions are refused
// for a stack the panel only connected to.
const connectedRefusal = "this is a connected stack: the panel never deploys or deletes it. Use the server's own tools for that."

// controlsLockedRefusal explains why Start/Stop/Down aren't available yet.
func controlsLockedRefusal(svc *Service) string {
	if svc.HealthySince == nil {
		return "Start, Stop and Down unlock after the stack has been healthy for 5 minutes in a row; it isn't healthy right now"
	}
	left := ControlsUnlockAfter - time.Since(*svc.HealthySince)
	if left < 0 {
		left = 0
	}
	return fmt.Sprintf("Start, Stop and Down unlock after 5 minutes of continuous health; about %s to go", left.Round(time.Second))
}

type deployRequest struct {
	Password string `json:"password"`
}

func (h *Handler) handleDeploy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	svc, ok := h.services.Get(r.Context(), id)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	if svc.Connected() {
		http.Error(w, connectedRefusal, http.StatusBadRequest)
		return
	}
	var req deployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	password, err := h.resolvePassword(r.Context(), svc, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password}

	var job *deploy.Job
	if svc.Type == "kubernetes" {
		job = h.jobs.RunKubernetesApply(target, svc.Manifest, svc.DeployDir)
	} else {
		// Starter config files only matter when the compose file mounts
		// them. Older stacks keep their config inline, and seeding there
		// would leave a starter alert rule waiting to switch on unasked
		// once the config is later moved to disk.
		var seeds []deploy.SeedFile
		if deploy.SeedsMounted(svc.Type, svc.Manifest) {
			seeds = deploy.SeedFilesFor(svc.Type, svc.DeployDir)
		}
		job = h.jobs.RunCompose(target, svc.Manifest, svc.DeployDir, deploy.DataDirsFor(svc.Type, svc.DeployDir), seeds)
	}
	jobID := job.Snapshot().ID
	h.startLog(r.Context(), svc, "deploy", job)

	if _, err := h.services.Update(r.Context(), id, func(s *Service) {
		s.Status = StatusDeploying
		s.LastJobID = jobID
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go h.watchJob(id, job, StatusDeployed)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"jobId": jobID})
}

// controlRequest is shared by the stop/down control actions, which need the
// same SSH credentials as deploy/sync but nothing else.
type controlRequest struct {
	Password string `json:"password"`
}

// startControl resolves credentials, kicks off runJob against target in the
// background, records transientStatus on the service immediately, and
// arranges for watchJob to record successStatus (or StatusFailed) once it
// finishes. Shared by handleStop and handleDown, which differ only in which
// deploy.Job constructor they call and which statuses they use.
func (h *Handler) startControl(w http.ResponseWriter, r *http.Request, action string, runJob func(target sshexec.Target, svc *Service) *deploy.Job, transientStatus, successStatus Status) {
	id := r.PathValue("id")
	svc, ok := h.services.Get(r.Context(), id)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if !svc.ControlsUnlocked() {
		http.Error(w, controlsLockedRefusal(svc), http.StatusBadRequest)
		return
	}
	if svc.Connected() && svc.ComposeProject == "" {
		http.Error(w, "this Prometheus wasn't started with Docker Compose, so the panel can't start or stop it", http.StatusBadRequest)
		return
	}
	if svc.Type == "kubernetes" {
		http.Error(w, "stop/down is only supported for docker-compose based services", http.StatusBadRequest)
		return
	}
	if svc.Connected() && successStatus == StatusDeployed {
		// A connected stack was never "deployed" by the panel.
		successStatus = StatusConnected
	}

	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	password, err := h.resolvePassword(r.Context(), svc, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password}
	job := runJob(target, svc)
	jobID := job.Snapshot().ID
	h.startLog(r.Context(), svc, action, job)

	if _, err := h.services.Update(r.Context(), id, func(s *Service) {
		s.Status = transientStatus
		s.LastJobID = jobID
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go h.watchJob(id, job, successStatus)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"jobId": jobID})
}

func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request) {
	// Start also prepares data folders, so a service whose containers failed
	// on folder permissions can be fixed with Start instead of a redeploy.
	h.startControl(w, r, "start", func(target sshexec.Target, svc *Service) *deploy.Job {
		// A connected stack's folders are its own; never re-own them.
		var dirs []deploy.DataDir
		if !svc.Connected() {
			dirs = deploy.DataDirsFor(svc.Type, svc.DeployDir)
		}
		return h.jobs.RunComposeStart(target, svc.Project(), dirs)
	}, StatusStarting, StatusDeployed)
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	h.startControl(w, r, "stop", func(target sshexec.Target, svc *Service) *deploy.Job {
		return h.jobs.RunComposeStop(target, svc.Project())
	}, StatusStopping, StatusStopped)
}

func (h *Handler) handleDown(w http.ResponseWriter, r *http.Request) {
	h.startControl(w, r, "down", func(target sshexec.Target, svc *Service) *deploy.Job {
		return h.jobs.RunComposeDown(target, svc.Project())
	}, StatusRemoving, StatusDown)
}

// handleDestroy tears down everything for a service on its target — "docker
// compose down -v" plus deleting deployDir itself, including any
// bind-mounted data — then deletes the service record. This is irreversible
// and the frontend must get explicit confirmation before calling it. Unlike
// startControl's actions, there's no "successStatus" to record: once the
// job succeeds there's nothing left to have a status, so the record is
// removed outright. On failure the record is kept (status "failed") so the
// user can retry or fall back to DELETE for a record-only removal.
func (h *Handler) handleDestroy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	svc, ok := h.services.Get(r.Context(), id)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if svc.Connected() {
		http.Error(w, connectedRefusal, http.StatusBadRequest)
		return
	}
	if svc.Type == "kubernetes" {
		http.Error(w, "destroy is only supported for docker-compose based services; use DELETE to remove the record directly", http.StatusBadRequest)
		return
	}

	var req controlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	password, err := h.resolvePassword(r.Context(), svc, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password}
	job := h.jobs.RunComposeDestroy(target, svc.DeployDir)
	jobID := job.Snapshot().ID
	h.startLog(r.Context(), svc, "destroy", job)

	if _, err := h.services.Update(r.Context(), id, func(s *Service) {
		s.Status = StatusRemoving
		s.LastJobID = jobID
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go h.watchDestroyJob(id, job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"jobId": jobID})
}

func (h *Handler) watchDestroyJob(serviceID string, job *deploy.Job) {
	ctx := context.Background()
	snapshot := h.trackJob(ctx, job)
	if snapshot.Status == deploy.StatusFailed {
		h.services.Update(ctx, serviceID, func(s *Service) { s.Status = StatusFailed })
		return
	}
	h.services.Delete(ctx, serviceID)
	h.snapshots.DeleteService(ctx, serviceID)
}

// watchJob polls job until it finishes and records the outcome on the
// service. The job itself already streams live output for the frontend to
// poll directly; this just closes the loop on the persisted status. It runs
// detached from the request that started it, so it uses its own context
// rather than the request's (which is canceled once that request returns).
func (h *Handler) watchJob(serviceID string, job *deploy.Job, successStatus Status) {
	ctx := context.Background()
	snapshot := h.trackJob(ctx, job)

	finalStatus := successStatus
	if snapshot.Status == deploy.StatusFailed {
		finalStatus = StatusFailed
	}
	h.services.Update(ctx, serviceID, func(s *Service) { s.Status = finalStatus })
}

// startLog records a just-started job in the service's saved history. A
// failure here is logged, not returned: losing the history entry shouldn't
// stop the action itself.
func (h *Handler) startLog(ctx context.Context, svc *Service, action string, job *deploy.Job) {
	snapshot := job.Snapshot()
	if err := h.logs.Start(ctx, snapshot.ID, svc, action, snapshot.StartedAt); err != nil {
		log.Printf("service logs: start %s for %s: %v", action, svc.ID, err)
	}
}

// trackJob waits for job to finish, saving its output to the service's
// history every couple of seconds along the way, so a backend restart
// mid-job loses at most the last few seconds. Returns the final snapshot.
func (h *Handler) trackJob(ctx context.Context, job *deploy.Job) deploy.Snapshot {
	var savedChunks int
	for {
		snapshot := job.Snapshot()
		running := snapshot.Status == deploy.StatusRunning

		if !running || len(snapshot.Log) != savedChunks {
			var finishedAt *time.Time
			if !running {
				finishedAt = &snapshot.UpdatedAt
			}
			if err := h.logs.Save(ctx, snapshot.ID, string(snapshot.Status), joinLog(snapshot.Log), finishedAt); err != nil {
				log.Printf("service logs: save %s: %v", snapshot.ID, err)
			}
			savedChunks = len(snapshot.Log)
		}

		if !running {
			return snapshot
		}
		time.Sleep(2 * time.Second)
	}
}

type syncRequest struct {
	Password string `json:"password"`
}

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	svc, ok := h.services.Get(r.Context(), id)
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	password, err := h.resolvePassword(r.Context(), svc, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password}
	startedAt := time.Now()
	health, detail := CheckHealth(target, svc)

	h.recordSync(r.Context(), svc, health, detail, startedAt, time.Now())
	updated, err := h.recordHealth(r.Context(), id, health, detail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, updated, nil)
}

// recordSync saves a sync's result to the service's history. Like
// startLog, a failure to save is logged rather than surfaced.
func (h *Handler) recordSync(ctx context.Context, svc *Service, health, detail string, startedAt, finishedAt time.Time) {
	id, err := randid.New(16)
	if err != nil {
		log.Printf("service logs: sync id: %v", err)
		return
	}
	entry := LogEntry{
		ID:          id,
		ServiceID:   svc.ID,
		ServiceName: svc.Name,
		Host:        svc.Host,
		Action:      "sync",
		Status:      health,
		Log:         fmt.Sprintf("Health check on %s@%s:%d\nResult: %s\n%s\n", svc.Username, svc.Host, svc.Port, health, detail),
		StartedAt:   startedAt,
		FinishedAt:  &finishedAt,
	}
	if err := h.logs.Record(ctx, entry); err != nil {
		log.Printf("service logs: record sync for %s: %v", svc.ID, err)
	}
}

func writeJSON(w http.ResponseWriter, data any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
