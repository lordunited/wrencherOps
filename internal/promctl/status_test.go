// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package promctl

import "testing"

const sampleMetrics = `# HELP prometheus_tsdb_head_series Total number of series in the head block.
# TYPE prometheus_tsdb_head_series gauge
prometheus_tsdb_head_series 18231
prometheus_tsdb_compactions_failed_total 2
prometheus_rule_evaluation_failures_total{rule_group="/etc/prometheus/rules/a.yml;a"} 3
prometheus_rule_evaluation_failures_total{rule_group="/etc/prometheus/rules/b.yml;b"} 4
prometheus_notifications_alertmanagers_discovered 0
process_open_fds 900
process_max_fds 1024
prometheus_tsdb_wal_corruptions_total 0
go_goroutines NaN
unrelated_metric 5
`

func TestParseMetrics(t *testing.T) {
	m := parseMetrics(sampleMetrics, selfMetrics)
	if m["prometheus_tsdb_head_series"] != 18231 {
		t.Errorf("head series = %v", m["prometheus_tsdb_head_series"])
	}
	if m["prometheus_rule_evaluation_failures_total"] != 7 {
		t.Errorf("labelled samples not summed: %v", m["prometheus_rule_evaluation_failures_total"])
	}
	if _, ok := m["unrelated_metric"]; ok {
		t.Error("kept a metric that wasn't asked for")
	}
	if _, ok := m["go_goroutines"]; ok {
		t.Error("kept a NaN")
	}
}

func TestFindings(t *testing.T) {
	st := &ServerStatus{Metrics: parseMetrics(sampleMetrics, selfMetrics), Disk: Disk{FSSizeBytes: 100, FSFreeBytes: 3}}
	got := findings(st, false, 1, 10, 20)
	titles := map[string]string{}
	for _, f := range got {
		titles[f.Title] = f.Severity
	}
	want := map[string]string{
		"Last config reload failed":    "critical",
		"Storage corruption detected":  "critical",
		"Disk almost full":             "critical",
		"Compactions have failed":      "warning",
		"Still replaying the WAL":      "warning",
		"Rule evaluations are failing": "warning",
		"No Alertmanager connected":    "warning",
		"Running out of file handles":  "warning",
	}
	for title, sev := range want {
		if titles[title] != sev {
			t.Errorf("%q: got %q, want %q", title, titles[title], sev)
		}
	}
	if got[0].Severity != "critical" || got[len(got)-1].Severity != "warning" {
		t.Error("critical findings should come first")
	}

	healthy := &ServerStatus{Metrics: map[string]float64{"prometheus_notifications_alertmanagers_discovered": 1}}
	if f := findings(healthy, true, 0, 5, 5); len(f) != 0 {
		t.Errorf("a healthy server has findings: %+v", f)
	}
}
