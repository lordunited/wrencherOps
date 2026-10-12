// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Rule raises an alert on a server when what it measures reaches
// Threshold. Built-in rules can be tuned or switched off, not deleted.
type Rule struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Threshold float64   `json:"threshold"`
	Severity  string    `json:"severity"` // "warning" | "critical"
	Enabled   bool      `json:"enabled"`
	ServerID  string    `json:"serverId"` // "" for every server
	Builtin   bool      `json:"builtin"`
	CreatedAt time.Time `json:"createdAt"`
}

// KindInfo describes what a kind of rule measures, for the rules editor.
type KindInfo struct {
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
}

// Kinds is every kind of rule, in the order the editor lists them.
var Kinds = []KindInfo{
	{"disk", "Disk usage", "%", "A filesystem is at least this full (each disk on its own)."},
	{"inodes", "Inode usage", "%", "A filesystem has used at least this share of its inodes — it can't create files even with space left."},
	{"failed_logins", "Failed SSH logins", "per hour", "At least this many failed SSH logins in the last hour."},
	{"connections", "Open connections", "established", "At least this many established TCP connections."},
	{"kernel_errors", "Kernel errors", "in the last hour", "At least this many error lines in the kernel log (dmesg) in the last hour."},
	{"load", "CPU load", "per CPU", "The 5-minute load average divided by the number of CPUs is at least this."},
	{"memory", "Memory usage", "%", "At least this share of memory is in use (not counting what the kernel can free)."},
	{"swap", "Swap usage", "%", "At least this share of swap is in use."},
	{"unreachable", "Server unreachable", "", "The panel can't log in to the server to check it."},
}

func kindInfo(kind string) (KindInfo, bool) {
	for _, k := range Kinds {
		if k.Kind == kind {
			return k, true
		}
	}
	return KindInfo{}, false
}

// DefaultRules are created once, with fixed IDs.
var DefaultRules = []Rule{
	{ID: "builtin-disk-95", Kind: "disk", Name: "Disk almost full", Threshold: 95, Severity: "critical", Enabled: true},
	{ID: "builtin-disk-85", Kind: "disk", Name: "Disk filling up", Threshold: 85, Severity: "warning", Enabled: true},
	{ID: "builtin-inodes-95", Kind: "inodes", Name: "Inodes almost used up", Threshold: 95, Severity: "critical", Enabled: true},
	{ID: "builtin-failed-logins", Kind: "failed_logins", Name: "Many failed SSH logins", Threshold: 20, Severity: "warning", Enabled: true},
	{ID: "builtin-connections", Kind: "connections", Name: "Too many open connections", Threshold: 1000, Severity: "warning", Enabled: true},
	{ID: "builtin-kernel-errors", Kind: "kernel_errors", Name: "Kernel errors", Threshold: 1, Severity: "warning", Enabled: true},
	{ID: "builtin-load", Kind: "load", Name: "High CPU load", Threshold: 2, Severity: "warning", Enabled: true},
	{ID: "builtin-memory", Kind: "memory", Name: "Memory almost full", Threshold: 95, Severity: "warning", Enabled: true},
	{ID: "builtin-swap", Kind: "swap", Name: "Heavy swapping", Threshold: 80, Severity: "warning", Enabled: false},
	{ID: "builtin-unreachable", Kind: "unreachable", Name: "Server unreachable", Threshold: 1, Severity: "critical", Enabled: true},
}

// Finding is one alert condition that holds right now.
type Finding struct {
	RuleID   string
	Subject  string // what it's about on the server: a mount point, or ""
	Severity string
	Message  string
	Value    float64
}

