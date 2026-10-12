// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"strings"
	"testing"
)

func TestSigNozComposeTemplate(t *testing.T) {
	a, err := SigNozComposeTemplate()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := SigNozComposeTemplate()
	if a.ComposeFile != "compose.yaml" || !strings.Contains(a.Manifest, "signoz/signoz:${SIGNOZ_VERSION:-") {
		t.Fatalf("manifest = %q", a.Manifest[:200])
	}
	if strings.Contains(a.Manifest, ":latest") || strings.Contains(a.Manifest, "POSTGRES_PASSWORD=signoz") {
		t.Error("template still has a :latest image or the fixed Postgres password")
	}
	if a.Env == b.Env || !strings.Contains(a.Env, "SIGNOZ_POSTGRES_PASSWORD=") {
		t.Errorf("each template should get its own password:\n%s", a.Env)
	}
	if err := ValidateProjectFiles(a.Files); err != nil {
		t.Fatal(err)
	}
	if len(a.Files) != 5 {
		t.Errorf("files = %d, want the 5 config files", len(a.Files))
	}

	// Every file the compose mounts is in the template, and nothing is missing.
	bundled := SigNozBundledFiles()
	if len(bundled) != 2 {
		t.Fatalf("bundled = %d files, want histogram-quantile for amd64 and arm64", len(bundled))
	}
	if strings.Contains(a.Manifest, "github.com") || strings.Contains(a.Manifest, "wget -O") {
		t.Error("the template still downloads from GitHub on the server")
	}
	an := AnalyzeProject(a.Manifest, a.Env, append(a.Files, bundled...), a.DeployDir)
	if an.ParseError != "" {
		t.Fatal(an.ParseError)
	}
	for _, w := range an.Warnings {
		t.Errorf("warning: %s", w)
	}
}
