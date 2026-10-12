// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/secretbox"
)

// TestTerraformEndToEnd runs real OpenTofu (tofu must be installed, with
// its registry reachable) against a throwaway Postgres (TEST_DATABASE_URL).
func TestTerraformEndToEnd(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if _, err := exec.LookPath("tofu"); err != nil || dbURL == "" {
		t.Skip("needs tofu installed and TEST_DATABASE_URL to a throwaway Postgres")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	store := NewStore(pool)
	h := NewHandler(store, box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}
	wait := func(id string) Run {
		t.Helper()
		var r Run
		for i := 0; i < 600; i++ {
			json.Unmarshal(do("GET", "/api/terraform/runs/"+id, nil).Body.Bytes(), &r)
			if r.Status != "running" {
				return r
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("run %s still running:\n%s", id, r.Log)
		return r
	}
	planWith := func(wsID string, req map[string]any) Run {
		t.Helper()
		rec := do("POST", "/api/terraform/workspaces/"+wsID+"/runs", req)
		var r Run
		json.Unmarshal(rec.Body.Bytes(), &r)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%v: %d %s", req, rec.Code, rec.Body)
		}
		return wait(r.ID)
	}
	plan := func(wsID, kind string) Run {
		t.Helper()
		return planWith(wsID, map[string]any{"kind": kind})
	}
	save := func(wsID string, files []File) {
		t.Helper()
		if rec := do("PUT", "/api/terraform/workspaces/"+wsID, map[string]any{"files": files}); rec.Code != http.StatusOK {
			t.Fatalf("update: %d %s", rec.Code, rec.Body)
		}
	}
	apply := func(planID, approve string) (*httptest.ResponseRecorder, Run) {
		rec := do("POST", "/api/terraform/runs/"+planID+"/apply", map[string]any{"approve": approve})
		var r Run
		json.Unmarshal(rec.Body.Bytes(), &r)
		if rec.Code == http.StatusCreated {
			r = wait(r.ID)
		}
		return rec, r
	}

	// The code uses only terraform_data, built into OpenTofu: the test
	// needs no provider download. The registry is used only for import
	// (and the lock file), when it can be reached.
	registry := registryReachable()
	if !registry {
		t.Log("registry.opentofu.org can't be reached: skipping the import and lock file steps")
	}
	rec := do("POST", "/api/terraform/workspaces", map[string]any{"name": fmt.Sprintf("e2e-%d", time.Now().UnixNano()), "files": offlineCode})
	var ws Workspace
	json.Unmarshal(rec.Body.Bytes(), &ws)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	// A sensitive variable a local-exec prints: the log must mask it.
	ws.Files = append(ws.Files, File{Path: "secret.tf", Content: `variable "token" {
  description = "A token the leak test prints."
  type        = string
  sensitive   = true
}

resource "terraform_data" "leak" {
  provisioner "local-exec" {
    command     = "echo token-is-$TOKEN"
    environment = { TOKEN = nonsensitive(var.token) }
  }
}
`})
	if rec := do("PUT", "/api/terraform/workspaces/"+ws.ID, map[string]any{"files": ws.Files}); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if rec := do("PUT", "/api/terraform/workspaces/"+ws.ID+"/variables", []map[string]any{
		{"key": "environment", "kind": "terraform", "value": "prod"},
		{"key": "token", "kind": "terraform", "value": "tok-9f8e7d", "sensitive": true},
	}); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "tok-9f8e7d") {
		t.Fatalf("variables (a sensitive value must never come back): %d %s", rec.Code, rec.Body)
	}

	p1 := plan(ws.ID, "plan")
	if p1.Status != "success" || p1.Summary.Add != 3 {
		t.Fatalf("plan: %s %s %+v\n%s", p1.Status, p1.Error, p1.Summary, p1.Log)
	}
	t.Logf("plan: +%d ~%d -%d  %v", p1.Summary.Add, p1.Summary.Change, p1.Summary.Destroy, p1.Summary.Changes)
	rec, a1 := apply(p1.ID, "")
	if rec.Code != http.StatusCreated || a1.Status != "success" {
		t.Fatalf("apply: %d %s %s\n%s", rec.Code, rec.Body, a1.Error, a1.Log)
	}
	if strings.Contains(a1.Log, "tok-9f8e7d") || !strings.Contains(a1.Log, "token-is-********") {
		t.Errorf("the sensitive value isn't masked in the apply log:\n%s", a1.Log)
	}
	var st StateView
	json.Unmarshal(do("GET", "/api/terraform/workspaces/"+ws.ID+"/state", nil).Body.Bytes(), &st)
	if !st.Managed || st.Serial < 1 || len(st.Resources) != 3 || len(st.Outputs) != 1 || st.Outputs[0].Value != "prod-app" || len(st.Versions) != 1 {
		t.Fatalf("state: %+v", st)
	}
	t.Logf("state serial %d: %d resources, output name = %v", st.Serial, len(st.Resources), st.Outputs[0].Value)
	// Applying the same plan again is refused — and nothing runs: a refusal
	// once answered 409 yet went on to apply.
	again := do("POST", "/api/terraform/runs/"+p1.ID+"/apply", map[string]any{})
	runs, _ := store.Runs(ctx, ws.ID, 20)
	if again.Code != http.StatusConflict || len(runs) != 2 {
		t.Errorf("second apply of the same plan: %d %s, %d runs (want 2: the plan and its one apply)", again.Code, strings.TrimSpace(again.Body.String()), len(runs))
	}

	// Nothing changed: the plan has nothing to apply.
	p2 := plan(ws.ID, "plan")
	if rec, _ := apply(p2.ID, ""); p2.Summary.Add+p2.Summary.Change+p2.Summary.Destroy != 0 || rec.Code != http.StatusConflict {
		t.Errorf("a no-change plan: %+v, apply %d", p2.Summary, rec.Code)
	}

	// The standard is enforced: code that breaks an enforced rule isn't
	// planned, and says why.
	base := append([]File(nil), ws.Files...)
	save(ws.ID, append(append([]File(nil), base...), File{Path: "loose.tf", Content: "variable \"loose\" {\n  default = 1\n}\n"}))
	if rec := do("POST", "/api/terraform/workspaces/"+ws.ID+"/runs", map[string]any{"kind": "plan"}); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "TF003") || !strings.Contains(rec.Body.String(), "TF004") {
		t.Errorf("a plan breaking the standard: %d %s", rec.Code, rec.Body)
	}
	var std standardView
	json.Unmarshal(do("POST", "/api/terraform/workspaces/"+ws.ID+"/analyze", nil).Body.Bytes(), &std)
	if !std.Blocking || len(std.Concepts) != 30 {
		t.Errorf("analyze: blocking %v, %d concepts", std.Blocking, len(std.Concepts))
	}
	// Unsaved code is checked as given.
	json.Unmarshal(do("POST", "/api/terraform/workspaces/"+ws.ID+"/analyze", map[string]any{"files": base}).Body.Bytes(), &std)
	if std.Blocking {
		t.Errorf("the base code blocks: %+v", std.Errors())
	}
	// Formatting (TF010) is checked by the run; Format fixes it.
	messy := append(append([]File(nil), base...), File{Path: "loose.tf", Content: "variable \"loose\" {\ndescription=\"d\"\n  type = number\n  default = 1\n}\n"})
	save(ws.ID, messy)
	if r := plan(ws.ID, "plan"); r.Status != "failed" || !strings.Contains(r.Error, "formatted") {
		t.Errorf("an unformatted plan: %s %s", r.Status, r.Error)
	}
	var formatted struct {
		Files   []File            `json:"files"`
		Changed []string          `json:"changed"`
		Errors  map[string]string `json:"errors"`
	}
	json.Unmarshal(do("POST", "/api/terraform/fmt", map[string]any{"files": messy}).Body.Bytes(), &formatted)
	if strings.Join(formatted.Changed, ",") != "loose.tf" || len(formatted.Errors) != 0 {
		t.Fatalf("fmt: %+v", formatted)
	}
	save(ws.ID, formatted.Files)
	if r := plan(ws.ID, "plan"); r.Status != "success" || r.Summary.Add+r.Summary.Change+r.Summary.Destroy != 0 {
		t.Errorf("formatted, the plan runs (with no changes): %s %s %+v\n%s", r.Status, r.Error, r.Summary, r.Log)
	}
	save(ws.ID, base)

	// Two plans; applying one moves the state, so the other is stale.
	do("PUT", "/api/terraform/workspaces/"+ws.ID+"/variables", []map[string]any{
		{"key": "environment", "kind": "terraform", "value": "staging"},
		{"key": "token", "kind": "terraform", "sensitive": true, "keep": true},
	})
	pa, pb := plan(ws.ID, "plan"), plan(ws.ID, "plan")
	if pa.Summary.Replace == 0 {
		t.Fatalf("changing the environment should replace the name: %+v", pa.Summary)
	}
	// It replaces the pet: applying it needs the workspace's name.
	if rec, _ := apply(pb.ID, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "type the workspace's name") {
		t.Errorf("a replacing plan applied without approval: %d %s", rec.Code, rec.Body)
	}
	if _, r := apply(pb.ID, ws.Name); r.Status != "success" {
		t.Fatalf("apply b: %s", r.Error)
	}
	if rec, _ := apply(pa.ID, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "state changed") {
		t.Errorf("a stale plan: %d %s", rec.Code, rec.Body)
	}
	// Replace (taint's successor): a resource planned for replacement.
	if rec := do("POST", "/api/terraform/workspaces/"+ws.ID+"/runs", map[string]any{"kind": "plan", "replace": []string{"data.x.y"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("replacing a data source: %d", rec.Code)
	}
	pr := planWith(ws.ID, map[string]any{"kind": "plan", "replace": []string{"terraform_data.announce"}})
	if pr.Status != "success" || pr.Summary.Replace != 1 || len(pr.Options.Replace) != 1 {
		t.Fatalf("replace plan: %s %+v %+v\n%s", pr.Status, pr.Summary, pr.Options, pr.Log)
	}
	if _, r := apply(pr.ID, ws.Name); r.Status != "success" {
		t.Fatalf("apply the replacement: %s", r.Error)
	}
	// Import: an import block brings an existing object in.
	resources := 3
	if registry {
		withImport := append(append([]File(nil), base...), File{Path: "import.tf", Content: `terraform {
  required_providers {
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

import {
  to = random_id.imported
  id = "p-9hUg"
}

resource "random_id" "imported" {
  byte_length = 4
}
`})
		save(ws.ID, withImport)
		pi := plan(ws.ID, "plan")
		if pi.Status != "success" || pi.Summary.Import != 1 {
			t.Fatalf("import plan: %s %s %+v\n%s", pi.Status, pi.Error, pi.Summary, pi.Log)
		}
		if _, r := apply(pi.ID, ""); r.Status != "success" {
			t.Fatalf("apply the import: %s\n%s", r.Error, r.Log)
		}
		ws.Files = withImport
		resources++
	}

	// Changed code makes a plan stale too.
	pc := plan(ws.ID, "plan")
	ws.Files = append(ws.Files, File{Path: "extra.tf", Content: "# nothing\n"})
	do("PUT", "/api/terraform/workspaces/"+ws.ID, map[string]any{"files": ws.Files})
	if rec, _ := apply(pc.ID, ""); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "code changed") {
		t.Errorf("code changed after the plan: %d %s", rec.Code, rec.Body)
	}

	// Deleting a workspace with resources needs its name.
	if rec := do("DELETE", "/api/terraform/workspaces/"+ws.ID, nil); rec.Code != http.StatusConflict {
		t.Errorf("deleting with resources: %d", rec.Code)
	}
	// Destroy: needs the name typed.
	pd := plan(ws.ID, "destroy-plan")
	if pd.Summary.Destroy != resources {
		t.Fatalf("destroy plan: %+v\n%s", pd.Summary, pd.Log)
	}
	if rec, _ := apply(pd.ID, ""); rec.Code != http.StatusConflict {
		t.Error("a destroy applied without approval")
	}
	if _, r := apply(pd.ID, ws.Name); r.Status != "success" {
		t.Fatalf("destroy: %s\n%s", r.Error, r.Log)
	}
	json.Unmarshal(do("GET", "/api/terraform/workspaces/"+ws.ID+"/state", nil).Body.Bytes(), &st)
	if len(st.Resources) != 0 || len(st.Versions) < 3 {
		t.Errorf("after destroy: %d resources, %d versions", len(st.Resources), len(st.Versions))
	}
	got, _ := store.Workspace(ctx, ws.ID)
	hasLock := false
	for _, f := range got.Files {
		hasLock = hasLock || f.Path == LockFile
	}
	if registry && !hasLock {
		t.Error("the lock file init wrote wasn't kept")
	}

	// Upload with a state to import.
	rec = do("POST", "/api/terraform/workspaces/upload", map[string]any{"filename": "adopt.zip", "files": []File{
		{Path: "main.tf", Content: "resource \"null_resource\" \"x\" {}\n"},
		{Path: "terraform.tfstate", Content: `{"version":4,"serial":7,"lineage":"abc","outputs":{},"resources":[]}`},
	}, "name": "adopted"})
	var up uploadResponse
	json.Unmarshal(rec.Body.Bytes(), &up)
	if rec.Code != http.StatusCreated || up.Workspace.StateSerial != 7 || !strings.Contains(strings.Join(up.Notes, " "), "imported") {
		t.Errorf("upload: %d %s", rec.Code, rec.Body)
	}
}

// offlineCode meets the standard and needs no provider download.
var offlineCode = []File{
	{Path: "versions.tf", Content: `terraform {
  required_version = ">= 1.6"
}
`},
	{Path: "variables.tf", Content: `variable "environment" {
  description = "Where this runs, e.g. staging or production."
  type        = string
  default     = "staging"
}
`},
	{Path: "main.tf", Content: `resource "terraform_data" "name" {
  input            = "${var.environment}-app"
  triggers_replace = [var.environment]
}

resource "terraform_data" "announce" {
  input = terraform_data.name.output
}
`},
	{Path: "outputs.tf", Content: `output "name" {
  description = "The name this workspace made."
  value       = terraform_data.name.output
}
`},
}

// registryReachable reports whether OpenTofu's registry answers.
func registryReachable() bool {
	c := &http.Client{Timeout: 10 * time.Second}
	res, err := c.Get("https://registry.opentofu.org/.well-known/terraform.json")
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}
