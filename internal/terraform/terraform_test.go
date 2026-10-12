// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"strings"
	"testing"
)

func TestValidateFiles(t *testing.T) {
	if err := ValidateFiles([]File{{Path: "main.tf", Content: "x"}, {Path: "modules/vpc/main.tf", Content: "y"}, {Path: LockFile, Content: "z"}}); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]File{
		"no root .tf":   {{Path: "modules/vpc/main.tf", Content: "y"}},
		"state":         {{Path: "main.tf", Content: "x"}, {Path: "terraform.tfstate", Content: "{}"}},
		"providers dir": {{Path: "main.tf", Content: "x"}, {Path: ".terraform/providers/x", Content: "x"}},
		"escapes":       {{Path: "main.tf", Content: "x"}, {Path: "../evil.tf", Content: "x"}},
		"binary":        {{Path: "main.tf", Content: "a\x00b"}},
	} {
		if ValidateFiles(bad) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestContentSHAIgnoresLockFile(t *testing.T) {
	a := []File{{Path: "main.tf", Content: "x"}}
	b := []File{{Path: "main.tf", Content: "x"}, {Path: LockFile, Content: "provider ..."}}
	if ContentSHA(a) != ContentSHA(b) {
		t.Error("the lock file init writes shouldn't make a plan stale")
	}
	if ContentSHA(a) == ContentSHA([]File{{Path: "main.tf", Content: "y"}}) {
		t.Error("a code change must change the hash")
	}
}

func TestOwnBackend(t *testing.T) {
	for content, want := range map[string]string{
		"terraform {\n  backend \"s3\" {\n    bucket = \"x\"\n  }\n}\n":           "s3",
		"terraform {\n  required_version = \">= 1\"\n  backend \"pg\" {}\n}\n":    "pg",
		"terraform {\n  # backend \"s3\" {}\n}\n":                                 "",
		"terraform {\n  backend \"local\" {}\n}\n":                                "",
		"resource \"x\" \"y\" {}\n":                                               "",
		"/*\nterraform {\n  backend \"gcs\" {}\n}\n*/\nresource \"a\" \"b\" {}\n": "",
	} {
		if got := OwnBackend([]File{{Path: "main.tf", Content: content}}); got != want {
			t.Errorf("OwnBackend(%q) = %q, want %q", content, got, want)
		}
	}
	if OwnBackend([]File{{Path: "modules/x/main.tf", Content: "terraform {\n backend \"s3\" {}\n}"}}) != "" {
		t.Error("only the root module's backend counts")
	}
}

// The shape of a real "tofu show -json" of a plan (OpenTofu 1.9).
const planJSONSample = `{"format_version":"1.2","resource_changes":[
 {"address":"null_resource.hello","change":{"actions":["create"]}},
 {"address":"random_pet.name","change":{"actions":["delete","create"]}},
 {"address":"aws_instance.web","change":{"actions":["update"]}},
 {"address":"aws_s3_bucket.old","change":{"actions":["delete"]}},
 {"address":"data.aws_ami.ubuntu","change":{"actions":["read"]}},
 {"address":"aws_vpc.main","change":{"actions":["no-op"]}}]}`

func TestSummarize(t *testing.T) {
	s := summarize([]byte(planJSONSample))
	if s.Add != 1 || s.Replace != 1 || s.Change != 1 || s.Destroy != 1 || len(s.Changes) != 5 {
		t.Errorf("%+v", s)
	}
	if s.Changes[1] != (Change{Address: "random_pet.name", Action: "replace"}) {
		t.Errorf("%+v", s.Changes[1])
	}
}

func TestCleanUpload(t *testing.T) {
	files, state, notes := cleanUpload([]File{
		{Path: "main.tf", Content: "x"},
		{Path: "terraform.tfstate", Content: `{"serial":3}`},
		{Path: "terraform.tfstate.backup", Content: "{}"},
		{Path: LockFile, Content: "locks"},
		{Path: ".terraform/providers/a", Content: "bin"},
		{Path: ".git/HEAD", Content: "ref"},
		{Path: "prod.tfvars", Content: "a = 1"},
	})
	if len(files) != 2 || files[0].Path != "main.tf" || files[1].Path != "prod.tfvars" {
		t.Errorf("files: %+v", files)
	}
	if state == nil || state.Content != `{"serial":3}` {
		t.Errorf("state to import: %+v", state)
	}
	all := strings.Join(notes, "\n")
	if !strings.Contains(all, "tfstate.backup: made by a run") || !strings.Contains(all, ".terraform.lock.hcl: left out") || strings.Contains(all, ".git") {
		t.Errorf("notes:\n%s", all)
	}
}

func TestLogMasksSecrets(t *testing.T) {
	l := &logBuffer{secret: []string{"AKIAEXAMPLE123", "x"}}
	l.Write([]byte("key=AKIAEXAMPLE123 x stays\n"))
	if got := l.String(); got != "key=******** x stays\n" {
		t.Errorf("%q (a 1-character value isn't masked, or it would mask everything)", got)
	}
}
