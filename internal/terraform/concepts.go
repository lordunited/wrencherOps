// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"fmt"
	"sort"
	"strings"
)

// The panel's Terraform standard follows 30 concepts. Each is shown on a
// workspace: how the panel handles it, what this workspace's code does
// with it, and the rules that check it.

// Concept is one of the 30, as it applies to one workspace.
type Concept struct {
	Name string `json:"name"`
	// Panel is how the panel supports or enforces it.
	Panel string `json:"panel"`
	// Here is what this workspace has of it ("" when nothing).
	Here string `json:"here"`
	// Enforced is whether a rule of it blocks the plan when broken.
	Enforced bool `json:"enforced"`
	// Status is "ok", "error" (an enforced rule is broken), "warning", or
	// "unused".
	Status string   `json:"status"`
	Rules  []string `json:"rules,omitempty"`
}

// conceptPanel is how the panel handles each concept, in order.
var conceptPanel = []struct{ name, panel string }{
	{"Providers", "Every provider must be pinned (source and version) in required_providers. Downloads are cached between runs; the lock file init writes is kept with the code."},
	{"Resources", "Listed with their meta-arguments; every change to them shows in the plan, one line per resource."},
	{"Variables", "Every variable needs a type and a description; secret-like ones must be sensitive, with no default."},
	{"Outputs", "Every output needs a description; secret-like ones must be sensitive. Their values are shown from the state."},
	{"Data Sources", "Listed; read again on every plan."},
	{"Local Values", "Listed; a secret can't be written into one."},
	{"State File", "Kept by the panel, encrypted, with every version that an apply wrote — unless the code has its own backend."},
	{"Backends", "None needed: the panel keeps the state. A remote backend in the code is used instead; a local one is refused."},
	{"Init", "Every run inits first (providers and modules); required_version must be set."},
	{"Plan", "Runs validate, then plan, and saves the plan. Nothing changes."},
	{"Apply", "Applies exactly a saved plan — refused if the code or the state changed since, or after a day. A plan that destroys or replaces anything needs the workspace's name typed."},
	{"Destroy", "A destroy plan, reviewed like any other, then applied with the workspace's name typed."},
	{"Modules", "Local modules (./…) are part of the workspace's files."},
	{"Remote Modules", "Registry modules must have a version, Git ones ?ref=."},
	{"Provisioners", "Allowed, flagged as a last resort."},
	{"Count", "Shown per resource; count and for_each together are refused."},
	{"For_each", "Shown per resource."},
	{"Depends_on", "Allowed; flagged when used, since references already order resources."},
	{"Lifecycle", "Honored by OpenTofu: prevent_destroy refuses a plan that would destroy; ignore_changes = all is flagged."},
	{"Taint", "Replace: plan with chosen resources marked for replacement (-replace, the successor of taint), then apply as usual."},
	{"Import", "Import blocks: add one (address and ID), plan to see what's imported, apply to bring it under management."},
	{"Workspaces", "Each panel workspace is its own code, variables and state; terraform.workspace is its name."},
	{"Workflows", "Write → format (enforced) → plan (standards checked, then validate) → review → apply. Every run is kept with its log."},
	{"Functions", "Counted per workspace; validate checks every call."},
	{"TFC/TFE", "The panel plays its part here: remote runs, state and variables per workspace, policies, run history, per-tab access."},
	{"Sentinel", "Policy as code: the rules on this page run before every plan; the enforced ones block it."},
	{"Interpolation", "Counted; a secret can't be written as a literal anywhere."},
	{"Workspace Variables", "Terraform and environment variables per workspace, sensitive ones encrypted and masked in logs; a required variable must have a value before planning."},
	{"Dynamic Blocks", "Shown per resource."},
	{"Ternary", "Counted (condition ? a : b)."},
}

