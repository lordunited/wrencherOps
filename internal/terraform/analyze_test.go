// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"sort"
	"strings"
	"testing"
)

// good is code that meets the whole standard and uses most of the
// language: it must have no enforced finding.
var good = []File{
	{Path: "versions.tf", Content: `terraform {
  required_version = ">= 1.6"

  required_providers {
    aws    = { source = "hashicorp/aws", version = "~> 5.0" }
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

provider "aws" {
  region = var.region
}

provider "aws" {
  alias  = "west"
  region = "us-west-2"
}
`},
	{Path: "variables.tf", Content: `variable "region" {
  description = "Where everything runs."
  type        = string
  default     = "eu-central-1"
}

variable "db_password" {
  description = "The database's admin password."
  type        = string
  sensitive   = true
}

variable "servers" {
  description = "Server names to their sizes."
  type        = map(string)
  default     = { web = "t3.small" }

  validation {
    condition     = length(var.servers) > 0
    error_message = "At least one server."
  }
}
`},
	{Path: "main.tf", Content: `locals {
  name = "app-${terraform.workspace}"
  big  = var.region == "eu-central-1" ? "large" : "small"
}

data "aws_ami" "linux" {
  most_recent = true
  owners      = ["amazon"]
}

resource "random_pet" "suffix" {}

resource "aws_instance" "web" {
  for_each      = var.servers
  ami           = data.aws_ami.linux.id
  instance_type = each.value
  tags          = { Name = "${local.name}-${each.key}", Upper = upper(each.key) }

  dynamic "ebs_block_device" {
    for_each = [for d in ["sdb", "sdc"] : d]
    content {
      device_name = "/dev/${ebs_block_device.value}"
    }
  }

  lifecycle {
    prevent_destroy       = true
    create_before_destroy = true
  }
}

resource "aws_db_instance" "db" {
  count    = 1
  password = var.db_password
  provider = aws.west
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.0"
}

module "local" {
  source = "./modules/thing"
}

import {
  to = aws_instance.old
  id = "i-0123456789"
}
`},
	{Path: "outputs.tf", Content: `output "web_ids" {
  description = "Every web server's ID."
  value       = [for i in aws_instance.web : i.id]
}

output "db_password" {
  description = "The password, for the app's config."
  value       = var.db_password
  sensitive   = true
}
`},
	// A local module's variables are set by its caller: no TF012 for them.
	{Path: "modules/thing/main.tf", Content: `variable "needed" {
  description = "Set by the caller."
  type        = string
}
`},
}

func rulesOf(a *Analysis, severity string) []string {
	var out []string
	for _, f := range a.Findings {
		if severity == "" || f.Severity == severity {
			out = append(out, f.Rule)
		}
	}
	sort.Strings(out)
	return out
}

func TestAnalyzeGoodCode(t *testing.T) {
	a := Analyze(good, AnalyzeOptions{VarKeys: map[string]bool{"db_password": true}})
	if len(a.ParseErrors) > 0 {
		t.Fatalf("parse errors: %+v", a.ParseErrors)
	}
	if errs := a.Errors(); len(errs) > 0 {
		t.Fatalf("good code has enforced findings: %+v", errs)
	}
	if a.RequiredVersion != ">= 1.6" {
		t.Errorf("required_version = %q", a.RequiredVersion)
	}
	if len(a.Providers) != 2 || a.Providers[0].Name != "aws" || a.Providers[0].Version != "~> 5.0" || !a.Providers[0].Declared || len(a.Providers[0].Aliases) != 1 {
		t.Errorf("providers = %+v", a.Providers)
	}
	if len(a.Resources) != 3 || len(a.DataSources) != 1 || len(a.Modules) != 2 || len(a.Variables) != 3 || len(a.Outputs) != 2 {
		t.Errorf("structure: %d resources, %d data, %d modules, %d variables, %d outputs", len(a.Resources), len(a.DataSources), len(a.Modules), len(a.Variables), len(a.Outputs))
	}
	var web, db Block
	for _, r := range a.Resources {
		switch r.Address {
		case "aws_instance.web":
			web = r
		case "aws_db_instance.db":
			db = r
		}
	}
	if !web.ForEach || web.Count || web.Lifecycle == nil || !web.Lifecycle.PreventDestroy || !web.Lifecycle.CreateBeforeDestroy || len(web.Dynamic) != 1 {
		t.Errorf("web = %+v", web)
	}
	if !db.Count || db.Provider != "aws.west" {
		t.Errorf("db = %+v", db)
	}
	if !a.Modules[0].Remote || a.Modules[0].Version != "~> 5.0" || a.Modules[1].Remote { // in file order: vpc, local
		t.Errorf("modules = %+v", a.Modules)
	}
	if strings.Join(a.Locals, ",") != "big,name" || len(a.Imports) != 1 || a.Imports[0].To != "aws_instance.old" || a.Imports[0].ID != "i-0123456789" {
		t.Errorf("locals %v, imports %+v", a.Locals, a.Imports)
	}
	if !a.UsesWorkspace || a.Ternaries != 1 || a.ForExprs < 2 || a.Interpolations < 3 || a.Functions["upper"] != 1 || a.Functions["length"] != 1 {
		t.Errorf("features: workspace %v, ternaries %d, for %d, interpolations %d, functions %v", a.UsesWorkspace, a.Ternaries, a.ForExprs, a.Interpolations, a.Functions)
	}
	if got := rulesOf(a, SeverityInfo); strings.Join(got, ",") != "TF017" {
		t.Errorf("info findings = %v", got)
	}
}

