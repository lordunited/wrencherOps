package promctl

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Shaped like a real hand-maintained config: inline comments on targets,
// commented-out targets, mixed indentation, and blackbox jobs.
const sampleConfig = `global:
  scrape_interval: 15s # Set the scrape interval to every 15 seconds.
  evaluation_interval: 15s

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093

rule_files:
  # - "first_rules.yml"
   - "prometheus-rules.yml"

scrape_configs:
  - job_name: "prometheus"
    static_configs:
      - targets: ["localhost:9090"]
  - job_name: "server_exporter"
    static_configs:
      - targets:
        - 192.0.2.34:9100 #vpn node exporter
        - 192.0.2.34:8080 #vpn cadvisor
          #        - 192.0.2.21:9100 #old node exporter
        - 192.0.2.22:9100 #backup node exporter
  - job_name: 'blackbox-http'
    metrics_path: /probe
    params:
      module: [http_2xx]
    static_configs:
      - targets:
          - https://example.com
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 192.0.2.41:9115
`

type parsedConfig struct {
	RuleFiles     []string `yaml:"rule_files"`
	ScrapeConfigs []struct {
		JobName     string              `yaml:"job_name"`
		MetricsPath string              `yaml:"metrics_path"`
		Params      map[string][]string `yaml:"params"`
		Static      []struct {
			Targets []string `yaml:"targets"`
		} `yaml:"static_configs"`
		Relabel []struct {
			SourceLabels []string `yaml:"source_labels"`
			TargetLabel  string   `yaml:"target_label"`
			Replacement  string   `yaml:"replacement"`
		} `yaml:"relabel_configs"`
	} `yaml:"scrape_configs"`
}

func parse(t *testing.T, s string) parsedConfig {
	t.Helper()
	var p parsedConfig
	if err := yaml.Unmarshal([]byte(s), &p); err != nil {
		t.Fatalf("result is not valid YAML: %v\n%s", err, s)
	}
	return p
}

func TestConfigJobs(t *testing.T) {
	jobs := ConfigJobs(sampleConfig)
	want := []ConfiguredJob{
		{Name: "prometheus", Kind: "exporter"},
		{Name: "server_exporter", Kind: "exporter"},
		{Name: "blackbox-http", Kind: "blackbox", Module: "http_2xx", Exporter: "192.0.2.41:9115"},
	}
	if len(jobs) != len(want) {
		t.Fatalf("got %v", jobs)
	}
	for i := range want {
		if jobs[i] != want[i] {
			t.Errorf("job %d = %v, want %v", i, jobs[i], want[i])
		}
	}
}

