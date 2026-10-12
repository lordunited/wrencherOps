// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

import (
	"testing"
)

func TestEvaluate(t *testing.T) {
	snap := &Snapshot{
		CPUs: 2, Load5: 5,
		Memory: Memory{TotalBytes: 100, AvailableBytes: 3, UsedPercent: 97},
		Disks: []Disk{
			{Mount: "/", UsedPercent: 96, SizeBytes: 100 << 30, AvailBytes: 4 << 30, InodesUsedPercent: 10},
			{Mount: "/data", UsedPercent: 88, SizeBytes: 100 << 30, AvailBytes: 12 << 30, InodesUsedPercent: -1},
			{Mount: "/backup", UsedPercent: 40, InodesUsedPercent: 99},
		},
		Failed:       FailedLogins{LastHour: 31, TopSources: []Count{{"45.1.1.1", 30}}, TopUsers: []Count{{"root", 25}}},
		Connections:  Connections{Readable: true, Established: 1500, TopPeers: []Count{{"8.8.8.8", 900}}},
		KernelErrors: KernelErrors{Readable: true, LastHour: 2, Recent: []KernelLine{{Message: "I/O error, dev sdb"}}},
	}
	rules := append([]Rule{}, DefaultRules...)
	rules = append(rules, Rule{ID: "only-other", Kind: "connections", Threshold: 1, Severity: "critical", Enabled: true, ServerID: "other"})

	findings, judged := Evaluate(rules, "srv", snap, "")
	got := map[string]Finding{}
	for _, f := range findings {
		got[f.RuleID+" "+f.Subject] = f
	}
	// / is past both 85 and 95: only the stricter (critical) one.
	if f, ok := got["builtin-disk-95 /"]; !ok || f.Severity != "critical" {
		t.Errorf("/ at 96%%: %+v", findings)
	}
	if _, ok := got["builtin-disk-85 /"]; ok {
		t.Error("/ got both the 85% and the 95% alert")
	}
	if _, ok := got["builtin-disk-85 /data"]; !ok {
		t.Error("/data at 88% should warn")
	}
	if _, ok := got["builtin-inodes-95 /backup"]; !ok {
		t.Error("/backup inodes at 99% should alert")
	}
	for _, id := range []string{"builtin-failed-logins ", "builtin-connections ", "builtin-kernel-errors ", "builtin-load ", "builtin-memory "} {
		if _, ok := got[id]; !ok {
			t.Errorf("missing %q in %+v", id, findings)
		}
	}
	if _, ok := got["only-other "]; ok {
		t.Error("a rule for another server fired here")
	}
	if _, ok := got["builtin-swap "]; ok {
		t.Error("a disabled rule fired")
	}
	if len(findings) == 0 || findings[0].Severity != "critical" {
		t.Error("critical findings should come first")
	}
	if got["builtin-failed-logins "].Message != `31 failed SSH logins in the last hour — most from 45.1.1.1 (30), trying user "root"` {
		t.Errorf("message: %q", got["builtin-failed-logins "].Message)
	}
	if len(judged) != len(DefaultRules) {
		t.Errorf("judged %d rules, want %d (every rule for this server, enabled or not)", len(judged), len(DefaultRules))
	}

	// Unreachable: only the unreachable rule is judged, so other alerts stay.
	findings, judged = Evaluate(rules, "srv", nil, "SSH connection failed: timeout")
	if len(findings) != 1 || findings[0].RuleID != "builtin-unreachable" || len(judged) != 1 {
		t.Errorf("unreachable: %+v judged %v", findings, judged)
	}
}
