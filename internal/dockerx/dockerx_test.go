// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package dockerx

import (
	"strings"
	"testing"
)

func TestHostPath(t *testing.T) {
	ct := &Container{Mounts: []Mount{
		{Source: "/srv/app", Destination: "/etc/app"},
		{Source: "/srv/app-conf.d/", Destination: "/etc/app/conf.d/"},
		{Source: "/srv/one.conf", Destination: "/etc/one.conf"},
	}}
	cases := map[string]string{
		"/etc/app":               "/srv/app",
		"/etc/app/main.conf":     "/srv/app/main.conf",
		"/etc/app/conf.d/x.conf": "/srv/app-conf.d/x.conf", // the longest mount wins
		"/etc/one.conf":          "/srv/one.conf",
	}
	for in, want := range cases {
		if got, ok := ct.HostPath(in); !ok || got != want {
			t.Errorf("HostPath(%s) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"/etc/application.conf", "/etc/one.conf.bak", "/var/x"} {
		if got, ok := ct.HostPath(in); ok {
			t.Errorf("HostPath(%s) = %q; want no match", in, got)
		}
	}
}

func TestCompose(t *testing.T) {
	ct := &Container{Labels: map[string]string{
		"com.docker.compose.project":              "shop",
		"com.docker.compose.project.working_dir":  "/opt/shop",
		"com.docker.compose.project.config_files": "/opt/shop/compose.yml,/opt/shop/compose.prod.yml",
	}}
	p, dir, files := ct.Compose()
	if p != "shop" || dir != "/opt/shop" || len(files) != 2 || files[1] != "/opt/shop/compose.prod.yml" {
		t.Errorf("Compose() = %q %q %v", p, dir, files)
	}
	if p, dir, files := (&Container{}).Compose(); p != "" || dir != "" || files != nil {
		t.Errorf("not compose: %q %q %v", p, dir, files)
	}
}

func TestImageBase(t *testing.T) {
	cases := map[string]string{
		"nginx":                                 "nginx",
		"nginx:1.27-alpine":                     "nginx",
		"docker.io/library/nginx:1.27-alpine":   "nginx",
		"registry.example.com:5000/team/app:v2": "app",
		"prom/prometheus@sha256:abc":            "prometheus",
		"envoyproxy/envoy:v1.39-latest":         "envoy",
	}
	for in, want := range cases {
		if got := ImageBase(in); got != want {
			t.Errorf("ImageBase(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSHA(t *testing.T) {
	if got := SHA(""); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("SHA(\"\") = %s", got)
	}
	if SHA("a") == SHA("b") || len(SHA("x")) != 64 {
		t.Error("SHA isn't a sha256 hex digest")
	}
}

func TestExecQuotesEverything(t *testing.T) {
	cmd := Exec("web-1", true, `cat "$1"`, "/etc/x; rm -rf /", "it's")
	for _, want := range []string{"docker exec -u 0 'web-1' sh -c ", `'cat "$1"'`, `'/etc/x; rm -rf /'`, `'it'\''s'`} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q lacks %q", cmd, want)
		}
	}
	if strings.Contains(Exec("web-1", false, "true"), "-u 0") {
		t.Error("a non-root exec runs as root")
	}
}

func TestBackupPath(t *testing.T) {
	cases := map[string]string{
		"/etc/nginx/nginx.conf":   BackupRoot + "/etc/nginx/nginx.conf.20260101",
		"etc/x.conf":              BackupRoot + "/etc/x.conf.20260101",
		"/etc/../../root/.ssh/id": BackupRoot + "/root/.ssh/id.20260101", // can't climb out of the backup root
	}
	for in, want := range cases {
		if got := BackupPath(in, "20260101"); got != want {
			t.Errorf("BackupPath(%s) = %s, want %s", in, got, want)
		}
	}
}
