package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/randid"
	"devops-wrapper/internal/sshexec"
)

// Rule management for Prometheus services: inspect what the running
// Prometheus loaded, preview a change as a before/after of each file,
// apply it (validated, backed up, hot-reloaded), and move an inline config
// onto the server's disk so edits survive redeploys.

func (h *Handler) registerPrometheusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/services/{id}/prometheus/inspect", h.handlePromInspect)
	mux.HandleFunc("POST /api/services/{id}/prometheus/preview", h.handlePromPreview)
	mux.HandleFunc("POST /api/services/{id}/prometheus/apply", h.handlePromApply)
	mux.HandleFunc("POST /api/services/{id}/prometheus/move-to-disk", h.handlePromMoveToDisk)
	mux.HandleFunc("GET /api/services/{id}/prometheus/api/{rest...}", h.handlePromAPI)
	mux.HandleFunc("POST /api/services/{id}/prometheus/status", h.handlePromStatus)
	mux.HandleFunc("GET /api/services/{id}/prometheus/snapshot", h.handlePromSnapshot)
}

// handlePromStatus reports the running server's internals: build, runtime,
// TSDB, WAL, storage, its own health metrics, and what needs attention.
func (h *Handler) handlePromStatus(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	v, ok := h.promRead(w, r, req.Password, snapshotStatus, func(_ *Service, c *sshexec.Client, inst *promctl.Instance) (any, error) {
		return promctl.Status(c, inst)
	})
	if ok {
		writeJSON(w, v, nil)
	}
}

// hasPrometheus reports whether a service runs Prometheus: the generated
// stack, or a pasted compose file that uses the Prometheus image.
func hasPrometheus(svc *Service) bool {
	return svc.Type == "prometheus" || (svc.Type == "docker" && strings.Contains(svc.Manifest, "prom/prometheus"))
}

// discoverPrometheus finds the service's Prometheus container: the pinned
// one for a connected stack, otherwise the one in its compose project.
func discoverPrometheus(client *sshexec.Client, svc *Service) (*promctl.Instance, error) {
	if svc.Container == "" {
		return promctl.Discover(client, svc.DeployDir)
	}
	inst, err := promctl.DiscoverContainer(client, svc.Container)
	if err != nil {
		return nil, err
	}
	if inst.State != "running" {
		return nil, fmt.Errorf("the Prometheus container %s is %s; start it on the server first", inst.Container, inst.State)
	}
	return inst, nil
}

// promSession resolves the service and credentials, connects, and finds
// the Prometheus container. Callers must Close the returned client.
func (h *Handler) promSession(w http.ResponseWriter, r *http.Request, password string) (*Service, *sshexec.Client, *promctl.Instance, bool) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return nil, nil, nil, false
	}
	if !hasPrometheus(svc) {
		http.Error(w, "this service does not run Prometheus", http.StatusBadRequest)
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
	inst, err := discoverPrometheus(client, svc)
	if err != nil {
		client.Close()
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, false
	}
	return svc, client, inst, true
}

type passwordRequest struct {
	Password string `json:"password"`
}

type inspectResponse struct {
	*promctl.Report
	// CanMoveToDisk is true for panel-generated stacks whose config is
	// still inline, which the panel knows how to convert.
	CanMoveToDisk bool `json:"canMoveToDisk"`
}

func (h *Handler) handlePromInspect(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	v, ok := h.promRead(w, r, req.Password, snapshotInspect, func(svc *Service, c *sshexec.Client, inst *promctl.Instance) (any, error) {
		report, err := promctl.Inspect(c, inst)
		if err != nil {
			return nil, err
		}
		return inspectResponse{Report: report, CanMoveToDisk: svc.Type == "prometheus" && !svc.Connected() && !report.OnDisk}, nil
	})
	if ok {
		writeJSON(w, v, nil)
	}
}

// promRead reads from the service's Prometheus over a kept connection
// (skipping a fresh SSH handshake and container lookup each time),
// retrying once on a new connection if the kept one fails, and saves a
// successful reading as the kind's snapshot.
func (h *Handler) promRead(w http.ResponseWriter, r *http.Request, password, kind string,
	read func(*Service, *sshexec.Client, *promctl.Instance) (any, error)) (any, bool) {
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return nil, false
	}
	if !hasPrometheus(svc) {
		http.Error(w, "this service does not run Prometheus", http.StatusBadRequest)
		return nil, false
	}
	pw, err := h.resolvePassword(r.Context(), svc, password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := h.promPool.get(svc, pw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return nil, false
		}
		v, err := read(svc, conn.client, conn.inst)
		if err == nil {
			h.snapshots.Save(r.Context(), svc.ID, kind, v)
			return v, true
		}
		lastErr = err
		h.promPool.drop(svc, pw)
	}
	http.Error(w, lastErr.Error(), http.StatusBadGateway)
	return nil, false
}