func TestAddTargetsKeepsComments(t *testing.T) {
	out, skipped, err := AddTargets(sampleConfig, "server_exporter", []TargetSpec{
		{Address: "192.0.2.50:9100", Comment: "new web node exporter"},
		{Address: "192.0.2.34:9100"}, // already there
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || skipped[0] != "192.0.2.34:9100" {
		t.Errorf("skipped = %v", skipped)
	}
	for _, keep := range []string{"#vpn node exporter", "#backup node exporter", "192.0.2.21:9100 #old node exporter", "# Set the scrape interval", "# new web node exporter"} {
		if !strings.Contains(out, keep) {
			t.Errorf("output lost %q:\n%s", keep, out)
		}
	}
	p := parse(t, out)
	got := p.ScrapeConfigs[1].Static[0].Targets
	if got[len(got)-1] != "192.0.2.50:9100" || len(got) != 4 {
		t.Errorf("targets = %v", got)
	}
}

func TestAddTargetsRejectsUnknownJobAndAllDuplicates(t *testing.T) {
	if _, _, err := AddTargets(sampleConfig, "nope", []TargetSpec{{Address: "a:1"}}); err == nil {
		t.Error("unknown job accepted")
	}
	if _, _, err := AddTargets(sampleConfig, "server_exporter", []TargetSpec{{Address: "192.0.2.34:9100"}}); err == nil {
		t.Error("all-duplicate add accepted")
	}
	if _, _, err := AddTargets(sampleConfig, "server_exporter", []TargetSpec{{Address: "bad address"}}); err == nil {
		t.Error("address with a space accepted")
	}
}

func TestAddBlackboxJob(t *testing.T) {
	out, err := AddJob(sampleConfig, JobSpec{
		Name:         "blackbox-telnet",
		Kind:         "blackbox-tcp",
		ExporterAddr: "192.0.2.41:9115",
		Targets:      []TargetSpec{{Address: "192.0.2.218:443", Comment: "HP SERVER 1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := parse(t, out)
	job := p.ScrapeConfigs[len(p.ScrapeConfigs)-1]
	if job.JobName != "blackbox-telnet" || job.MetricsPath != "/probe" || job.Params["module"][0] != "tcp_connect" {
		t.Errorf("job = %+v", job)
	}
	if len(job.Relabel) != 3 || job.Relabel[0].TargetLabel != "__param_target" || job.Relabel[2].Replacement != "192.0.2.41:9115" {
		t.Errorf("relabel = %+v", job.Relabel)
	}
	if !strings.Contains(out, "# HP SERVER 1") || !strings.Contains(out, "#vpn node exporter") {
		t.Errorf("comments missing:\n%s", out)
	}
}

func TestAddJobValidation(t *testing.T) {
	cases := []JobSpec{
		{Name: "server_exporter", Targets: []TargetSpec{{Address: "a:1"}}},          // duplicate
		{Name: "has space", Targets: []TargetSpec{{Address: "a:1"}}},                // bad name
		{Name: "x", Kind: "blackbox-http", Targets: []TargetSpec{{Address: "a:1"}}}, // no exporter
		{Name: "x", Targets: nil}, // no targets
		{Name: "x", ScrapeInterval: "soon", Targets: []TargetSpec{{Address: "a:1"}}}, // bad interval
		{Name: "x", Kind: "carrier-pigeon", Targets: []TargetSpec{{Address: "a:1"}}}, // bad kind
	}
	for _, c := range cases {
		if _, err := AddJob(sampleConfig, c); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
}

func TestAddRuleToEmptyFileAndExistingGroup(t *testing.T) {
	out, err := AddRule("", "servers", RuleSpec{
		Alert: "InstanceDown", Expr: "up == 0", For: "5m",
		Labels:      map[string]string{"severity": "critical"},
		Annotations: map[string]string{"summary": "{{ $labels.instance }} is down"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = AddRule(out, "servers", RuleSpec{Alert: "DiskAlmostFull", Expr: "x > 0.9"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Groups []struct {
			Name  string `yaml:"name"`
			Rules []struct {
				Alert  string            `yaml:"alert"`
				For    string            `yaml:"for"`
				Labels map[string]string `yaml:"labels"`
			} `yaml:"rules"`
		} `yaml:"groups"`
	}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Groups) != 1 || len(parsed.Groups[0].Rules) != 2 || parsed.Groups[0].Rules[0].Labels["severity"] != "critical" {
		t.Errorf("parsed = %+v", parsed)
	}
	if _, err := AddRule(out, "servers", RuleSpec{Alert: "InstanceDown", Expr: "up == 0"}); err == nil {
		t.Error("duplicate alert accepted")
	}
	if _, err := AddRule(out, "servers", RuleSpec{Alert: "X", Expr: "up", For: "later"}); err == nil {
		t.Error("bad duration accepted")
	}
}

func TestEnsureRuleFile(t *testing.T) {
	out, changed, err := EnsureRuleFile(sampleConfig, "panel-rules.yml")
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	p := parse(t, out)
	if len(p.RuleFiles) != 2 || p.RuleFiles[1] != "panel-rules.yml" {
		t.Errorf("rule_files = %v", p.RuleFiles)
	}
	if _, changed, _ := EnsureRuleFile(out, "panel-rules.yml"); changed {
		t.Error("added the same entry twice")
	}
}

func TestHostPathAndFlags(t *testing.T) {
	inst := &Instance{Mounts: []Mount{
		{Source: "/opt/mon/prometheus-config", Destination: "/etc/prometheus"},
		{Source: "/opt/mon/special.yml", Destination: "/etc/prometheus/rules/special.yml"},
	}}
	cases := map[string]string{
		"/etc/prometheus/prometheus.yml":    "/opt/mon/prometheus-config/prometheus.yml",
		"/etc/prometheus/rules/special.yml": "/opt/mon/special.yml",
		"/etc/prometheus/rules/a.yml":       "/opt/mon/prometheus-config/rules/a.yml",
	}
	for in, want := range cases {
		if got, ok := inst.HostPath(in); !ok || got != want {
			t.Errorf("HostPath(%s) = %s, %v; want %s", in, got, ok, want)
		}
	}
	if _, ok := inst.HostPath("/etc/prometheus2/x.yml"); ok {
		t.Error("matched a sibling folder with a shared prefix")
	}

	args := []string{"--config.file=/cfg/p.yml", "--web.listen-address", "0.0.0.0:9091"}
	if v := flagValue(args, "config.file", ""); v != "/cfg/p.yml" {
		t.Errorf("config.file = %s", v)
	}
	if p := listenPort(flagValue(args, "web.listen-address", ":9090")); p != "9091" {
		t.Errorf("port = %s", p)
	}
	if !isPrometheusImage("prom/prometheus:v3.5.0") || isPrometheusImage("prom/alertmanager:latest") {
		t.Error("isPrometheusImage")
	}
}

func TestRuleFilePatterns(t *testing.T) {
	got := ruleFilePatterns(sampleConfig, "/etc/prometheus")
	if len(got) != 1 || got[0] != "/etc/prometheus/prometheus-rules.yml" {
		t.Errorf("patterns = %v", got)
	}
}

func TestQueryParamAndPromtoolError(t *testing.T) {
	q, ok := queryParam("/api/v1/query?query=up%20%3D%3D&time=1")
	if !ok || q != "up ==" {
		t.Errorf("queryParam = %q, %v", q, ok)
	}
	if _, ok := queryParam("/api/v1/labels?match[]=up"); ok {
		t.Error("labels request treated as a query")
	}
	got := cleanPromtoolError("query error: bad_data: 1:5: parse error: unexpected end of input\nmore")
	if got != "1:5: parse error: unexpected end of input" {
		t.Errorf("cleanPromtoolError = %q", got)
	}
}

func TestRoutePrefix(t *testing.T) {
	cases := map[string][]string{
		"":            {"--config.file=/x"},
		"/prom":       {"--web.route-prefix=/prom/"},
		"/monitoring": {"--web.external-url=https://ops.example.com/monitoring"},
		"/a":          {"--web.external-url=https://x/b", "--web.route-prefix=a"},
	}
	for want, args := range cases {
		if got := routePrefix(args); got != want {
			t.Errorf("routePrefix(%v) = %q, want %q", args, got, want)
		}
	}
}

func TestTargetNotes(t *testing.T) {
	notes := TargetNotes(sampleConfig)
	if notes["server_exporter"]["192.0.2.34:9100"] != "vpn node exporter" ||
		notes["server_exporter"]["192.0.2.22:9100"] != "backup node exporter" {
		t.Errorf("notes = %v", notes)
	}
	if _, ok := notes["server_exporter"]["192.0.2.21:9100"]; ok {
		t.Error("a commented-out target was treated as live")
	}
}

func TestBackupPathIsOutsideRuleFolders(t *testing.T) {
	got := BackupPath("/opt/mon/prometheus-config/rules/hosts.yml", "20260927-081512")
	if got != "/var/backups/devops-panel/opt/mon/prometheus-config/rules/hosts.yml.20260927-081512" {
		t.Errorf("BackupPath = %s", got)
	}
	if got := BackupPath("rel/../x.yml", "s"); got != "/var/backups/devops-panel/x.yml.s" {
		t.Errorf("relative path not cleaned: %s", got)
	}
}
