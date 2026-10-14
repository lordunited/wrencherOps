// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"devops-wrapper/internal/access"
)

// standardView is a workspace's code against the standard: its structure,
// findings, and the 30 concepts.
type standardView struct {
	*Analysis
	Concepts []Concept `json:"concepts"`
	// Blocking is whether an enforced rule is broken: Plan is refused.
	Blocking bool `json:"blocking"`
}

func (h *Handler) standardOf(ctx context.Context, ws *Workspace, files []File) (*standardView, error) {
	vars, err := h.store.Variables(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	a := Analyze(files, AnalyzeOptions{VarKeys: varKeys(vars)})
	return &standardView{Analysis: a, Concepts: Concepts(a, ws, vars), Blocking: len(a.Errors()) > 0}, nil
}

// varKeys are the Terraform variables that have a value.
func varKeys(vars []Variable) map[string]bool {
	out := map[string]bool{}
	for _, v := range vars {
		if v.Kind != "env" {
			out[v.Key] = true
		}
	}
	return out
}

// handleAnalyze checks a workspace's code against the standard: the saved
// code, or the files given (what's in the editor, not saved yet).
func (h *Handler) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.store.Workspace(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	var req struct {
		Files *[]File `json:"files"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes*2)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	files := ws.Files
	if req.Files != nil {
		files = *req.Files
	}
	v, err := h.standardOf(r.Context(), ws, files)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, v, 0)
}

// handleStandard is the standard itself: every rule, the 30 concepts, the
// examples that can be added for them, and the providers that can be.
func (h *Handler) handleStandard(w http.ResponseWriter, r *http.Request) {
	type snippet struct {
		Concept string `json:"concept"`
		Title   string `json:"title"`
		File    string `json:"file"`
		Code    string `json:"code"`
	}
	snippets := make([]snippet, 0, len(Snippets))
	for _, s := range Snippets {
		snippets = append(snippets, snippet{s.Concept, s.Title, s.File, strings.ReplaceAll(s.Code, "{n}", "")})
	}
	writeJSON(w, map[string]any{"rules": Rules, "concepts": ConceptNames(), "snippets": snippets, "providers": ProviderChoices}, 0)
}

// handleSnippet adds a concept's example, or a provider, to the files
// given (what's in the editor) and returns them: like Format, nothing is
// saved until the editor saves.
func (h *Handler) handleSnippet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files    []File `json:"files"`
		Concept  string `json:"concept"`
		Provider string `json:"provider"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes*2)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	type result struct {
		Files []File `json:"files"`
		File  string `json:"file"`
		Line  int    `json:"line,omitempty"`
	}
	var res result
	var err error
	switch {
	case req.Provider != "":
		res.Files, res.File, err = AddProvider(req.Files, req.Provider)
	default:
		s, ok := SnippetFor(req.Concept)
		if !ok {
			http.Error(w, fmt.Sprintf("%s: %v", req.Concept, ErrNoSnippet), http.StatusBadRequest)
			return
		}
		res.Files, res.File, res.Line, err = AddSnippet(req.Files, s)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := ValidateFiles(res.Files); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, res, 0)
}

// fmtTimeout bounds formatting one request's files.
const fmtTimeout = 30 * time.Second

// handleFmt formats files with tofu fmt and returns them: the editor shows
// the result, and saving keeps it.
func (h *Handler) handleFmt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files []File `json:"files"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes*2)).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Files) > maxFiles {
		http.Error(w, fmt.Sprintf("at most %d files", maxFiles), http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fmtTimeout)
	defer cancel()
	type result struct {
		Files   []File            `json:"files"`
		Changed []string          `json:"changed"`
		Errors  map[string]string `json:"errors"`
	}
	res := result{Files: []File{}, Changed: []string{}, Errors: map[string]string{}}
	for _, f := range req.Files {
		if !strings.HasSuffix(f.Path, ".tf") && !strings.HasSuffix(f.Path, ".tfvars") {
			res.Files = append(res.Files, f)
			continue
		}
		out, err := FormatHCL(ctx, f.Content)
		if err != nil {
			res.Errors[f.Path] = err.Error()
			res.Files = append(res.Files, f)
			continue
		}
		if out != f.Content {
			res.Changed = append(res.Changed, f.Path)
		}
		res.Files = append(res.Files, File{Path: f.Path, Content: out})
	}
	writeJSON(w, res, 0)
}

// FormatHCL is content formatted by tofu fmt (read from stdin), run with a
// clean environment.
func FormatHCL(ctx context.Context, content string) (string, error) {
	home, err := os.MkdirTemp("", "panel-tofu-fmt-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	var out, errOut bytes.Buffer
	cmd := exec.CommandContext(ctx, "tofu", "fmt", "-no-color", "-")
	cmd.Dir = home
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=" + home, "CHECKPOINT_DISABLE=1"}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(content), &out, &errOut
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// startedBy is the signed-in user starting a run.
func startedBy(ctx context.Context) string {
	if a := access.FromContext(ctx); a != nil {
		return a.Username
	}
	return ""
}

// blockedMessage is why a plan is refused: the enforced rules it breaks.
func blockedMessage(errs []Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "not planned: the code breaks %d enforced rule", len(errs))
	if len(errs) != 1 {
		b.WriteString("s")
	}
	b.WriteString(" of the standard —")
	for i, f := range errs {
		if i == 5 {
			fmt.Fprintf(&b, " … and %d more (see Standards)", len(errs)-5)
			break
		}
		where := ""
		if f.File != "" {
			where = fmt.Sprintf(" (%s:%d)", f.File, f.Line)
		}
		fmt.Fprintf(&b, " %s %s%s;", f.Rule, f.Message, where)
	}
	return strings.TrimSuffix(b.String(), ";")
}