type fileRef struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA     string `json:"sha"`
}

type previewRequest struct {
	Kind   string  `json:"kind"` // "targets", "job" or "rule"
	Config fileRef `json:"config"`

	// kind "targets"
	Job     string               `json:"job"`
	Targets []promctl.TargetSpec `json:"targets"`

	// kind "job"
	NewJob promctl.JobSpec `json:"newJob"`

	// kind "rule"; RuleFile is nil to let the panel pick or create one.
	RuleFile *fileRef         `json:"ruleFile"`
	Group    string           `json:"group"`
	Rule     promctl.RuleSpec `json:"rule"`
}

type previewChange struct {
	Path    string `json:"path"`
	Before  string `json:"before"`
	After   string `json:"after"`
	BaseSHA string `json:"baseSha"`
	IsNew   bool   `json:"isNew"`
}

type previewResponse struct {
	Changes []previewChange `json:"changes"`
	Skipped []string        `json:"skipped,omitempty"`
	Summary string          `json:"summary"`
}

// panelRuleFile is the file the panel adds rules to when the user doesn't
// pick an existing one.
const panelRuleFile = "panel-rules.yml"

// handlePromPreview computes a change without touching the server: the
// client sends the current file contents it got from inspect, and gets
// back the new contents to review. Apply re-checks that the server still
// has those same contents before writing.
func (h *Handler) handlePromPreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if !path.IsAbs(req.Config.Path) {
		http.Error(w, "missing config file; inspect first", http.StatusBadRequest)
		return
	}
	resp, err := buildPreview(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, resp, nil)
}

func buildPreview(req previewRequest) (*previewResponse, error) {
	configChange := func(after string) previewChange {
		return previewChange{Path: req.Config.Path, Before: req.Config.Content, After: after, BaseSHA: req.Config.SHA}
	}

	switch req.Kind {
	case "targets":
		after, skipped, err := promctl.AddTargets(req.Config.Content, req.Job, req.Targets)
		if err != nil {
			return nil, err
		}
		added := -len(skipped)
		for _, t := range req.Targets {
			if strings.TrimSpace(t.Address) != "" {
				added++
			}
		}
		return &previewResponse{
			Changes: []previewChange{configChange(after)},
			Skipped: skipped,
			Summary: fmt.Sprintf("Add %d target(s) to job %s", added, req.Job),
		}, nil

	case "job":
		after, err := promctl.AddJob(req.Config.Content, req.NewJob)
		if err != nil {
			return nil, err
		}
		return &previewResponse{
			Changes: []previewChange{configChange(after)},
			Summary: fmt.Sprintf("Add scrape job %s with %d target(s)", req.NewJob.Name, len(req.NewJob.Targets)),
		}, nil

	case "rule":
		summary := fmt.Sprintf("Add alert %s to group %s", strings.TrimSpace(req.Rule.Alert), strings.TrimSpace(req.Group))
		if req.RuleFile != nil {
			after, err := promctl.AddRule(req.RuleFile.Content, req.Group, req.Rule)
			if err != nil {
				return nil, err
			}
			return &previewResponse{
				Changes: []previewChange{{Path: req.RuleFile.Path, Before: req.RuleFile.Content, After: after, BaseSHA: req.RuleFile.SHA}},
				Summary: summary,
			}, nil
		}

		after, err := promctl.AddRule("", req.Group, req.Rule)
		if err != nil {
			return nil, err
		}
		// Use a location the config already loads, if there is one;
		// otherwise put the file next to the config and list it.
		dir := path.Dir(req.Config.Path)
		patterns := promctl.RulePatterns(req.Config.Content, dir)
		candidate := dir + "/rules/" + panelRuleFile
		for _, p := range patterns {
			if ok, _ := path.Match(p, candidate); ok {
				return &previewResponse{
					Changes: []previewChange{{Path: candidate, After: after, IsNew: true}},
					Summary: summary + " (new file " + candidate + ")",
				}, nil
			}
		}
		config, _, err := promctl.EnsureRuleFile(req.Config.Content, panelRuleFile)
		if err != nil {
			return nil, err
		}
		return &previewResponse{
			Changes: []previewChange{
				{Path: dir + "/" + panelRuleFile, After: after, IsNew: true},
				configChange(config),
			},
			Summary: summary + " (new file " + panelRuleFile + ", listed in rule_files)",
		}, nil
	}
	return nil, fmt.Errorf("unknown change kind %q", req.Kind)
}

type applyRequest struct {
	Password string           `json:"password"`
	Summary  string           `json:"summary"`
	Changes  []promctl.Change `json:"changes"`
}

type applyResponse struct {
	OK    bool   `json:"ok"`
	Log   string `json:"log"`
	Error string `json:"error,omitempty"`
}

