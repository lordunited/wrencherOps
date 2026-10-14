// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// A Snippet is a working example of one of the 30 concepts, added to a
// workspace's code from its Standards tab. Every one meets the standard and
// needs no cloud account (random, null, tls and terraform_data), so it can be
// planned and applied right away. "{n}" in a snippet's names is replaced so
// they don't clash with what the code has: "" the first time, then "_2", ….
type Snippet struct {
	Concept string `json:"concept"`
	Title   string `json:"title"`
	// File is where Code goes, at the workspace's root.
	File string `json:"file"`
	Code string `json:"code"`
	// Files are more files the snippet adds (a local module's).
	Files map[string]string `json:"files,omitempty"`
	// Providers are added to required_providers when the code lacks them.
	Providers []ProviderReq `json:"providers,omitempty"`
}

// ProviderChoice is a provider the Providers concept can add, pinned.
type ProviderChoice struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Label   string `json:"label"`
}

// ProviderChoices are the providers the panel offers to add, pinned to the
// latest stable release on the OpenTofu registry on 2026-10-12.
var ProviderChoices = []ProviderChoice{
	{"esxi", "josenk/esxi", "~> 1.10", "VMware ESXi (standalone host)"},
	{"vsphere", "hashicorp/vsphere", "~> 2.17", "VMware vCenter / vSphere"},
	{"virtualbox", "terra-farm/virtualbox", "0.2.2-alpha.1", "VirtualBox"},
	{"proxmox", "bpg/proxmox", "~> 0.116.0", "Proxmox VE"},
	{"libvirt", "dmacvicar/libvirt", "~> 0.9.9", "libvirt (KVM / QEMU)"},
	{"vault", "hashicorp/vault", "~> 5.12", "HashiCorp Vault"},
	{"aws", "hashicorp/aws", "~> 6.68", "AWS"},
	{"azurerm", "hashicorp/azurerm", "~> 5.9", "Azure"},
	{"google", "hashicorp/google", "~> 8.6", "Google Cloud"},
	{"cloudflare", "cloudflare/cloudflare", "~> 5.27", "Cloudflare"},
	{"kubernetes", "hashicorp/kubernetes", "~> 3.3", "Kubernetes"},
	{"helm", "hashicorp/helm", "~> 3.3", "Helm"},
	{"docker", "kreuzwerker/docker", "~> 4.6", "Docker"},
	{"gitlab", "gitlabhq/gitlab", "~> 19.4", "GitLab"},
	{"random", "hashicorp/random", "~> 3.9", "random (no account)"},
	{"tls", "hashicorp/tls", "~> 4.4", "tls (no account)"},
	{"null", "hashicorp/null", "~> 3.3", "null (no account)"},
	{"local", "hashicorp/local", "~> 2.9", "local files (no account)"},
}

var (
	random = ProviderReq{Name: "random", Source: "hashicorp/random", Version: "~> 3.9"}
	tlsP   = ProviderReq{Name: "tls", Source: "hashicorp/tls", Version: "~> 4.4"}
)

