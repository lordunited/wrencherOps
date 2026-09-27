package promctl

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMoveConfigToDisk(t *testing.T) {
	old, err := os.ReadFile("testdata/old-prometheus-stack.yml")
	if err != nil {
		t.Fatal(err)
	}
	out, err := MoveConfigToDisk(string(old), "/opt/monitoring")
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Services map[string]struct {
			Configs []map[string]string `yaml:"configs"`
			Volumes []string            `yaml:"volumes"`
			Command []string            `yaml:"command"`
		} `yaml:"services"`
		Configs map[string]any `yaml:"configs"`
	}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid YAML: %v\n%s", err, out)
	}
	prom := parsed.Services["prometheus"]
	if len(prom.Configs) != 0 {
		t.Errorf("prometheus still has inline configs: %v", prom.Configs)
	}
	if prom.Volumes[len(prom.Volumes)-1] != "/opt/monitoring/prometheus-config:/etc/prometheus" {
		t.Errorf("volumes = %v", prom.Volumes)
	}
	if prom.Command[0] != "--config.file=/etc/prometheus/prometheus.yml" {
		t.Errorf("command changed: %v", prom.Command)
	}
	if _, ok := parsed.Configs["prometheus_config"]; ok {
		t.Error("unused prometheus_config definition kept")
	}
	if _, ok := parsed.Configs["grafana_datasource"]; !ok {
		t.Error("grafana_datasource, still used by Grafana, was removed")
	}
	if len(parsed.Services["grafana"].Configs) != 1 {
		t.Error("grafana's config reference was touched")
	}

	// Running it again changes nothing further.
	again, err := MoveConfigToDisk(out, "/opt/monitoring")
	if err != nil || again != out {
		t.Errorf("second run changed the file (err=%v)", err)
	}
}

func TestMoveConfigToDiskRejectsNonPrometheus(t *testing.T) {
	if _, err := MoveConfigToDisk("services:\n  web:\n    image: nginx\n", "/opt/x"); err == nil ||
		!strings.Contains(err.Error(), "no Prometheus") {
		t.Errorf("err = %v", err)
	}
}
