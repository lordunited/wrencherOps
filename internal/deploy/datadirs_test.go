// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"devops-wrapper/internal/sshexec"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDataDirsFor(t *testing.T) {
	cases := []struct {
		serviceType, deployDir string
		want                   []DataDir
	}{
		{"nexus3", "/opt/nexus3", []DataDir{{"/opt/nexus3/nexus-data", "200:200"}}},
		{"prometheus", "/opt/prometheus-stack/", []DataDir{
			{"/opt/prometheus-stack/prometheus-data", "65534:65534"},
			{"/opt/prometheus-stack/grafana-data", "472:0"},
			{"/opt/prometheus-stack/alertmanager-data", "65534:65534"},
		}},
		{"gitlab", "/opt/gitlab", nil},
		{"docker", "/opt/app", nil},
		{"kubernetes", "/opt/app", nil},
	}
	for _, c := range cases {
		got := DataDirsFor(c.serviceType, c.deployDir)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("DataDirsFor(%q, %q) = %v, want %v", c.serviceType, c.deployDir, got, c.want)
		}
	}
}

// Every generated bind mount that DataDirsFor names must really appear in
// the compose file, or the prepared folder isn't the one the container uses.
func TestDataDirsMatchComposeMounts(t *testing.T) {
	nexus := Nexus3Config{HTTPPort: 8081, ImageTag: "latest", DeployDir: "/opt/nexus3", ContainerName: "nexus3"}.ComposeYAML()
	for _, d := range DataDirsFor("nexus3", "/opt/nexus3") {
		if !strings.Contains(nexus, "'"+d.Path+":") {
			t.Errorf("nexus3 compose has no mount for %s", d.Path)
		}
	}
	prom := PrometheusStackConfig{ImageTag: "latest", DeployDir: "/opt/mon", ContainerPrefix: "m", RetentionTime: "15d"}.ComposeYAML()
	for _, d := range DataDirsFor("prometheus", "/opt/mon") {
		if !strings.Contains(prom, "'"+d.Path+":") {
			t.Errorf("prometheus compose has no mount for %s", d.Path)
		}
	}
}

func TestPrepareDirsCmd(t *testing.T) {
	got := prepareDirsCmd([]DataDir{{"/opt/it's", "200:200"}})
	want := sshexec.AsRootPrelude + "true" +
		` && $AS_ROOT mkdir -p '/opt/it'\''s' && { [ "$(stat -c %u:%g '/opt/it'\''s')" = '200:200' ] || $AS_ROOT chown -R '200:200' '/opt/it'\''s'; }`
	if got != want {
		t.Errorf("prepareDirsCmd:\n got %s\nwant %s", got, want)
	}
}

func TestSeedFilesForPrometheus(t *testing.T) {
	seeds := SeedFilesFor("prometheus", "/opt/mon/")
	if len(seeds) != 2 || seeds[0].Path != "/opt/mon/prometheus-config/prometheus.yml" ||
		seeds[1].Path != "/opt/mon/prometheus-config/rules/availability.yml" {
		t.Fatalf("seeds = %+v", seeds)
	}
	for _, s := range seeds {
		var v map[string]any
		if err := yaml.Unmarshal([]byte(s.Content), &v); err != nil {
			t.Errorf("%s is not valid YAML: %v", s.Path, err)
		}
	}
	if !strings.Contains(seeds[0].Content, "rules/*.yml") {
		t.Error("default config doesn't load the rules folder")
	}
	if SeedFilesFor("nexus3", "/opt/x") != nil {
		t.Error("nexus3 got seed files")
	}
}