// Snippets are the concepts' examples, by concept name.
var Snippets = []Snippet{
	{Concept: "Resources", Title: "Add a resource", File: "main.tf", Providers: []ProviderReq{random}, Code: `# Resources: something OpenTofu creates and manages.
resource "random_pet" "example{n}" {
  length = 2
}
`},
	{Concept: "Variables", Title: "Add a variable", File: "variables.tf", Code: `# Variables: inputs, with a type, a description, a default and a check.
variable "environment{n}" {
  type        = string
  description = "Where this runs: dev, staging or prod."
  default     = "dev"

  validation {
    condition     = contains(["dev", "staging", "prod"], var.environment{n})
    error_message = "environment must be dev, staging or prod."
  }
}
`},
	{Concept: "Outputs", Title: "Add an output", File: "outputs.tf", Code: `# Outputs: values shown after an apply (and readable by other code).
output "workspace_name{n}" {
  description = "The workspace this state belongs to."
  value       = terraform.workspace
}
`},
	{Concept: "Data Sources", Title: "Add a data source", File: "main.tf", Providers: []ProviderReq{tlsP}, Code: `# Data sources: read something instead of creating it. Here, the public
# half of a key this code makes.
resource "tls_private_key" "example{n}" {
  algorithm = "ED25519"
}

data "tls_public_key" "example{n}" {
  private_key_pem = tls_private_key.example{n}.private_key_pem
}

output "ssh_public_key{n}" {
  description = "The key's public half, for authorized_keys."
  value       = data.tls_public_key.example{n}.public_key_openssh
}
`},
	{Concept: "Local Values", Title: "Add local values", File: "main.tf", Code: `# Local values: names for expressions used more than once.
locals {
  app_name{n} = "app-${terraform.workspace}"
  common_tags{n} = {
    app       = local.app_name{n}
    managedBy = "opentofu"
  }
}
`},
	{Concept: "Backends", Title: "Add a backend example (commented out)", File: "backend.tf", Code: `# Backends: where the state is kept. The panel keeps it for you,
# encrypted and versioned, so none is needed. Uncommenting this moves the
# state to S3 (credentials as AWS_* environment variables); the panel
# then no longer keeps it.
#
# terraform {
#   backend "s3" {
#     bucket       = "my-tf-state"
#     key          = "workspaces/app/terraform.tfstate"
#     region       = "eu-central-1"
#     use_lockfile = true
#   }
# }
`},
	{Concept: "Modules", Title: "Add a local module", File: "main.tf", Providers: []ProviderReq{random},
		Files: map[string]string{
			"modules/name{n}/versions.tf": `terraform {
  required_version = ">= 1.6"

  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.9"
    }
  }
}
`,
			"modules/name{n}/variables.tf": `variable "prefix" {
  type        = string
  description = "What the name starts with."
}
`,
			"modules/name{n}/main.tf": `resource "random_pet" "this" {
  prefix = var.prefix
  length = 2
}
`,
			"modules/name{n}/outputs.tf": `output "name" {
  description = "The generated name."
  value       = random_pet.this.id
}
`,
		},
		Code: `# Modules: reusable code. This one lives in modules/name{n}.
module "name{n}" {
  source = "./modules/name{n}"
  prefix = terraform.workspace
}

output "module_name{n}" {
  description = "The name the module made."
  value       = module.name{n}.name
}
`},
	{Concept: "Remote Modules", Title: "Add a registry module", File: "main.tf", Code: `# Remote modules: from a registry (or Git), always pinned to a version.
# cloudposse/label builds consistent names and tags; no account needed.
module "label{n}" {
  source  = "cloudposse/label/null"
  version = "~> 0.25"

  namespace = "acme"
  stage     = terraform.workspace
  name      = "app"
}

output "label_id{n}" {
  description = "The name the label module built, e.g. acme-dev-app."
  value       = module.label{n}.id
}
`},
	{Concept: "Provisioners", Title: "Add a provisioner (last resort)", File: "main.tf", Code: `# Provisioners: run a command when a resource is created. A last resort:
# prefer cloud-init, images or Ansible. This one runs on the panel.
resource "terraform_data" "announce{n}" {
  input = terraform.workspace

  provisioner "local-exec" {
    command = "echo Created in workspace ${self.input}"
  }
}
`},
	{Concept: "Count", Title: "Add a resource with count", File: "main.tf", Providers: []ProviderReq{random}, Code: `# count: several copies of one resource, numbered from 0.
resource "random_id" "server{n}" {
  count       = 3
  byte_length = 4
}

output "server_ids{n}" {
  description = "One ID per copy."
  value       = random_id.server{n}[*].hex
}
`},
	{Concept: "For_each", Title: "Add a resource with for_each", File: "main.tf", Providers: []ProviderReq{random}, Code: `# for_each: one copy per key of a set or map, addressed by the key.
resource "random_pet" "team{n}" {
  for_each = toset(["api", "web", "worker"])
  prefix   = each.key
}

output "team_names{n}" {
  description = "One name per key."
  value       = { for k, p in random_pet.team{n} : k => p.id }
}
`},
	{Concept: "Depends_on", Title: "Add depends_on", File: "main.tf", Code: `# depends_on: order two resources that don't reference each other. Only
# when needed: a reference already orders them.
resource "terraform_data" "database{n}" {
  input = "database"
}

resource "terraform_data" "app{n}" {
  input      = "app"
  depends_on = [terraform_data.database{n}]
}
`},
	{Concept: "Lifecycle", Title: "Add a lifecycle block", File: "main.tf", Providers: []ProviderReq{random}, Code: `# lifecycle: change how OpenTofu replaces or updates a resource.
resource "random_string" "suffix{n}" {
  length  = 8
  special = false

  lifecycle {
    create_before_destroy = true
    ignore_changes        = [special]
    # prevent_destroy = true # refuses any plan that would destroy it
  }
}
`},
	{Concept: "Workspaces", Title: "Use terraform.workspace", File: "main.tf", Code: `# Workspaces: in the panel each workspace has its own code, variables and
# state, and terraform.workspace is its name.
locals {
  is_production{n} = terraform.workspace == "production"
}

output "is_production{n}" {
  description = "Whether this workspace is named production."
  value       = local.is_production{n}
}
`},
	{Concept: "Functions", Title: "Add function calls", File: "main.tf", Code: `# Functions: built in, for strings, lists, maps, numbers and more.
locals {
  servers{n} = ["web-1", "web-2", "db-1"]
  sizes{n}   = { small = 1, large = 4 }
}

output "functions{n}" {
  description = "A few built-in functions at work."
  value = {
    count  = length(local.servers{n})
    joined = join(", ", local.servers{n})
    first  = element(local.servers{n}, 0)
    upper  = upper(terraform.workspace)
    size   = lookup(local.sizes{n}, "large", 1)
    web    = [for s in local.servers{n} : s if startswith(s, "web")]
  }
}
`},
	{Concept: "Interpolation", Title: "Add interpolation", File: "outputs.tf", Code: `# Interpolation: ${...} puts a value inside a string.
output "greeting{n}" {
  description = "A string built from values."
  value       = "Hello from ${terraform.workspace}, made by OpenTofu ${terraform.workspace == "default" ? "" : "in its own workspace"}"
}
`},
	{Concept: "Dynamic Blocks", Title: "Add a dynamic block", File: "main.tf", Providers: []ProviderReq{tlsP}, Code: `# Dynamic blocks: write a nested block once, repeat it per item.
locals {
  dns_names{n} = ["app.example.com", "api.example.com"]
}

resource "tls_private_key" "cert{n}" {
  algorithm = "ECDSA"
}

resource "tls_cert_request" "cert{n}" {
  private_key_pem = tls_private_key.cert{n}.private_key_pem
  dns_names       = local.dns_names{n}

  dynamic "subject" {
    for_each = { main = local.dns_names{n}[0] }
    content {
      common_name  = subject.value
      organization = "Acme"
    }
  }
}
`},
	{Concept: "Ternary", Title: "Add a conditional", File: "main.tf", Code: `# Ternary: condition ? if_true : if_false.
locals {
  instance_size{n} = terraform.workspace == "production" ? "large" : "small"
}

output "instance_size{n}" {
  description = "large in production, small elsewhere."
  value       = local.instance_size{n}
}
`},
}

