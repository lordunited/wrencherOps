// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"strings"
	"testing"
)

// Import keeps the project's own compose file name, so any name docker
// compose finds by itself is fine; what can't be imported is a project the
// panel couldn't redeploy as the same project.
func TestJudgeImport(t *testing.T) {
	for _, c := range []struct {
		label   string
		p       DetectedProject
		ok      bool
		problem string
	}{
		{"docker-compose.yaml", DetectedProject{Name: "vector-ai", WorkingDir: "/home/a/vector-ai", ConfigFiles: []string{"/home/a/vector-ai/docker-compose.yaml"}, Status: "running(2)"}, true, ""},
		{"compose.yaml on disk", DetectedProject{Name: "app", WorkingDir: "/opt/app", ConfigFiles: []string{"/opt/app/compose.yaml"}}, true, ""},
		{"docker-compose.yml", DetectedProject{Name: "shop", WorkingDir: "/opt/shop", ConfigFiles: []string{"/opt/shop/docker-compose.yml"}, Status: "exited(1)"}, true, ""},
		{"custom -f name", DetectedProject{Name: "x", WorkingDir: "/opt/x", ConfigFiles: []string{"/opt/x/prod.yml"}, Status: "running(1)"}, false, "-f"},
		{"several -f files", DetectedProject{Name: "x", WorkingDir: "/opt/x", ConfigFiles: []string{"/opt/x/a.yml", "/opt/x/b.yml"}, Status: "running(1)"}, false, "several compose files"},
		{"two compose files on disk", DetectedProject{Name: "x", WorkingDir: "/opt/x", ConfigFiles: []string{"/opt/x/compose.yaml", "/opt/x/docker-compose.yml"}}, false, "more than one compose file"},
		{"stray second file", DetectedProject{Name: "x", WorkingDir: "/opt/x", ConfigFiles: []string{"/opt/x/compose.yaml"}, Status: "running(1)", otherComposeFiles: []string{"/opt/x/docker-compose.yml"}}, false, "more than one compose file"},
		{"-p name", DetectedProject{Name: "custom", WorkingDir: "/opt/x", ConfigFiles: []string{"/opt/x/compose.yaml"}, Status: "running(1)"}, false, `"custom"`},
	} {
		p := c.p
		judge(&p)
		if p.Importable != c.ok || !strings.Contains(p.ImportProblem, c.problem) {
			t.Errorf("%s: importable=%v problem=%q", c.label, p.Importable, p.ImportProblem)
		}
	}
}
