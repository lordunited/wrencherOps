// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"devops-wrapper/internal/ansible"
	"devops-wrapper/internal/secretbox"
)

type Handler struct {
	store   *Store
	runner  *Runner
	secrets *secretbox.Box
}

func NewHandler(store *Store, secrets *secretbox.Box) *Handler {
	return &Handler{store: store, runner: NewRunner(), secrets: secrets}
}

func (h *Handler) PathPrefixes() []string { return []string{"/api/terraform", "/api/terraform/"} }

func (h *Handler) RegisterRoutes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("GET /api/terraform/templates", h.handleTemplates)
	mux.HandleFunc("GET /api/terraform/standard", h.handleStandard)
	mux.HandleFunc("POST /api/terraform/fmt", h.handleFmt)
	mux.HandleFunc("POST /api/terraform/workspaces/{id}/analyze", h.handleAnalyze)
	mux.HandleFunc("GET /api/terraform/workspaces", h.handleList)
	mux.HandleFunc("POST /api/terraform/workspaces", h.handleCreate)
	mux.HandleFunc("POST /api/terraform/workspaces/upload", h.handleUpload)
	mux.HandleFunc("GET /api/terraform/workspaces/{id}", h.handleGet)
	mux.HandleFunc("PUT /api/terraform/workspaces/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/terraform/workspaces/{id}", h.handleDelete)
	mux.HandleFunc("GET /api/terraform/workspaces/{id}/variables", h.handleVariables)
	mux.HandleFunc("PUT /api/terraform/workspaces/{id}/variables", h.handleSaveVariables)
	mux.HandleFunc("GET /api/terraform/workspaces/{id}/state", h.handleState)
	mux.HandleFunc("POST /api/terraform/workspaces/{id}/runs", h.handlePlan)
	mux.HandleFunc("GET /api/terraform/runs", h.handleRuns)
	mux.HandleFunc("GET /api/terraform/runs/{id}", h.handleRun)
	mux.HandleFunc("POST /api/terraform/runs/{id}/apply", h.handleApply)
	mux.HandleFunc("POST /api/terraform/runs/{id}/cancel", h.handleCancel)
}

// Start marks runs a restart interrupted as failed.
func (h *Handler) Start(ctx context.Context) { h.store.MarkInterrupted(ctx) }

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
	}
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) handleTemplates(w http.ResponseWriter, r *http.Request) { writeJSON(w, Templates, 0) }

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Workspaces(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs, _ := h.store.Runs(r.Context(), "", 500)
	last := map[string]*Run{}
	for _, run := range runs {
		if _, ok := last[run.WorkspaceID]; !ok {
			last[run.WorkspaceID] = run
		}
	}
	for _, ws := range list {
		ws.LastRun, ws.Files = last[ws.ID], nil
	}
	writeJSON(w, list, 0)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	writeJSON(w, ws, 0)
}

type workspaceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Files       *[]File `json:"files"`
	Template    string  `json:"template"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	ws := &Workspace{}
	if req.Template != "" {
		for _, t := range Templates {
			if t.Key == req.Template {
				ws.Name, ws.Description, ws.Files = t.Name, t.Description, append([]File(nil), t.Files...)
			}
		}
		if ws.Files == nil {
			http.Error(w, "unknown template "+req.Template, http.StatusBadRequest)
			return
		}
	}
	if req.Name != nil {
		ws.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		ws.Description = strings.TrimSpace(*req.Description)
	}
	if req.Files != nil {
		ws.Files = *req.Files
	}
	if ws.Name == "" {
		http.Error(w, "the workspace needs a name", http.StatusBadRequest)
		return
	}
	if err := ValidateFiles(ws.Files); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.CreateWorkspace(r.Context(), ws); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, ws, http.StatusCreated)
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	var req workspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name != nil {
		if ws.Name = strings.TrimSpace(*req.Name); ws.Name == "" {
			http.Error(w, "the workspace needs a name", http.StatusBadRequest)
			return
		}
	}
	if req.Description != nil {
		ws.Description = strings.TrimSpace(*req.Description)
	}
	if req.Files != nil {
		if err := ValidateFiles(*req.Files); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ws.Files = *req.Files
	}
	if err := h.store.UpdateWorkspace(r.Context(), ws); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ws.OwnBackend = OwnBackend(ws.Files)
	writeJSON(w, ws, 0)
}

// handleDelete deletes a workspace. One whose state still has resources is
// only deleted with its name typed out: the real resources stay, and the
// panel loses track of them.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	if _, busy := h.runner.Busy(ws.ID); busy {
		http.Error(w, "a run is going in this workspace", http.StatusConflict)
		return
	}
	if view, err := h.stateView(ws); err == nil && len(view.Resources) > 0 && r.URL.Query().Get("approve") != ws.Name {
		http.Error(w, fmt.Sprintf("its state still has %d resource(s); destroy them first, or type the workspace's name to delete it anyway (the resources stay, untracked)", len(view.Resources)), http.StatusConflict)
		return
	}
	if err := h.store.DeleteWorkspace(r.Context(), ws.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- upload

type uploadRequest struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Archive  []byte `json:"archive"`
	Files    []File `json:"files"`
}

type uploadResponse struct {
	Workspace *Workspace `json:"workspace,omitempty"`
	Notes     []string   `json:"notes"`
	Error     string     `json:"error,omitempty"`
}

var junkRe = regexp.MustCompile(`(^|/)(\.git|__MACOSX|\.idea|\.vscode|node_modules)(/|$)|(^|/)\._|\.DS_Store$`)

// cleanUpload keeps what can be part of a workspace: no clutter, nothing a
// run makes, and no lock file Terraform wrote — it pins HashiCorp's provider
// builds, which OpenTofu's differ from, so init would refuse; OpenTofu
// writes its own. A terraform.tfstate is handed back to import.
func cleanUpload(in []File) (files []File, state *File, notes []string) {
	for _, f := range in {
		p := f.Path
		base := path.Base(p)
		switch {
		case junkRe.MatchString(p):
			continue
		case base == "terraform.tfstate" && !strings.Contains(p, "/"):
			ff := f
			state = &ff
			continue
		case generated(p):
			notes = append(notes, p+": made by a run (providers, state backups, plans), left out")
			continue
		case p == LockFile:
			notes = append(notes, p+": left out — it pins Terraform's provider builds; OpenTofu writes its own on the first run")
			continue
		case !pathRe.MatchString(p) || strings.Contains(p, "//"):
			notes = append(notes, p+": has characters a workspace path can't (letters, digits, . _ - @ + and /), left out")
			continue
		}
		files = append(files, f)
	}
	return files, state, notes
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	var req uploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid upload (too big, or not JSON): "+err.Error(), http.StatusBadRequest)
		return
	}
	var raw []File
	var notes []string
	if len(req.Archive) > 0 {
		af, n, err := ansible.ExtractArchiveRaw(req.Filename, req.Archive)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for _, f := range af {
			raw = append(raw, File{Path: f.Path, Content: f.Content})
		}
		notes = n
	} else {
		raw = req.Files
	}
	files, state, more := cleanUpload(raw)
	notes = append(notes, more...)
	if notes == nil {
		notes = []string{}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(path.Base(req.Filename), ".gz"), ".tar"), path.Ext(req.Filename))
	}
	if name == "" || name == "." {
		name = "Uploaded workspace"
	}
	ws := &Workspace{Name: name, Files: files}
	if err := ValidateFiles(ws.Files); err != nil {
		writeJSON(w, uploadResponse{Notes: notes, Error: err.Error()}, http.StatusBadRequest)
		return
	}
	if err := h.store.CreateWorkspace(r.Context(), ws); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if state != nil {
		var st struct {
			Serial int `json:"serial"`
		}
		switch {
		case OwnBackend(files) != "":
			notes = append(notes, "terraform.tfstate: not imported — the code declares its own backend, where the state is")
		case json.Unmarshal([]byte(state.Content), &st) != nil:
			notes = append(notes, "terraform.tfstate: not imported — it isn't a state file")
		default:
			if enc, err := h.secrets.Encrypt(state.Content); err == nil && h.store.SaveState(r.Context(), ws.ID, st.Serial, enc, "") == nil {
				ws.StateSerial = st.Serial
				notes = append(notes, fmt.Sprintf("terraform.tfstate: imported as the workspace's state (serial %d) — plan to see how the code compares to it", st.Serial))
			}
		}
	}
	writeJSON(w, uploadResponse{Workspace: ws, Notes: notes}, http.StatusCreated)
}

// --- variables

var varKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (h *Handler) handleVariables(w http.ResponseWriter, r *http.Request) {
	vars, err := h.store.Variables(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for i := range vars {
		if !vars[i].Sensitive {
			vars[i].Value, _ = h.secrets.Decrypt(vars[i].valueEnc)
		}
	}
	writeJSON(w, vars, 0)
}

// handleSaveVariables replaces a workspace's variables. A sensitive one
// sent with keep keeps its stored value, which is never sent back.
func (h *Handler) handleSaveVariables(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := h.store.Workspace(r.Context(), id); !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	var req []Variable
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	current, _ := h.store.Variables(r.Context(), id)
	stored := map[string]string{}
	for _, v := range current {
		stored[v.Kind+"\x00"+v.Key] = v.valueEnc
	}
	seen := map[string]bool{}
	var out []Variable
	for _, v := range req {
		v.Key = strings.TrimSpace(v.Key)
		if v.Kind != "terraform" && v.Kind != "env" {
			http.Error(w, "a variable is a terraform or an env one", http.StatusBadRequest)
			return
		}
		if !varKeyRe.MatchString(v.Key) {
			http.Error(w, fmt.Sprintf("%q isn't a valid variable name (letters, digits and _)", v.Key), http.StatusBadRequest)
			return
		}
		k := v.Kind + "\x00" + v.Key
		if seen[k] {
			http.Error(w, v.Key+" is listed twice", http.StatusBadRequest)
			return
		}
		seen[k] = true
		if v.Keep && v.Sensitive && stored[k] != "" {
			v.valueEnc = stored[k]
		} else {
			enc, err := h.secrets.Encrypt(v.Value)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			v.valueEnc = enc
		}
		out = append(out, v)
	}
	if err := h.store.ReplaceVariables(r.Context(), id, out); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.handleVariables(w, r)
}

// --- state

type StateResource struct {
	Address   string `json:"address"`
	Mode      string `json:"mode"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Module    string `json:"module,omitempty"`
	Provider  string `json:"provider"`
	Instances int    `json:"instances"`
}

