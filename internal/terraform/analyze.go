// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// The panel's Terraform standard, from its 30 concepts: Analyze reads a
// workspace's code into its structure (providers, resources and their
// meta-arguments, variables, outputs, modules…) and checks it against the
// rules below. Rules with SeverityError are enforced: a workspace that
// breaks one isn't planned. The others are advice.

const (
	SeverityError   = "error"   // enforced: blocks plan
	SeverityWarning = "warning" // advice
	SeverityInfo    = "info"
)

// Rule is one check of the standard.
type Rule struct {
	ID       string `json:"id"`
	Concept  string `json:"concept"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Why      string `json:"why"`
}

// Rules is the whole standard.
var Rules = []Rule{
	{"TF001", "Providers", SeverityError, "Every provider is pinned in required_providers",
		"An unpinned provider upgrades itself on a fresh init and can change or break resources without a code change."},
	{"TF002", "Init", SeverityError, "The terraform block sets required_version",
		"The code states which OpenTofu/Terraform versions it was written for, so an incompatible one fails at init instead of mid-apply."},
	{"TF003", "Variables", SeverityError, "Every variable has a type",
		"A typed variable rejects a wrong value at plan time instead of producing a surprising resource."},
	{"TF004", "Variables", SeverityError, "Every variable has a description",
		"Whoever sets the workspace's variables needs to know what each one is."},
	{"TF005", "Variables", SeverityError, "Secret-like variables are sensitive and have no default",
		"A password, token or key variable is hidden from plans and logs only when marked sensitive; its value belongs in the workspace's sensitive variables, never in code."},
	{"TF006", "Interpolation", SeverityError, "No secret is hardcoded",
		"A password, token or key written as a literal in code ends up in Git, the plan and the state. Use a sensitive variable."},
	{"TF007", "Outputs", SeverityError, "Every output has a description",
		"Outputs are the workspace's interface; each says what it is."},
	{"TF008", "Outputs", SeverityError, "Secret-like outputs are sensitive",
		"An output named like a password, token or key is printed after every apply unless marked sensitive."},
	{"TF009", "Remote Modules", SeverityError, "Remote modules are pinned to a version",
		"A registry module needs version, a Git module ?ref=: otherwise each init can pull different code."},
	{"TF010", "Workflows", SeverityError, "The code is formatted (tofu fmt)",
		"One canonical layout keeps diffs about changes, not whitespace. Format fixes it in one click."},
	{"TF011", "Backends", SeverityError, "No local backend",
		"Each run works in a throwaway folder: a local backend's state would be lost after the run. Leave the backend out (the panel keeps the state, encrypted and versioned) or use a remote one."},
	{"TF012", "Workspace Variables", SeverityError, "Every required variable has a value",
		"A variable without a default needs a value in the workspace's variables (or a .tfvars file); runs are non-interactive."},
	{"TF013", "Count", SeverityError, "count and for_each aren't used together",
		"A block can be repeated by count or by for_each, not both."},
	{"TF014", "Provisioners", SeverityWarning, "Provisioners are a last resort",
		"local-exec/remote-exec run outside the plan: their effect isn't shown, tracked or undone. Prefer a provider resource, cloud-init or Ansible."},
	{"TF015", "Depends_on", SeverityWarning, "depends_on only for hidden dependencies",
		"References already order resources; depends_on makes plans more conservative (more unknown values). Keep it for dependencies the code can't express."},
	{"TF016", "Lifecycle", SeverityWarning, "ignore_changes = all hides drift",
		"Ignoring every attribute means the code no longer describes the resource. List the attributes that really change outside Terraform."},
	{"TF017", "Data Sources", SeverityInfo, "Data sources read at every plan",
		"A data source is read on each plan; one that can't be read fails the plan."},
}

// RuleByID finds a rule.
func RuleByID(id string) (Rule, bool) {
	for _, r := range Rules {
		if r.ID == id {
			return r, true
		}
	}
	return Rule{}, false
}

// Finding is one place the code breaks a rule.
type Finding struct {
	Rule     string `json:"rule"`
	Concept  string `json:"concept"`
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// Lifecycle is a block's lifecycle settings.
type Lifecycle struct {
	PreventDestroy      bool `json:"preventDestroy,omitempty"`
	CreateBeforeDestroy bool `json:"createBeforeDestroy,omitempty"`
	IgnoreChanges       bool `json:"ignoreChanges,omitempty"`
	IgnoreAll           bool `json:"ignoreAll,omitempty"`
	ReplaceTriggeredBy  bool `json:"replaceTriggeredBy,omitempty"`
}

// Block is a resource, data source or module, with its meta-arguments.
type Block struct {
	Kind         string     `json:"kind"` // resource, data, module
	Type         string     `json:"type,omitempty"`
	Name         string     `json:"name"`
	Address      string     `json:"address"`
	File         string     `json:"file"`
	Line         int        `json:"line"`
	Provider     string     `json:"provider,omitempty"`
	Count        bool       `json:"count,omitempty"`
	ForEach      bool       `json:"forEach,omitempty"`
	DependsOn    []string   `json:"dependsOn,omitempty"`
	Lifecycle    *Lifecycle `json:"lifecycle,omitempty"`
	Provisioners []string   `json:"provisioners,omitempty"`
	Dynamic      []string   `json:"dynamic,omitempty"`
	// Modules only.
	Source  string `json:"source,omitempty"`
	Version string `json:"version,omitempty"`
	Remote  bool   `json:"remote,omitempty"`
}

// ProviderReq is a provider the code needs.
type ProviderReq struct {
	Name     string   `json:"name"`
	Source   string   `json:"source,omitempty"`
	Version  string   `json:"version,omitempty"`
	Declared bool     `json:"declared"` // in required_providers
	Aliases  []string `json:"aliases,omitempty"`
}

// VariableDecl is a declared input variable.
type VariableDecl struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	HasDefault  bool   `json:"hasDefault"`
	Sensitive   bool   `json:"sensitive"`
	Validations int    `json:"validations,omitempty"`
	File        string `json:"file"`
	Line        int    `json:"line"`
}

// OutputDecl is a declared output.
type OutputDecl struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Sensitive   bool   `json:"sensitive"`
	File        string `json:"file"`
	Line        int    `json:"line"`
}

// ImportDecl is an import block: an existing object brought under
// management by the next plan and apply.
type ImportDecl struct {
	To string `json:"to"`
	ID string `json:"id"`
}

// Analysis is a workspace's code, read: its structure, the language
// features it uses, and what breaks the standard.
type Analysis struct {
	RequiredVersion string         `json:"requiredVersion,omitempty"`
	Backend         string         `json:"backend,omitempty"`
	Providers       []ProviderReq  `json:"providers"`
	Resources       []Block        `json:"resources"`
	DataSources     []Block        `json:"dataSources"`
	Modules         []Block        `json:"modules"`
	Variables       []VariableDecl `json:"variables"`
	Outputs         []OutputDecl   `json:"outputs"`
	Locals          []string       `json:"locals"`
	Imports         []ImportDecl   `json:"imports"`
	Moved           int            `json:"moved,omitempty"`
	// Language features.
	Functions      map[string]int `json:"functions"`
	Ternaries      int            `json:"ternaries"`
	Interpolations int            `json:"interpolations"`
	ForExprs       int            `json:"forExpressions"`
	UsesWorkspace  bool           `json:"usesWorkspace"` // terraform.workspace
	TfvarsFiles    []string       `json:"tfvarsFiles,omitempty"`
	Findings       []Finding      `json:"findings"`
	// ParseErrors are files that aren't valid HCL: validate reports them
	// in full, with the plan.
	ParseErrors []Finding `json:"parseErrors,omitempty"`
}

// Errors are the findings that block a plan.
func (a *Analysis) Errors() []Finding {
	var out []Finding
	for _, f := range a.Findings {
		if f.Severity == SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// AnalyzeOptions is what Analyze knows beyond the code.
type AnalyzeOptions struct {
	// VarKeys are the workspace's Terraform variables that have a value.
	VarKeys map[string]bool
}

// secretName is an attribute or variable named like a credential: one that
// ends in password, token, secret, a key…
var secretName = regexp.MustCompile(`(?i)(^|_)(password|passwd|secret|token|private_key|access_key|secret_key|secret_access_key|api_key|apikey|client_secret|auth_token)$`)

// IsSecretName reports whether a name looks like a credential's.
func IsSecretName(name string) bool { return secretName.MatchString(name) }

type analyzer struct {
	a         *Analysis
	opts      AnalyzeOptions
	providers map[string]*ProviderReq
	// Where each provider is first used.
	usedAt   map[string]hcl.Pos
	usedFile map[string]string
}

func (z *analyzer) find(rule, file string, line int, format string, args ...any) {
	r, _ := RuleByID(rule)
	z.a.Findings = append(z.a.Findings, Finding{Rule: rule, Concept: r.Concept, Severity: r.Severity, File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// Analyze reads a workspace's .tf and .tfvars files.
func Analyze(files []File, opts AnalyzeOptions) *Analysis {
	a := &Analysis{
		Providers: []ProviderReq{}, Resources: []Block{}, DataSources: []Block{}, Modules: []Block{},
		Variables: []VariableDecl{}, Outputs: []OutputDecl{}, Locals: []string{}, Imports: []ImportDecl{},
		Functions: map[string]int{}, Findings: []Finding{},
	}
	z := &analyzer{a: a, opts: opts, providers: map[string]*ProviderReq{}, usedAt: map[string]hcl.Pos{}, usedFile: map[string]string{}}
	tfvarsKeys := map[string]bool{}

	sorted := append([]File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for _, f := range sorted {
		// Only the root module's files: a local module's own variables are
		// set by its caller, not the workspace.
		root := !strings.Contains(f.Path, "/")
		switch {
		case strings.HasSuffix(f.Path, ".tf"):
			body, ok := z.parse(f)
			if ok {
				z.body(f.Path, body, root)
			}
		case root && (strings.HasSuffix(f.Path, ".auto.tfvars") || f.Path == "terraform.tfvars"):
			a.TfvarsFiles = append(a.TfvarsFiles, f.Path)
			if body, ok := z.parse(f); ok {
				for name, attr := range body.Attributes {
					tfvarsKeys[name] = true
					if IsSecretName(name) && literalString(attr.Expr) != "" {
						z.find("TF006", f.Path, attr.SrcRange.Start.Line, "%s sets %s in a file: put it in the workspace's variables, marked sensitive", f.Path, name)
					}
				}
			}
		}
	}

	// Providers: every one used is declared, with a version.
	for name := range z.usedFile {
		if _, ok := z.providers[name]; !ok {
			z.providers[name] = &ProviderReq{Name: name}
		}
	}
	names := make([]string, 0, len(z.providers))
	for n := range z.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := z.providers[n]
		a.Providers = append(a.Providers, *p)
		where, line := z.usedFile[n], z.usedAt[n].Line
		switch {
		case !p.Declared:
			z.find("TF001", where, line, "provider %q isn't in required_providers: add it with a source and a version constraint", n)
		case p.Version == "":
			z.find("TF001", where, line, "provider %q has no version constraint in required_providers (e.g. version = \"~> 5.0\")", n)
		}
	}

	if a.RequiredVersion == "" && len(files) > 0 {
		z.find("TF002", "", 0, "no required_version: add terraform { required_version = \">= 1.6\" }")
	}

	// Variables need a value: a default, a workspace variable or a tfvars.
	for _, v := range a.Variables {
		if !v.HasDefault && !opts.VarKeys[v.Name] && !tfvarsKeys[v.Name] {
			z.find("TF012", v.File, v.Line, "variable %q has no default and no value: set it in the workspace's variables", v.Name)
		}
	}
	sort.SliceStable(a.Findings, func(i, j int) bool {
		return severityRank(a.Findings[i].Severity) < severityRank(a.Findings[j].Severity)
	})
	return a
}

func severityRank(s string) int {
	switch s {
	case SeverityError:
		return 0
	case SeverityWarning:
		return 1
	}
	return 2
}

func (z *analyzer) parse(f File) (*hclsyntax.Body, bool) {
	file, diags := hclsyntax.ParseConfig([]byte(f.Content), f.Path, hcl.InitialPos)
	if diags.HasErrors() {
		d := diags[0]
		line := 0
		if d.Subject != nil {
			line = d.Subject.Start.Line
		}
		z.a.ParseErrors = append(z.a.ParseErrors, Finding{Severity: SeverityError, File: f.Path, Line: line, Message: d.Summary + ": " + d.Detail})
		return nil, false
	}
	body, ok := file.Body.(*hclsyntax.Body)
	return body, ok
}

func (z *analyzer) body(file string, body *hclsyntax.Body, root bool) {
	z.expressions(body)
	for _, b := range body.Blocks {
		line := b.TypeRange.Start.Line
		switch b.Type {
		case "terraform":
			z.terraformBlock(file, b)
		case "provider":
			if len(b.Labels) == 1 {
				p := z.provider(b.Labels[0])
				if alias := literalString(attrExpr(b.Body, "alias")); alias != "" {
					p.Aliases = append(p.Aliases, alias)
				}
				z.use(b.Labels[0], file, b.TypeRange.Start)
				z.secrets(file, b.Body, "provider "+b.Labels[0])
			}
		case "resource", "data":
			if len(b.Labels) != 2 {
				continue
			}
			blk := z.resource(file, b)
			if b.Type == "resource" {
				z.a.Resources = append(z.a.Resources, blk)
			} else {
				z.a.DataSources = append(z.a.DataSources, blk)
				z.find("TF017", file, line, "%s is read on every plan", blk.Address)
			}
		case "module":
			if len(b.Labels) == 1 {
				z.module(file, b)
			}
		case "variable":
			if len(b.Labels) == 1 && root {
				z.variable(file, b)
			}
		case "output":
			if len(b.Labels) == 1 && root {
				z.output(file, b)
			}
		case "locals":
			for name, attr := range b.Body.Attributes {
				z.a.Locals = append(z.a.Locals, name)
				if IsSecretName(name) && literalString(attr.Expr) != "" {
					z.find("TF006", file, attr.SrcRange.Start.Line, "local.%s is a hardcoded secret: use a sensitive variable", name)
				}
			}
			sort.Strings(z.a.Locals)
		case "import":
			to := ""
			if e := attrExpr(b.Body, "to"); e != nil {
				to = exprText(e)
			}
			id := literalString(attrExpr(b.Body, "id"))
			if id == "" && attrExpr(b.Body, "id") != nil {
				id = "(computed)"
			}
			z.a.Imports = append(z.a.Imports, ImportDecl{To: to, ID: id})
		case "moved", "removed":
			z.a.Moved++
		}
	}
}

func (z *analyzer) provider(name string) *ProviderReq {
	p, ok := z.providers[name]
	if !ok {
		p = &ProviderReq{Name: name}
		z.providers[name] = p
	}
	return p
}

// use records that provider name is used, first where.
func (z *analyzer) use(name, file string, pos hcl.Pos) {
	if name == "terraform" {
		return // built in: terraform_data, terraform_remote_state
	}
	if _, ok := z.usedFile[name]; !ok {
		z.usedFile[name], z.usedAt[name] = file, pos
	}
}

func (z *analyzer) terraformBlock(file string, b *hclsyntax.Block) {
	if v := literalString(attrExpr(b.Body, "required_version")); v != "" {
		z.a.RequiredVersion = v
	}
	for _, inner := range b.Body.Blocks {
		switch inner.Type {
		case "required_providers":
			for name, attr := range inner.Body.Attributes {
				p := z.provider(name)
				p.Declared = true
				val, diags := attr.Expr.Value(nil)
				if diags.HasErrors() || !val.IsWhollyKnown() || val.IsNull() {
					continue
				}
				switch {
				case val.Type() == cty.String:
					p.Version = val.AsString() // the old form: name = "version"
				case val.Type().IsObjectType():
					if val.Type().HasAttribute("source") && val.GetAttr("source").Type() == cty.String {
						p.Source = val.GetAttr("source").AsString()
					}
					if val.Type().HasAttribute("version") && val.GetAttr("version").Type() == cty.String {
						p.Version = val.GetAttr("version").AsString()
					}
				}
			}
		case "backend":
			if len(inner.Labels) == 1 {
				z.a.Backend = inner.Labels[0]
				if inner.Labels[0] == "local" {
					z.find("TF011", file, inner.TypeRange.Start.Line, "backend \"local\" would lose the state after each run: remove it (the panel keeps the state) or use a remote backend")
				}
				z.secrets(file, inner.Body, "backend "+inner.Labels[0])
			}
		case "cloud":
			z.a.Backend = "cloud"
		}
	}
}

func (z *analyzer) resource(file string, b *hclsyntax.Block) Block {
	kind, typ, name := b.Type, b.Labels[0], b.Labels[1]
	addr := typ + "." + name
	if kind == "data" {
		addr = "data." + addr
	}
	blk := Block{Kind: kind, Type: typ, Name: name, Address: addr, File: file, Line: b.TypeRange.Start.Line}
	provider := typ
	if i := strings.Index(typ, "_"); i > 0 {
		provider = typ[:i]
	}
	if e := attrExpr(b.Body, "provider"); e != nil {
		blk.Provider = exprText(e)
		provider = strings.SplitN(blk.Provider, ".", 2)[0]
	}
	z.use(provider, file, b.TypeRange.Start)
	z.metaArgs(file, b, &blk)
	z.secrets(file, b.Body, addr)
	return blk
}

func (z *analyzer) metaArgs(file string, b *hclsyntax.Block, blk *Block) {
	_, blk.Count = b.Body.Attributes["count"]
	_, blk.ForEach = b.Body.Attributes["for_each"]
	if blk.Count && blk.ForEach {
		z.find("TF013", file, blk.Line, "%s uses both count and for_each", blk.Address)
	}
	if attr, ok := b.Body.Attributes["depends_on"]; ok {
		if tup, ok := attr.Expr.(*hclsyntax.TupleConsExpr); ok {
			for _, e := range tup.Exprs {
				blk.DependsOn = append(blk.DependsOn, exprText(e))
			}
		}
		z.find("TF015", file, attr.SrcRange.Start.Line, "%s has depends_on %s: needed only if nothing in it references them", blk.Address, strings.Join(blk.DependsOn, ", "))
	}
	for _, inner := range b.Body.Blocks {
		switch inner.Type {
		case "lifecycle":
			lc := &Lifecycle{}
			lc.PreventDestroy = literalBool(attrExpr(inner.Body, "prevent_destroy"))
			lc.CreateBeforeDestroy = literalBool(attrExpr(inner.Body, "create_before_destroy"))
			_, lc.ReplaceTriggeredBy = inner.Body.Attributes["replace_triggered_by"]
			if attr, ok := inner.Body.Attributes["ignore_changes"]; ok {
				lc.IgnoreChanges = true
				if t, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr); ok && t.Traversal.RootName() == "all" {
					lc.IgnoreAll = true
					z.find("TF016", file, attr.SrcRange.Start.Line, "%s ignores all changes", blk.Address)
				}
			}
			blk.Lifecycle = lc
		case "provisioner":
			if len(inner.Labels) == 1 {
				blk.Provisioners = append(blk.Provisioners, inner.Labels[0])
				z.find("TF014", file, inner.TypeRange.Start.Line, "%s runs a %s provisioner", blk.Address, inner.Labels[0])
			}
		}
	}
	blk.Dynamic = dynamicBlocks(b.Body)
}

// dynamicBlocks are the names of a body's dynamic blocks, nested ones too.
func dynamicBlocks(body *hclsyntax.Body) []string {
	var out []string
	for _, inner := range body.Blocks {
		if inner.Type == "dynamic" && len(inner.Labels) == 1 {
			out = append(out, inner.Labels[0])
		}
		out = append(out, dynamicBlocks(inner.Body)...)
	}
	return out
}

// registrySource is a registry module address: [host/]namespace/name/system.
var registrySource = regexp.MustCompile(`^([a-z0-9.-]+\.[a-z]{2,}/)?[A-Za-z0-9_-]+/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+(//.*)?$`)

func (z *analyzer) module(file string, b *hclsyntax.Block) {
	name := b.Labels[0]
	blk := Block{Kind: "module", Name: name, Address: "module." + name, File: file, Line: b.TypeRange.Start.Line}
	blk.Source = literalString(attrExpr(b.Body, "source"))
	blk.Version = literalString(attrExpr(b.Body, "version"))
	z.metaArgs(file, b, &blk)
	src := blk.Source
	local := strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../")
	blk.Remote = src != "" && !local
	if blk.Remote {
		git := strings.HasPrefix(src, "git::") || strings.HasPrefix(src, "git@") || strings.HasPrefix(src, "github.com/") ||
			strings.HasPrefix(src, "gitlab.com/") || strings.HasPrefix(src, "bitbucket.org/") || strings.Contains(src, ".git")
		switch {
		case git && !strings.Contains(src, "ref="):
			z.find("TF009", file, blk.Line, "module %q comes from Git without ?ref=: pin a tag or commit", name)
		case !git && registrySource.MatchString(src) && blk.Version == "":
			z.find("TF009", file, blk.Line, "module %q comes from a registry without version: pin one (e.g. version = \"~> 5.0\")", name)
		}
	}
	for attrName, attr := range b.Body.Attributes {
		if IsSecretName(attrName) && literalString(attr.Expr) != "" {
			z.find("TF006", file, attr.SrcRange.Start.Line, "module %q is given %s as a literal: pass a sensitive variable", name, attrName)
		}
	}
	z.a.Modules = append(z.a.Modules, blk)
}

func (z *analyzer) variable(file string, b *hclsyntax.Block) {
	v := VariableDecl{Name: b.Labels[0], File: file, Line: b.TypeRange.Start.Line}
	if e := attrExpr(b.Body, "type"); e != nil {
		v.Type = exprText(e)
	}
	v.Description = literalString(attrExpr(b.Body, "description"))
	def, hasDefault := b.Body.Attributes["default"]
	v.HasDefault = hasDefault
	v.Sensitive = literalBool(attrExpr(b.Body, "sensitive"))
	for _, inner := range b.Body.Blocks {
		if inner.Type == "validation" {
			v.Validations++
		}
	}
	if v.Type == "" {
		z.find("TF003", file, v.Line, "variable %q has no type (e.g. type = string)", v.Name)
	}
	if v.Description == "" {
		z.find("TF004", file, v.Line, "variable %q has no description", v.Name)
	}
	if IsSecretName(v.Name) {
		if !v.Sensitive {
			z.find("TF005", file, v.Line, "variable %q looks like a secret: add sensitive = true", v.Name)
		}
		if hasDefault && literalString(def.Expr) != "" {
			z.find("TF005", file, def.SrcRange.Start.Line, "variable %q has its secret as a default: remove the default and set it in the workspace's sensitive variables", v.Name)
		}
	}
	z.a.Variables = append(z.a.Variables, v)
}

func (z *analyzer) output(file string, b *hclsyntax.Block) {
	o := OutputDecl{Name: b.Labels[0], File: file, Line: b.TypeRange.Start.Line}
	o.Description = literalString(attrExpr(b.Body, "description"))
	o.Sensitive = literalBool(attrExpr(b.Body, "sensitive"))
	if o.Description == "" {
		z.find("TF007", file, o.Line, "output %q has no description", o.Name)
	}
	if IsSecretName(o.Name) && !o.Sensitive {
		z.find("TF008", file, o.Line, "output %q looks like a secret: add sensitive = true", o.Name)
	}
	z.a.Outputs = append(z.a.Outputs, o)
}

// secrets finds credentials written as literals in a body, nested blocks
// too.
func (z *analyzer) secrets(file string, body *hclsyntax.Body, where string) {
	for name, attr := range body.Attributes {
		if IsSecretName(name) && literalString(attr.Expr) != "" {
			z.find("TF006", file, attr.SrcRange.Start.Line, "%s sets %s to a literal: use a sensitive variable (var.%s)", where, name, name)
		}
	}
	for _, inner := range body.Blocks {
		z.secrets(file, inner.Body, where)
	}
}

// expressions counts the language features a body uses.
func (z *analyzer) expressions(body *hclsyntax.Body) {
	hclsyntax.VisitAll(body, func(n hclsyntax.Node) hcl.Diagnostics {
		switch e := n.(type) {
		case *hclsyntax.FunctionCallExpr:
			z.a.Functions[e.Name]++
		case *hclsyntax.ConditionalExpr:
			z.a.Ternaries++
		case *hclsyntax.ForExpr:
			z.a.ForExprs++
		case *hclsyntax.TemplateExpr:
			for _, p := range e.Parts {
				if _, lit := p.(*hclsyntax.LiteralValueExpr); !lit {
					z.a.Interpolations++
					break
				}
			}
		case *hclsyntax.ScopeTraversalExpr:
			if len(e.Traversal) >= 2 && e.Traversal.RootName() == "terraform" {
				if attr, ok := e.Traversal[1].(hcl.TraverseAttr); ok && attr.Name == "workspace" {
					z.a.UsesWorkspace = true
				}
			}
		}
		return nil
	})
}

func attrExpr(body *hclsyntax.Body, name string) hclsyntax.Expression {
	if a, ok := body.Attributes[name]; ok {
		return a.Expr
	}
	return nil
}

// literalString is an expression's value if it's a constant string, or "".
func literalString(e hclsyntax.Expression) string {
	if e == nil || len(e.Variables()) > 0 {
		return ""
	}
	if hasFunctionCall(e) {
		return "" // file(), base64decode()…: not written out in the code
	}
	v, diags := e.Value(nil)
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

func hasFunctionCall(e hclsyntax.Expression) bool {
	found := false
	hclsyntax.VisitAll(e, func(n hclsyntax.Node) hcl.Diagnostics {
		if _, ok := n.(*hclsyntax.FunctionCallExpr); ok {
			found = true
		}
		return nil
	})
	return found
}

func literalBool(e hclsyntax.Expression) bool {
	if e == nil || len(e.Variables()) > 0 {
		return false
	}
	v, diags := e.Value(nil)
	return !diags.HasErrors() && v.IsWhollyKnown() && !v.IsNull() && v.Type() == cty.Bool && v.True()
}

// exprText is an expression as written: a traversal like aws.west or
// aws_instance.web, or a type like list(string).
func exprText(e hclsyntax.Expression) string {
	switch t := e.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return traversalText(t.Traversal)
	case *hclsyntax.FunctionCallExpr:
		args := make([]string, len(t.Args))
		for i, a := range t.Args {
			args[i] = exprText(a)
		}
		return t.Name + "(" + strings.Join(args, ", ") + ")"
	}
	if s := literalString(e); s != "" {
		return s
	}
	r := e.Range()
	return fmt.Sprintf("(expression at line %d)", r.Start.Line)
}

func traversalText(t hcl.Traversal) string {
	var b strings.Builder
	for i, step := range t {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			b.WriteString(s.Name)
		case hcl.TraverseAttr:
			b.WriteString("." + s.Name)
		case hcl.TraverseIndex:
			if s.Key.Type() == cty.String {
				fmt.Fprintf(&b, "[%q]", s.Key.AsString())
			} else if s.Key.Type() == cty.Number {
				fmt.Fprintf(&b, "[%s]", s.Key.AsBigFloat().String())
			}
		default:
			if i == 0 {
				b.WriteString("?")
			}
		}
	}
	return b.String()
}

// replaceAddr is a resource address that can be planned for replacement
// (-replace): resources, inside modules too — never a data source.
var replaceAddr = regexp.MustCompile(`^(module\.[A-Za-z0-9_-]+(\[[0-9]+\]|\["[^"\]]+"\])?\.)*[A-Za-z][A-Za-z0-9_-]*\.[A-Za-z_][A-Za-z0-9_-]*(\[[0-9]+\]|\["[^"\]]+"\])?$`)

// ValidReplaceAddr reports whether addr can be given to -replace.
func ValidReplaceAddr(addr string) bool {
	if !replaceAddr.MatchString(addr) {
		return false
	}
	// What's left after the module path is type.name: not a data source,
	// nor a module itself.
	rest := addr
	for strings.HasPrefix(rest, "module.") {
		i := strings.Index(rest[len("module."):], ".")
		if i < 0 {
			return false
		}
		next := rest[len("module.")+i+1:]
		if !strings.Contains(next, ".") {
			break // rest is module.<name>.<x>: x alone isn't type.name
		}
		rest = next
	}
	return !strings.HasPrefix(rest, "data.") && !strings.HasPrefix(rest, "module.")
}
