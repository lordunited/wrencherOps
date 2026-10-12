// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"strings"
	"testing"
)

func TestValidProjectPath(t *testing.T) {
	for _, ok := range []string{".env.prod", "config/nginx.conf", "init/01-schema.sql", "a/b/c.yml"} {
		if err := ValidProjectPath(ok); err != nil {
			t.Errorf("ValidProjectPath(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../../x", "a//b", "a/", "docker-compose.yml", PanelFilesList, "a b.txt", "./x"} {
		if ValidProjectPath(bad) == nil {
			t.Errorf("ValidProjectPath(%q) accepted", bad)
		}
	}
}

func TestValidateProjectFilesRejectsBinaryAndDuplicates(t *testing.T) {
	if err := ValidateProjectFiles([]ProjectFile{{Path: "a.bin", Content: "x\x00y"}}); err == nil {
		t.Error("binary content accepted")
	}
	if err := ValidateProjectFiles([]ProjectFile{{Path: "a"}, {Path: "a"}}); err == nil {
		t.Error("duplicate path accepted")
	}
	if err := ValidateProjectFiles([]ProjectFile{{Path: "big", Content: strings.Repeat("x", MaxProjectFileBytes+1)}}); err == nil {
		t.Error("oversized file accepted")
	}
}

func TestInterpolate(t *testing.T) {
	vars := map[string]string{"TAG": "1.27", "EMPTY": ""}
	got, missing := interpolate("${TAG} $TAG ${EMPTY:-d1} ${EMPTY-d2} ${NOPE:-d3} ${TAG:+alt} ${NOPE} $$HOME ${NOPE}", vars)
	if want := "1.27 1.27 d1  d3 alt  $HOME "; got != want {
		t.Errorf("interpolate = %q, want %q", got, want)
	}
	if len(missing) != 1 || missing[0] != "NOPE" {
		t.Errorf("missing = %v, want [NOPE] once", missing)
	}
}

func TestAnalyzeProject(t *testing.T) {
	compose := `
services:
  web:
    image: nginx:${TAG}
    env_file: [.env, app.env]
    volumes:
      - ./conf/default.conf:/etc/nginx/conf.d/default.conf:ro
      - ./site:/usr/share/nginx/html
      - ./missing.conf:/etc/app/app.conf
      - data:/var/lib/web
      - /srv/shared:/shared
      - /cache
  db:
    image: postgres:16
    volumes:
      - type: volume
        source: pgdata
        target: /var/lib/postgresql/data
      - undeclared:/x
volumes:
  data:
  pgdata:
    name: my-pgdata
`
	files := []ProjectFile{{Path: "conf/default.conf", Content: "server {}"}, {Path: "site/index.html", Content: "hi"}}
	a := AnalyzeProject(compose, "TAG=1.27\nCOMPOSE_PROJECT_NAME=shop_app\n", files, "/opt/shop")
	if a.ParseError != "" {
		t.Fatal(a.ParseError)
	}
	if a.Project != "shop_app" {
		t.Errorf("project = %q, want shop_app (from .env)", a.Project)
	}

	by := map[string]Mount{}
	for _, m := range a.Mounts {
		by[m.Service+" "+m.Target] = m
	}
	check := func(key string, want Mount) {
		t.Helper()
		got, ok := by[key]
		if !ok {
			t.Errorf("no mount %s", key)
			return
		}
		want.Service, want.Target = got.Service, got.Target
		if got != want {
			t.Errorf("%s = %+v, want %+v", key, got, want)
		}
	}
	check("web /etc/nginx/conf.d/default.conf", Mount{Kind: "bind", Source: "./conf/default.conf", HostPath: "/opt/shop/conf/default.conf", ReadOnly: true, InProject: true})
	check("web /usr/share/nginx/html", Mount{Kind: "bind", Source: "./site", HostPath: "/opt/shop/site", InProject: true})
	check("web /etc/app/app.conf", Mount{Kind: "bind", Source: "./missing.conf", HostPath: "/opt/shop/missing.conf"})
	check("web /var/lib/web", Mount{Kind: "volume", Source: "data", Volume: "shop_app_data"})
	check("web /shared", Mount{Kind: "bind", Source: "/srv/shared", HostPath: "/srv/shared"})
	check("web /cache", Mount{Kind: "anonymous"})
	check("db /var/lib/postgresql/data", Mount{Kind: "volume", Source: "pgdata", Volume: "my-pgdata"})

	warned := strings.Join(a.Warnings, "\n")
	for _, want := range []string{"no file missing.conf", "/srv/shared, outside the deploy folder", `"undeclared"`, "env_file app.env"} {
		if !strings.Contains(warned, want) {
			t.Errorf("no warning mentioning %q in:\n%s", want, warned)
		}
	}
	if strings.Contains(warned, "env_file .env") || strings.Contains(warned, "${TAG}") {
		t.Errorf("warned about something that's fine:\n%s", warned)
	}
}

// Verified against real compose: a folder name is cleaned up, but an
// explicit invalid name makes compose refuse to start.
func TestAnalyzeProjectNames(t *testing.T) {
	if a := AnalyzeProject("services: {}\n", "", nil, "/opt/My App"); a.Project != "myapp" || len(a.Warnings) != 0 {
		t.Errorf("folder name: project = %q, warnings = %v; want myapp, none", a.Project, a.Warnings)
	}
	if a := AnalyzeProject("name: fromfile\nservices: {}\n", "COMPOSE_PROJECT_NAME=shop_app\n", nil, "/opt/x"); a.Project != "shop_app" {
		t.Errorf(".env must win over name:, got %q", a.Project)
	}
	a := AnalyzeProject("services: {}\n", "COMPOSE_PROJECT_NAME=Shop.App\n", nil, "/opt/x")
	if a.Project != "Shop.App" || len(a.Warnings) != 1 || !strings.Contains(a.Warnings[0], "refuse to start") {
		t.Errorf("invalid explicit name: project = %q, warnings = %v", a.Project, a.Warnings)
	}
}

func TestAnalyzeProjectBadYAML(t *testing.T) {
	if a := AnalyzeProject("services: [unclosed", "", nil, "/opt/x"); a.ParseError == "" {
		t.Error("broken YAML not reported")
	}
}