type StateOutput struct {
	Name      string `json:"name"`
	Value     any    `json:"value,omitempty"`
	Sensitive bool   `json:"sensitive"`
}

type StateView struct {
	Managed    bool            `json:"managed"`
	OwnBackend string          `json:"ownBackend,omitempty"`
	Serial     int             `json:"serial"`
	Resources  []StateResource `json:"resources"`
	Outputs    []StateOutput   `json:"outputs"`
	Versions   []StateVersion  `json:"versions"`
}

func (h *Handler) decryptState(ws *Workspace) ([]byte, error) {
	if ws.stateEnc == "" {
		return nil, nil
	}
	s, err := h.secrets.Decrypt(ws.stateEnc)
	return []byte(s), err
}

func (h *Handler) stateView(ws *Workspace) (*StateView, error) {
	v := &StateView{Managed: ws.OwnBackend == "", OwnBackend: ws.OwnBackend, Serial: ws.StateSerial, Resources: []StateResource{}, Outputs: []StateOutput{}}
	raw, err := h.decryptState(ws)
	if err != nil || len(raw) == 0 {
		return v, err
	}
	var st struct {
		Outputs map[string]struct {
			Value     any  `json:"value"`
			Sensitive bool `json:"sensitive"`
		} `json:"outputs"`
		Resources []struct {
			Module    string `json:"module"`
			Mode      string `json:"mode"`
			Type      string `json:"type"`
			Name      string `json:"name"`
			Provider  string `json:"provider"`
			Instances []any  `json:"instances"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return v, err
	}
	for _, r := range st.Resources {
		addr := r.Type + "." + r.Name
		if r.Mode == "data" {
			addr = "data." + addr
		}
		if r.Module != "" {
			addr = r.Module + "." + addr
		}
		provider := r.Provider
		if i := strings.Index(provider, `"]`); i > 0 {
			provider = strings.TrimPrefix(provider[:i], `provider["`)
		}
		v.Resources = append(v.Resources, StateResource{Address: addr, Mode: r.Mode, Type: r.Type, Name: r.Name, Module: r.Module, Provider: provider, Instances: len(r.Instances)})
	}
	for name, o := range st.Outputs {
		out := StateOutput{Name: name, Sensitive: o.Sensitive}
		if !o.Sensitive {
			out.Value = o.Value
		}
		v.Outputs = append(v.Outputs, out)
	}
	return v, nil
}