func (h *Handler) handlePromApply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, inst, ok := h.promSession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()

	started := time.Now()
	var out strings.Builder
	if req.Summary != "" {
		fmt.Fprintf(&out, "%s\n", req.Summary)
	}
	fmt.Fprintf(&out, "Prometheus container: %s\n", inst.Container)
	err := promctl.Apply(client, inst, req.Changes, &out)

	resp := applyResponse{OK: err == nil}
	status := "success"
	if err != nil {
		resp.Error = err.Error()
		fmt.Fprintf(&out, "Failed: %v\n", err)
		status = "failed"
	}
	resp.Log = out.String()
	h.recordAction(r.Context(), svc, "rules", status, resp.Log, started)
	h.promPool.dropService(svc.ID)
	writeJSON(w, resp, nil)
}

// handlePromMoveToDisk converts a panel-generated stack whose Prometheus
// config is inline in docker-compose.yml: it writes the config Prometheus
// is running with into deployDir/prometheus-config on the server, rewrites
// the compose file to mount that folder, and redeploys, which recreates
// only the Prometheus container.
func (h *Handler) handlePromMoveToDisk(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	svc, client, inst, ok := h.promSession(w, r, req.Password)
	if !ok {
		return
	}
	defer client.Close()
	if svc.Type != "prometheus" || svc.Connected() {
		http.Error(w, "only panel-created Prometheus stacks can be moved automatically", http.StatusBadRequest)
		return
	}
	if _, onDisk := inst.HostPath(inst.ConfigPath); onDisk {
		http.Error(w, "the config is already on the server's disk", http.StatusBadRequest)
		return
	}

	report, err := promctl.Inspect(client, inst)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	manifest, err := promctl.MoveConfigToDisk(svc.Manifest, svc.DeployDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Load a rules folder from now on, so rules can be added as files.
	config, _, err := promctl.EnsureRuleFile(report.Config.Content, "rules/*.yml")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	folder := strings.TrimRight(svc.DeployDir, "/") + "/" + promctl.ConfigFolder
	file := folder + "/prometheus.yml"
	backup := promctl.BackupPath(file, time.Now().UTC().Format("20060102-150405"))
	cmd := `if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; else AS_ROOT="sudo -n"; fi; set -e; ` +
		fmt.Sprintf(`$AS_ROOT mkdir -p %s %s; `, sshexec.ShellQuote(folder), sshexec.ShellQuote(folder+"/rules")) +
		fmt.Sprintf(`[ ! -f %s ] || { $AS_ROOT mkdir -p "$(dirname %s)" && $AS_ROOT cp -p %s %s; }; `,
			sshexec.ShellQuote(file), sshexec.ShellQuote(backup), sshexec.ShellQuote(file), sshexec.ShellQuote(backup)) +
		fmt.Sprintf(`echo %s | base64 -d | $AS_ROOT tee %s > /dev/null`, base64.StdEncoding.EncodeToString([]byte(config)), sshexec.ShellQuote(file))
	if _, stderr, err := client.Output(cmd); err != nil {
		http.Error(w, fmt.Sprintf("writing %s failed: %v %s", file, err, strings.TrimSpace(stderr)), http.StatusBadGateway)
		return
	}

	if _, err := h.services.Update(r.Context(), svc.ID, func(s *Service) { s.Manifest = manifest }); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	svc.Manifest = manifest

	pw, err := h.resolvePassword(r.Context(), svc, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: pw}
	// No seed files here: the config written above is the one Prometheus
	// was already running, and no new rules should appear unasked.
	job := h.jobs.RunCompose(target, manifest, svc.DeployDir, deploy.DataDirsFor(svc.Type, svc.DeployDir), nil)
	jobID := job.Snapshot().ID
	h.startLog(r.Context(), svc, "move-config", job)
	if _, err := h.services.Update(r.Context(), svc.ID, func(s *Service) {
		s.Status = StatusDeploying
		s.LastJobID = jobID
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.promPool.dropService(svc.ID)
	go h.watchJob(svc.ID, job, StatusDeployed)
	writeJSON(w, map[string]string{"jobId": jobID}, nil)
}

// recordAction saves a finished, synchronous action to the service's
// history. Failures to save are logged, not surfaced.
func (h *Handler) recordAction(ctx context.Context, svc *Service, action, status, text string, started time.Time) {
	id, err := randid.New(16)
	if err != nil {
		log.Printf("service logs: id: %v", err)
		return
	}
	finished := time.Now()
	if err := h.logs.Record(ctx, LogEntry{
		ID: id, ServiceID: svc.ID, ServiceName: svc.Name, Host: svc.Host,
		Action: action, Status: status, Log: text, StartedAt: started, FinishedAt: &finished,
	}); err != nil {
		log.Printf("service logs: record %s for %s: %v", action, svc.ID, err)
	}
}