// ConceptNames are the 30 concepts, in order.
func ConceptNames() []string {
	out := make([]string, len(conceptPanel))
	for i, c := range conceptPanel {
		out[i] = c.name
	}
	return out
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

// Concepts is the 30 concepts for a workspace with analysis a.
func Concepts(a *Analysis, ws *Workspace, vars []Variable) []Concept {
	count := func(f func(Block) bool, blocks ...[]Block) int {
		n := 0
		for _, list := range blocks {
			for _, b := range list {
				if f(b) {
					n++
				}
			}
		}
		return n
	}
	all := [][]Block{a.Resources, a.DataSources, a.Modules}
	remote := count(func(b Block) bool { return b.Remote }, a.Modules)
	local := len(a.Modules) - remote
	provisioners := 0
	for _, r := range a.Resources {
		provisioners += len(r.Provisioners)
	}
	dynamic := 0
	for _, r := range a.Resources {
		dynamic += len(r.Dynamic)
	}
	lifecycle := count(func(b Block) bool { return b.Lifecycle != nil }, a.Resources)
	protected := count(func(b Block) bool { return b.Lifecycle != nil && b.Lifecycle.PreventDestroy }, a.Resources)
	functions := 0
	var fnNames []string
	for name, n := range a.Functions {
		functions += n
		fnNames = append(fnNames, name)
	}
	sort.Strings(fnNames)
	if len(fnNames) > 6 {
		fnNames = append(fnNames[:6], "…")
	}
	tfVars, envVars := 0, 0
	for _, v := range vars {
		if v.Kind == "env" {
			envVars++
		} else {
			tfVars++
		}
	}
	var providers []string
	for _, p := range a.Providers {
		s := p.Name
		if p.Version != "" {
			s += " " + p.Version
		}
		providers = append(providers, s)
	}
	state := "no state yet"
	switch {
	case ws.OwnBackend != "":
		state = "kept in the code's own " + ws.OwnBackend + " backend"
	case ws.StateSerial > 0:
		state = fmt.Sprintf("kept by the panel, serial %d", ws.StateSerial)
	}
	backend := ""
	if a.Backend != "" {
		backend = "backend \"" + a.Backend + "\""
	}

	here := map[string]string{
		"Providers":           strings.Join(providers, ", "),
		"Resources":           nonZero(len(a.Resources), "resource"),
		"Variables":           nonZero(len(a.Variables), "variable"),
		"Outputs":             nonZero(len(a.Outputs), "output"),
		"Data Sources":        nonZero(len(a.DataSources), "data source"),
		"Local Values":        nonZero(len(a.Locals), "local"),
		"State File":          state,
		"Backends":            backend,
		"Init":                a.RequiredVersion,
		"Modules":             nonZero(local, "local module"),
		"Remote Modules":      nonZero(remote, "remote module"),
		"Provisioners":        nonZero(provisioners, "provisioner"),
		"Count":               nonZero(count(func(b Block) bool { return b.Count }, all...), "block"),
		"For_each":            nonZero(count(func(b Block) bool { return b.ForEach }, all...), "block"),
		"Depends_on":          nonZero(count(func(b Block) bool { return len(b.DependsOn) > 0 }, all...), "block"),
		"Lifecycle":           lifecycleHere(lifecycle, protected),
		"Import":              nonZero(len(a.Imports), "import block"),
		"Workspaces":          workspaceHere(a.UsesWorkspace),
		"Functions":           functionsHere(functions, fnNames),
		"Interpolation":       nonZero(a.Interpolations, "interpolation"),
		"Workspace Variables": varsHere(tfVars, envVars),
		"Dynamic Blocks":      nonZero(dynamic, "dynamic block"),
		"Ternary":             nonZero(a.Ternaries, "conditional"),
	}
	// Always in play, for every workspace.
	for _, name := range []string{"Plan", "Apply", "Destroy", "Taint", "Workflows", "TFC/TFE", "Sentinel"} {
		here[name] = "available"
	}
	if ws.StateSerial == 0 && ws.OwnBackend == "" {
		here["State File"] = ""
	}

	rulesOf := map[string][]Rule{}
	for _, r := range Rules {
		rulesOf[r.Concept] = append(rulesOf[r.Concept], r)
	}
	broken := map[string]string{} // concept -> worst severity
	for _, f := range append(a.Findings, a.ParseErrors...) {
		c := f.Concept
		if c == "" {
			c = "Plan"
		}
		if prev, ok := broken[c]; !ok || severityRank(f.Severity) < severityRank(prev) {
			broken[c] = f.Severity
		}
	}
	// Sentinel is the rules as a whole.
	if len(a.Errors()) > 0 {
		broken["Sentinel"] = SeverityError
	}

	out := make([]Concept, 0, len(conceptPanel))
	for _, c := range conceptPanel {
		k := Concept{Name: c.name, Panel: c.panel, Here: here[c.name], Status: "ok"}
		for _, r := range rulesOf[c.name] {
			k.Rules = append(k.Rules, r.ID)
			if r.Severity == SeverityError {
				k.Enforced = true
			}
		}
		if c.name == "Sentinel" {
			k.Enforced = true
		}
		switch broken[c.name] {
		case SeverityError:
			k.Status = "error"
		case SeverityWarning:
			k.Status = "warning"
		default:
			if k.Here == "" {
				k.Status = "unused"
			}
		}
		out = append(out, k)
	}
	return out
}

func nonZero(n int, what string) string {
	if n == 0 {
		return ""
	}
	return plural(n, what)
}

func lifecycleHere(n, protected int) string {
	if n == 0 {
		return ""
	}
	s := plural(n, "resource") + " with lifecycle"
	if protected > 0 {
		s += fmt.Sprintf(", %d protected from destroy", protected)
	}
	return s
}

func workspaceHere(uses bool) string {
	if uses {
		return "uses terraform.workspace"
	}
	return "this workspace"
}

func functionsHere(n int, names []string) string {
	if n == 0 {
		return ""
	}
	return plural(n, "call") + ": " + strings.Join(names, ", ")
}

func varsHere(tf, env int) string {
	if tf+env == 0 {
		return ""
	}
	parts := []string{}
	if tf > 0 {
		parts = append(parts, plural(tf, "variable"))
	}
	if env > 0 {
		parts = append(parts, plural(env, "environment variable"))
	}
	return strings.Join(parts, ", ")
}