func (h *Handler) handleState(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	v, err := h.stateView(ws)
	if err != nil {
		http.Error(w, "reading the state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v.Versions, _ = h.store.StateVersions(r.Context(), ws.ID)
	writeJSON(w, v, 0)
}

// --- runs

// jobFor gathers what a run of ws needs, decrypted.
func (h *Handler) jobFor(ctx context.Context, ws *Workspace, run *Run) (Job, error) {
	job := Job{Run: run, Files: ws.Files, Managed: ws.OwnBackend == ""}
	vars, err := h.store.Variables(ctx, ws.ID)
	if err != nil {
		return job, err
	}
	for _, v := range vars {
		if v.Value, err = h.secrets.Decrypt(v.valueEnc); err != nil {
			return job, fmt.Errorf("variable %s couldn't be decrypted: %v", v.Key, err)
		}
		job.Vars = append(job.Vars, v)
	}
	if job.Managed {
		if job.State, err = h.decryptState(ws); err != nil {
			return job, fmt.Errorf("the state couldn't be decrypted: %v", err)
		}
	}
	return job, nil
}

// start saves run and starts it; when it ends, its plan and the state it
// left are stored, encrypted.
func (h *Handler) start(ctx context.Context, ws *Workspace, run *Run, job Job, after func(*Run)) error {
	if err := h.store.CreateRun(ctx, run); err != nil {
		return err
	}
	job.Run = run
	err := h.runner.Start(job,
		func(l string) { h.store.SaveLog(context.Background(), run.ID, l) },
		func(status, errMsg, logText string, res Result, release func()) {
			bg := context.Background()
			run.Status, run.Error, run.Log = status, errMsg, logText
			if res.Plan != nil && status == "success" {
				if enc, err := h.secrets.Encrypt(base64.StdEncoding.EncodeToString(res.Plan)); err == nil {
					run.planEnc = enc
				}
			}
			if res.State != nil {
				if enc, err := h.secrets.Encrypt(string(res.State)); err == nil {
					if err := h.store.SaveState(bg, ws.ID, res.Serial, enc, run.ID); err != nil {
						log.Printf("terraform: saving state of %s: %v", ws.ID, err)
					}
				}
			}
			if res.LockFile != "" {
				h.store.SaveLockFile(bg, ws.ID, res.LockFile)
			}
			// The state is saved: the workspace is free before the run
			// shows as finished.
			release()
			if err := h.store.FinishRun(bg, run); err != nil {
				log.Printf("terraform: saving run %s: %v", run.ID, err)
			}
			if after != nil {
				after(run)
			}
		})
	if err != nil {
		run.Status, run.Error = "failed", err.Error()
		h.store.FinishRun(ctx, run)
	}
	return err
}

type planRequest struct {
	Kind string `json:"kind"` // "plan" | "destroy-plan"
	// Replace are resources to plan for replacement (taint's successor).
	Replace []string `json:"replace"`
}

func (h *Handler) handlePlan(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	var req planRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Kind != "plan" && req.Kind != "destroy-plan" {
		http.Error(w, "kind must be plan or destroy-plan", http.StatusBadRequest)
		return
	}
	if err := ValidateFiles(ws.Files); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Replace) > 0 && req.Kind != "plan" {
		http.Error(w, "replacing resources is for a plan, not a destroy plan", http.StatusBadRequest)
		return
	}
	for _, addr := range req.Replace {
		if !ValidReplaceAddr(addr) {
			http.Error(w, fmt.Sprintf("%q isn't a resource address to replace (e.g. aws_instance.web or module.app.aws_instance.web[0])", addr), http.StatusBadRequest)
			return
		}
	}
	// The standard, enforced: code that breaks a rule marked enforced isn't
	// planned (formatting, TF010, is checked by the run itself).
	std, err := h.standardOf(r.Context(), ws, ws.Files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if errs := std.Errors(); len(errs) > 0 {
		http.Error(w, blockedMessage(errs), http.StatusUnprocessableEntity)
		return
	}
	run := &Run{WorkspaceID: ws.ID, WorkspaceName: ws.Name, Kind: req.Kind, ContentSHA: ContentSHA(ws.Files), StateSerial: ws.StateSerial,
		StartedBy: startedBy(r.Context()), Options: RunOptions{Replace: req.Replace}, Checks: std.Findings}
	job, err := h.jobFor(r.Context(), ws, run)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.start(r.Context(), ws, run, job, nil); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, run, http.StatusCreated)
}

type applyRequest struct {
	Approve string `json:"approve"`
}

// planMaxAge is how long a plan can be applied after it was made.
const planMaxAge = 24 * time.Hour

// handleApply applies one plan run's saved plan, exactly. It's refused
// unless the plan is current: made from the code there is now, against the
// state there is now, not applied yet, under a day old, with changes. A
// destroy plan needs the workspace's name typed out.
func (h *Handler) handleApply(w http.ResponseWriter, r *http.Request) {
	plan, ok := h.store.Run(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	var req applyRequest
	json.NewDecoder(r.Body).Decode(&req)
	ws, ok := h.store.Workspace(r.Context(), plan.WorkspaceID)
	if !ok {
		http.Error(w, "its workspace no longer exists", http.StatusNotFound)
		return
	}
	// refusal is why this plan can't be applied, "" when it can — its own
	// variable, so no earlier lookup's "ok" can let a refused apply through.
	s := plan.Summary
	refusal := ""
	switch {
	case plan.Kind != "plan" && plan.Kind != "destroy-plan":
		refusal = "only a plan can be applied"
	case plan.Status != "success" || plan.planEnc == "":
		refusal = "this plan didn't succeed, so there's nothing to apply"
	case plan.AppliedByRun != "":
		refusal = "this plan was already applied (run " + plan.AppliedByRun + ")"
	case time.Since(plan.StartedAt) > planMaxAge:
		refusal = "this plan is over a day old; plan again"
	case plan.ContentSHA != ContentSHA(ws.Files):
		refusal = "the code changed since this plan; plan again"
	case ws.OwnBackend == "" && plan.StateSerial != ws.StateSerial:
		refusal = "the state changed since this plan (another apply ran); plan again"
	case s.Add+s.Change+s.Destroy+s.Replace+s.Import == 0:
		refusal = "this plan has no changes to apply"
	case plan.Kind == "destroy-plan" && strings.TrimSpace(req.Approve) != ws.Name:
		refusal = "applying a destroy plan deletes real resources: type the workspace's name to approve"
	case s.Destroy+s.Replace > 0 && strings.TrimSpace(req.Approve) != ws.Name:
		refusal = "this plan destroys or replaces real resources: type the workspace's name to approve"
	}
	if refusal != "" {
		http.Error(w, refusal, http.StatusConflict)
		return
	}
	enc, err := h.secrets.Decrypt(plan.planEnc)
	if err != nil {
		http.Error(w, "the saved plan couldn't be decrypted: "+err.Error(), http.StatusInternalServerError)
		return
	}
	planFile, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		http.Error(w, "the saved plan is damaged", http.StatusInternalServerError)
		return
	}
	run := &Run{WorkspaceID: ws.ID, WorkspaceName: ws.Name, Kind: "apply", ContentSHA: plan.ContentSHA, AppliesRunID: plan.ID, StateSerial: ws.StateSerial, Summary: plan.Summary,
		StartedBy: startedBy(r.Context()), Options: plan.Options}
	job, err := h.jobFor(r.Context(), ws, run)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	job.Plan = planFile
	if err := h.start(r.Context(), ws, run, job, nil); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	h.store.MarkApplied(r.Context(), plan.ID, run.ID)
	writeJSON(w, run, http.StatusCreated)
}

func (h *Handler) handleRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.store.Runs(r.Context(), r.URL.Query().Get("workspace"), 100)
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