// SnippetFor is concept's example.
func SnippetFor(concept string) (Snippet, bool) {
	for _, s := range Snippets {
		if s.Concept == concept {
			return s, true
		}
	}
	return Snippet{}, false
}

// ErrNoSnippet is a concept with no example to add.
var ErrNoSnippet = errors.New("this concept has no example to add")

// AddSnippet adds s to files: Code at the end of s.File, s.Files, and the
// providers it needs in required_providers. It returns the new files and
// where the snippet starts, to show it.
func AddSnippet(files []File, s Snippet) (out []File, file string, line int, err error) {
	taken, err := declared(files)
	if err != nil {
		return nil, "", 0, err
	}
	suffix := ""
	for n := 2; clashes(s, suffix, taken, files); n++ {
		if n > 50 {
			return nil, "", 0, errors.New("the code already has this example many times over")
		}
		suffix = fmt.Sprintf("_%d", n)
	}
	sub := func(c string) string { return strings.ReplaceAll(c, "{n}", suffix) }

	out = append([]File(nil), files...)
	put := func(p, content string) {
		for i := range out {
			if out[i].Path == p {
				out[i].Content = content
				return
			}
		}
		out = append(out, File{Path: p, Content: content})
	}
	get := func(p string) (string, bool) {
		for _, f := range out {
			if f.Path == p {
				return f.Content, true
			}
		}
		return "", false
	}

	for _, p := range sortedKeys(s.Files) {
		put(sub(p), sub(s.Files[p]))
	}
	existing, _ := get(s.File)
	if existing != "" {
		existing = strings.TrimRight(existing, "\n") + "\n\n"
	}
	line = strings.Count(existing, "\n") + 1
	put(s.File, existing+sub(s.Code))

	if err := addProviders(&out, s.Providers); err != nil {
		return nil, "", 0, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, s.File, line, nil
}

// declared is every top-level name the root module declares: blocks as
// type.label…, locals as local.name.
func declared(files []File) (map[string]bool, error) {
	taken := map[string]bool{}
	for _, f := range files {
		if strings.Contains(f.Path, "/") || !strings.HasSuffix(f.Path, ".tf") {
			continue
		}
		body, err := parseBody(f.Path, f.Content)
		if err != nil {
			return nil, fmt.Errorf("%s doesn't parse, so nothing was added: %w", f.Path, err)
		}
		addDeclared(taken, body)
	}
	return taken, nil
}

func addDeclared(taken map[string]bool, body *hclsyntax.Body) {
	for _, b := range body.Blocks {
		if b.Type == "locals" {
			for name := range b.Body.Attributes {
				taken["local."+name] = true
			}
			continue
		}
		taken[strings.Join(append([]string{b.Type}, b.Labels...), ".")] = true
	}
}

func parseBody(filename, content string) (*hclsyntax.Body, error) {
	f, diags := hclsyntax.ParseConfig([]byte(content), filename, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, diags
	}
	return f.Body.(*hclsyntax.Body), nil
}

// clashes reports whether s, named with suffix, declares a name the code
// has or would add a file that exists.
func clashes(s Snippet, suffix string, taken map[string]bool, files []File) bool {
	body, err := parseBody(s.File, strings.ReplaceAll(s.Code, "{n}", suffix))
	if err != nil {
		return false
	}
	mine := map[string]bool{}
	addDeclared(mine, body)
	for k := range mine {
		if k != "terraform" && taken[k] {
			return true
		}
	}
	for p := range s.Files {
		p = strings.ReplaceAll(p, "{n}", suffix)
		for _, f := range files {
			if f.Path == p {
				return true
			}
		}
	}
	return false
}

// addProviders puts each provider the code lacks into required_providers:
// in the root file whose terraform block has it, else in versions.tf.
func addProviders(files *[]File, need []ProviderReq) error {
	if len(need) == 0 {
		return nil
	}
	var target = -1
	have := map[string]bool{}
	for i, f := range *files {
		if strings.Contains(f.Path, "/") || !strings.HasSuffix(f.Path, ".tf") {
			continue
		}
		wf, diags := hclwrite.ParseConfig([]byte(f.Content), f.Path, hcl.InitialPos)
		if diags.HasErrors() {
			return fmt.Errorf("%s doesn't parse: %s", f.Path, diags.Error())
		}
		for _, b := range wf.Body().Blocks() {
			if b.Type() != "terraform" {
				continue
			}
			if rp := b.Body().FirstMatchingBlock("required_providers", nil); rp != nil {
				for name := range rp.Body().Attributes() {
					have[name] = true
				}
				if target < 0 {
					target = i
				}
			} else if target < 0 && path.Base(f.Path) == "versions.tf" {
				target = i
			}
		}
	}
	var missing []ProviderReq
	for _, p := range need {
		if !have[p.Name] {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if target < 0 {
		*files = append(*files, File{Path: "versions.tf", Content: "terraform {\n  required_version = \">= 1.6\"\n}\n"})
		target = len(*files) - 1
	}
	f := &(*files)[target]
	wf, _ := hclwrite.ParseConfig([]byte(f.Content), f.Path, hcl.InitialPos)
	tf := wf.Body().FirstMatchingBlock("terraform", nil)
	if tf == nil {
		tf = wf.Body().AppendNewBlock("terraform", nil)
	}
	rp := tf.Body().FirstMatchingBlock("required_providers", nil)
	if rp == nil {
		tf.Body().AppendNewline()
		rp = tf.Body().AppendNewBlock("required_providers", nil)
	}
	for _, p := range missing {
		rp.Body().SetAttributeRaw(p.Name, hclwrite.TokensForObject([]hclwrite.ObjectAttrTokens{
			{Name: hclwrite.TokensForIdentifier("source"), Value: hclwrite.TokensForValue(cty.StringVal(p.Source))},
			{Name: hclwrite.TokensForIdentifier("version"), Value: hclwrite.TokensForValue(cty.StringVal(p.Version))},
		}))
	}
	f.Content = string(hclwrite.Format(wf.Bytes()))
	return nil
}

// AddProvider adds one of ProviderChoices to required_providers.
func AddProvider(files []File, name string) ([]File, string, error) {
	for _, c := range ProviderChoices {
		if c.Name != name {
			continue
		}
		out := append([]File(nil), files...)
		if err := addProviders(&out, []ProviderReq{{Name: c.Name, Source: c.Source, Version: c.Version}}); err != nil {
			return nil, "", err
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		for _, f := range out {
			if strings.Contains(f.Content, "required_providers") && !strings.Contains(f.Path, "/") {
				return out, f.Path, nil
			}
		}
		return out, "versions.tf", nil
	}
	return nil, "", fmt.Errorf("unknown provider %q", name)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