// Evaluate checks a server's snapshot against the rules that apply to it.
// unreachable is the reason the server couldn't be checked, or "" if it
// was. It returns the findings, and the IDs of the rules it could judge:
// when the server can't be reached only the unreachable rule can be, so
// alerts from other rules stay as they were until it can.
func Evaluate(rules []Rule, serverID string, snap *Snapshot, unreachable string) (findings []Finding, judged []string) {
	type candidate struct {
		f    Finding
		kind string
		thr  float64
	}
	var cands []candidate
	add := func(r Rule, subject, msg string, value float64) {
		cands = append(cands, candidate{Finding{RuleID: r.ID, Subject: subject, Severity: r.Severity, Message: msg, Value: value}, r.Kind, r.Threshold})
	}
	for _, r := range rules {
		if r.ServerID != "" && r.ServerID != serverID {
			continue
		}
		if unreachable != "" && r.Kind != "unreachable" {
			continue
		}
		judged = append(judged, r.ID)
		if !r.Enabled {
			continue
		}
		if r.Kind == "unreachable" {
			if unreachable != "" {
				add(r, "", "Can't reach the server: "+unreachable, 1)
			}
			continue
		}
		if snap == nil {
			continue
		}
		switch r.Kind {
		case "disk":
			for _, d := range snap.Disks {
				if d.UsedPercent >= r.Threshold {
					add(r, d.Mount, fmt.Sprintf("Disk %s is %.0f%% full (%s free of %s)", d.Mount, d.UsedPercent, bytesLabel(d.AvailBytes), bytesLabel(d.SizeBytes)), d.UsedPercent)
				}
			}
		case "inodes":
			for _, d := range snap.Disks {
				if d.InodesUsedPercent >= 0 && d.InodesUsedPercent >= r.Threshold {
					add(r, d.Mount, fmt.Sprintf("Disk %s has used %.0f%% of its inodes", d.Mount, d.InodesUsedPercent), d.InodesUsedPercent)
				}
			}
		case "failed_logins":
			if n := float64(snap.Failed.LastHour); n >= r.Threshold {
				msg := fmt.Sprintf("%d failed SSH logins in the last hour", snap.Failed.LastHour)
				if len(snap.Failed.TopSources) > 0 {
					msg += fmt.Sprintf(" — most from %s (%d)", snap.Failed.TopSources[0].Key, snap.Failed.TopSources[0].Count)
				}
				if len(snap.Failed.TopUsers) > 0 {
					msg += fmt.Sprintf(", trying user %q", snap.Failed.TopUsers[0].Key)
				}
				add(r, "", msg, n)
			}
		case "connections":
			if n := float64(snap.Connections.Established); snap.Connections.Readable && n >= r.Threshold {
				msg := fmt.Sprintf("%d established TCP connections", snap.Connections.Established)
				if len(snap.Connections.TopPeers) > 0 {
					msg += fmt.Sprintf(" — most with %s (%d)", snap.Connections.TopPeers[0].Key, snap.Connections.TopPeers[0].Count)
				}
				add(r, "", msg, n)
			}
		case "kernel_errors":
			if n := float64(snap.KernelErrors.LastHour); n >= r.Threshold {
				msg := fmt.Sprintf("%d kernel error%s in the last hour", snap.KernelErrors.LastHour, plural(snap.KernelErrors.LastHour))
				if len(snap.KernelErrors.Recent) > 0 {
					msg += ": " + truncate(snap.KernelErrors.Recent[0].Message, 140)
				}
				add(r, "", msg, n)
			}
		case "load":
			perCPU := snap.Load5 / float64(max(snap.CPUs, 1))
			if perCPU >= r.Threshold {
				add(r, "", fmt.Sprintf("Load %.2f on %d CPU%s (%.1f per CPU over 5 minutes)", snap.Load5, snap.CPUs, plural(snap.CPUs), perCPU), round1(perCPU))
			}
		case "memory":
			if snap.Memory.TotalBytes > 0 && snap.Memory.UsedPercent >= r.Threshold {
				add(r, "", fmt.Sprintf("Memory %.0f%% used (%s available of %s)", snap.Memory.UsedPercent, bytesLabel(snap.Memory.AvailableBytes), bytesLabel(snap.Memory.TotalBytes)), snap.Memory.UsedPercent)
			}
		case "swap":
			if snap.Memory.SwapTotalBytes > 0 {
				p := round1(100 * float64(snap.Memory.SwapUsedBytes) / float64(snap.Memory.SwapTotalBytes))
				if p >= r.Threshold {
					add(r, "", fmt.Sprintf("Swap %.0f%% used (%s of %s)", p, bytesLabel(snap.Memory.SwapUsedBytes), bytesLabel(snap.Memory.SwapTotalBytes)), p)
				}
			}
		}
	}

	// Two rules of one kind firing on the same thing (disk / at 96% is past
	// both 85% and 95%) make one alert: the stricter rule's.
	best := map[string]candidate{}
	var order []string
	for _, c := range cands {
		key := c.kind + "\x00" + c.f.Subject
		cur, ok := best[key]
		if !ok {
			order = append(order, key)
		}
		if !ok || c.thr > cur.thr {
			best[key] = c
		}
	}
	for _, k := range order {
		findings = append(findings, best[k].f)
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Severity == "critical" && findings[j].Severity != "critical" })
	return findings, judged
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func bytesLabel(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(b)/float64(div)), ".0") + " " + string("KMGTPE"[exp]) + "iB"
}