func TestAnalyzeRequiredVariableNeedsAValue(t *testing.T) {
	a := Analyze(good, AnalyzeOptions{})
	if got := rulesOf(a, SeverityError); strings.Join(got, ",") != "TF012" {
		t.Errorf("without db_password's value: %v", got)
	}
	withTfvars := append(append([]File{}, good...), File{Path: "prod.auto.tfvars", Content: `db_password_hint = "x"` + "\n"})
	if len(Analyze(withTfvars, AnalyzeOptions{}).Errors()) != 1 {
		t.Error("a tfvars that doesn't set it changes nothing")
	}
}

func TestAnalyzeFindsEveryRule(t *testing.T) {
	cases := []struct {
		name, code string
		want       []string // rules found, sorted
	}{
		{"unpinned provider", `terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = { source = "hashicorp/aws" }
  }
}
resource "aws_s3_bucket" "b" {}
resource "google_storage_bucket" "g" {}`, []string{"TF001", "TF001"}},
		{"no required_version", `resource "terraform_data" "x" {}`, []string{"TF002"}},
		{"variable without type or description", `terraform { required_version = ">= 1.6" }
variable "x" { default = 1 }`, []string{"TF003", "TF004"}},
		{"secret variable", `terraform { required_version = ">= 1.6" }
variable "api_token" {
  type = string
  description = "d"
  default = "abc123"
}`, []string{"TF005", "TF005"}},
		{"hardcoded secrets", `terraform { required_version = ">= 1.6" }
resource "terraform_data" "x" {
  input = { password = "hunter2" }
}
resource "terraform_data" "y" {
  connection {
    password = "hunter2"
  }
  provisioner "local-exec" { command = "true" }
}
locals { secret = "s3cr3t" }
module "m" {
  source = "./m"
  api_key = "k"
}`, []string{"TF006", "TF006", "TF006", "TF014"}},
		{"not secrets", `terraform { required_version = ">= 1.6" }
resource "terraform_data" "x" {
  input = {
    password_length = 16
    secret_name     = "my-app"
    password        = var.pw
    token           = file("token.txt")
    access_key      = ""
  }
}`, nil},
		{"outputs", `terraform { required_version = ">= 1.6" }
output "a" { value = 1 }
output "admin_password" {
  description = "d"
  value = "x"
}`, []string{"TF007", "TF008"}},
		{"unpinned modules", `terraform { required_version = ">= 1.6" }
module "reg" { source = "terraform-aws-modules/vpc/aws" }
module "git" { source = "git::https://example.com/m.git" }
module "gitok" { source = "git::https://example.com/m.git?ref=v1.2.0" }
module "local" { source = "../shared" }`, []string{"TF009", "TF009"}},
		{"local backend", `terraform {
  required_version = ">= 1.6"
  backend "local" {}
}`, []string{"TF011"}},
		{"count and for_each", `terraform { required_version = ">= 1.6" }
resource "terraform_data" "x" {
  count = 2
  for_each = toset(["a"])
}`, []string{"TF013"}},
		{"depends_on and ignore all", `terraform { required_version = ">= 1.6" }
resource "terraform_data" "a" {}
resource "terraform_data" "b" {
  depends_on = [terraform_data.a]
  lifecycle { ignore_changes = all }
}`, []string{"TF015", "TF016"}},
		{"tfvars secret", `terraform { required_version = ">= 1.6" }`, []string{"TF006"}},
	}
	for _, c := range cases {
		files := []File{{Path: "main.tf", Content: c.code + "\n"}}
		if c.name == "tfvars secret" {
			files = append(files, File{Path: "terraform.tfvars", Content: "db_password = \"x\"\n"})
		}
		a := Analyze(files, AnalyzeOptions{})
		if len(a.ParseErrors) > 0 {
			t.Errorf("%s: parse errors %+v", c.name, a.ParseErrors)
			continue
		}
		var got []string
		for _, r := range rulesOf(a, "") {
			if r != "TF017" {
				got = append(got, r)
			}
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: found %v, want %v\n%+v", c.name, got, c.want, a.Findings)
		}
	}
}

func TestAnalyzeParseError(t *testing.T) {
	a := Analyze([]File{{Path: "main.tf", Content: "resource \"x\" {\n"}}, AnalyzeOptions{})
	if len(a.ParseErrors) != 1 || a.ParseErrors[0].File != "main.tf" {
		t.Errorf("parse errors = %+v", a.ParseErrors)
	}
}

func TestEnforcedFindingsComeFirst(t *testing.T) {
	a := Analyze([]File{{Path: "main.tf", Content: `resource "terraform_data" "b" {
  depends_on = [terraform_data.a]
}
variable "x" {}
`}}, AnalyzeOptions{})
	seenAdvice := false
	for _, f := range a.Findings {
		if f.Severity != SeverityError {
			seenAdvice = true
		} else if seenAdvice {
			t.Fatalf("an enforced finding after advice: %+v", a.Findings)
		}
	}
}

func TestSecretNames(t *testing.T) {
	for _, n := range []string{"password", "db_password", "api_token", "token", "secret", "client_secret", "aws_secret_access_key", "private_key", "API_KEY"} {
		if !IsSecretName(n) {
			t.Errorf("%s should look like a secret", n)
		}
	}
	for _, n := range []string{"password_length", "secret_name", "token_ttl", "keyname", "public_key", "tokens_enabled"} {
		if IsSecretName(n) {
			t.Errorf("%s shouldn't look like a secret", n)
		}
	}
}

func TestReplaceAddr(t *testing.T) {
	for _, ok := range []string{"aws_instance.web", `aws_instance.web["a"]`, "aws_instance.web[0]", "module.app.aws_instance.web", `module.app["x"].module.db.aws_db_instance.main[2]`} {
		if !ValidReplaceAddr(ok) {
			t.Errorf("%s should be replaceable", ok)
		}
	}
	for _, bad := range []string{"", "-destroy", "data.aws_ami.x", "module.app.data.aws_ami.x", "aws_instance", "aws_instance.web; rm -rf /", "aws_instance.web[x]", "module.app", "../x.y"} {
		if ValidReplaceAddr(bad) {
			t.Errorf("%q shouldn't be replaceable", bad)
		}
	}
}

func TestRulesAndConcepts(t *testing.T) {
	names := ConceptNames()
	if len(names) != 30 {
		t.Fatalf("%d concepts, want 30", len(names))
	}
	concept := map[string]bool{}
	for _, n := range names {
		if concept[n] {
			t.Errorf("concept %s twice", n)
		}
		concept[n] = true
	}
	ids := map[string]bool{}
	enforced := 0
	for _, r := range Rules {
		if ids[r.ID] {
			t.Errorf("rule %s twice", r.ID)
		}
		ids[r.ID] = true
		if !concept[r.Concept] {
			t.Errorf("rule %s is for concept %q, which isn't one of the 30", r.ID, r.Concept)
		}
		if r.Severity == SeverityError {
			enforced++
		}
		if r.Title == "" || r.Why == "" {
			t.Errorf("rule %s isn't explained", r.ID)
		}
	}
	if enforced < 10 {
		t.Errorf("only %d enforced rules", enforced)
	}
}

func TestConcepts(t *testing.T) {
	ws := &Workspace{Name: "w", StateSerial: 4}
	a := Analyze(good, AnalyzeOptions{})
	cs := Concepts(a, ws, []Variable{{Key: "region", Kind: "terraform"}, {Key: "AWS_PROFILE", Kind: "env"}})
	if len(cs) != 30 {
		t.Fatalf("%d concepts", len(cs))
	}
	by := map[string]Concept{}
	for _, c := range cs {
		by[c.Name] = c
	}
	checks := map[string]string{
		"Providers":           "ok",
		"Workspace Variables": "error", // db_password has no value
		"Sentinel":            "error",
		"Provisioners":        "unused",
		"Data Sources":        "ok",
	}
	for name, want := range checks {
		if by[name].Status != want {
			t.Errorf("%s: status %s, want %s (%+v)", name, by[name].Status, want, by[name])
		}
	}
	if !by["Providers"].Enforced || by["Provisioners"].Enforced || !by["Sentinel"].Enforced {
		t.Error("enforced flags are wrong")
	}
	if !strings.Contains(by["Remote Modules"].Here, "1 remote module") || !strings.Contains(by["Lifecycle"].Here, "1 protected") ||
		!strings.Contains(by["State File"].Here, "serial 4") || by["Workspace Variables"].Here != "1 variable, 1 environment variable" {
		t.Errorf("here: %q / %q / %q / %q", by["Remote Modules"].Here, by["Lifecycle"].Here, by["State File"].Here, by["Workspace Variables"].Here)
	}
}

// Every template the panel offers meets the standard. The one rule a
// template may break is TF012, and only for its root variables: a login
// (an address, a region) has no sensible default, so the workspace asks
// for it on the Variables tab before its first plan.
func TestTemplatesMeetTheStandard(t *testing.T) {
	for _, tp := range Templates {
		if err := ValidateFiles(tp.Files); err != nil {
			t.Errorf("template %s: %v", tp.Key, err)
		}
		a := Analyze(tp.Files, AnalyzeOptions{})
		if len(a.ParseErrors) > 0 {
			t.Errorf("template %s: %+v", tp.Key, a.ParseErrors)
		}
		for _, f := range a.Errors() {
			if f.Rule == "TF012" && f.File == "variables.tf" {
				continue
			}
			t.Errorf("template %s: %+v", tp.Key, f)
		}
	}
}

// Every module template folder is listed, and every listed one has a
// root module that calls its own module.
func TestModuleTemplatesAreComplete(t *testing.T) {
	entries, err := moduleTemplateFS.ReadDir("moduletemplates")
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, m := range moduleTemplates {
		listed[m.Key] = true
	}
	for _, e := range entries {
		if !listed[e.Name()] {
			t.Errorf("moduletemplates/%s isn't listed in moduleTemplates", e.Name())
		}
	}
	keys := map[string]bool{}
	for _, tp := range Templates {
		if keys[tp.Key] {
			t.Errorf("template key %s is used twice", tp.Key)
		}
		keys[tp.Key] = true
		if !listed[tp.Key] {
			continue
		}
		have := map[string]string{}
		for _, f := range tp.Files {
			have[f.Path] = f.Content
		}
		for _, p := range []string{"versions.tf", "providers.tf", "variables.tf", "main.tf", "outputs.tf"} {
			if _, ok := have[p]; !ok {
				t.Errorf("template %s has no %s", tp.Key, p)
			}
		}
		if !strings.Contains(have["main.tf"], `source   = "./modules/`) {
			t.Errorf("template %s: main.tf doesn't call a module under modules/", tp.Key)
		}
	}
}

func TestBlockedMessage(t *testing.T) {
	a := Analyze([]File{{Path: "main.tf", Content: "variable \"x\" {}\n"}}, AnalyzeOptions{})
	msg := blockedMessage(a.Errors())
	for _, want := range []string{"TF003", "TF004", "main.tf:1", "enforced rules"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q lacks %q", msg, want)
		}
	}
}

func TestSummarizeImports(t *testing.T) {
	s := summarize([]byte(`{"resource_changes":[
		{"address":"aws_instance.old","change":{"actions":["no-op"],"importing":{"id":"i-1"}}},
		{"address":"aws_instance.web","change":{"actions":["create"]}},
		{"address":"aws_instance.x","change":{"actions":["update"],"importing":{"id":"i-2"}}}]}`))
	if s.Import != 2 || s.Add != 1 || s.Change != 1 || len(s.Changes) != 4 {
		t.Errorf("summary = %+v", s)
	}
}
