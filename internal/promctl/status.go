// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package promctl

import (
	"encoding/json"
	"fmt"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/sshexec"
)

// ServerStatus is everything the panel can learn about a running
// Prometheus: build and runtime details, TSDB and WAL state, storage use,
// its own health metrics, and a list of things that need attention.
type ServerStatus struct {
	Build         json.RawMessage    `json:"build"`
	Runtime       json.RawMessage    `json:"runtime"`
	Flags         map[string]string  `json:"flags"`
	TSDB          json.RawMessage    `json:"tsdb"`
	WALReplay     json.RawMessage    `json:"walReplay"`
	Alertmanagers json.RawMessage    `json:"alertmanagers"`
	Metrics       map[string]float64 `json:"metrics"`
	Disk          Disk               `json:"disk"`
	Attention     []Finding          `json:"attention"`
}

// Disk is storage use of the TSDB folder, measured inside the container.
type Disk struct {
	DataPath    string `json:"dataPath"`
	DataBytes   int64  `json:"dataBytes"`
	WALBytes    int64  `json:"walBytes"`
	WALSegments int    `json:"walSegments"`
	Blocks      int    `json:"blocks"`
	FSSizeBytes int64  `json:"fsSizeBytes"`
	FSFreeBytes int64  `json:"fsFreeBytes"`
	Error       string `json:"error,omitempty"`
}

// Finding is one thing that needs attention, most serious first.
type Finding struct {
	Severity string `json:"severity"` // "critical" or "warning"
	Title    string `json:"title"`
	Detail   string `json:"detail"`
}

// selfMetrics are Prometheus's own metrics worth showing. Values with
// labels are summed across them.
var selfMetrics = []string{
	"prometheus_tsdb_head_series", "prometheus_tsdb_head_chunks", "prometheus_tsdb_head_samples_appended_total",
	"prometheus_tsdb_wal_segment_current", "prometheus_tsdb_wal_storage_size_bytes", "prometheus_tsdb_wal_corruptions_total",
	"prometheus_tsdb_wal_truncations_failed_total", "prometheus_tsdb_wal_truncations_total", "prometheus_tsdb_wal_writes_failed_total",
	"prometheus_tsdb_wal_fsync_duration_seconds_sum", "prometheus_tsdb_wal_fsync_duration_seconds_count",
	"prometheus_tsdb_wal_page_flushes_total", "prometheus_tsdb_wal_completed_pages_total",
	"prometheus_tsdb_checkpoint_creations_total", "prometheus_tsdb_checkpoint_creations_failed_total",
	"prometheus_tsdb_checkpoint_deletions_failed_total",
	"prometheus_tsdb_blocks_loaded", "prometheus_tsdb_storage_blocks_bytes",
	"prometheus_tsdb_retention_limit_bytes", "prometheus_tsdb_retention_limit_seconds",
	"prometheus_tsdb_compactions_total", "prometheus_tsdb_compactions_failed_total",
	"prometheus_tsdb_reloads_failures_total", "prometheus_tsdb_head_truncations_failed_total",
	"prometheus_tsdb_time_retentions_total", "prometheus_tsdb_size_retentions_total",
	"prometheus_tsdb_out_of_order_samples_total", "prometheus_tsdb_symbol_table_size_bytes",
	"prometheus_rule_evaluations_total", "prometheus_rule_evaluation_failures_total",
	"prometheus_rule_group_iterations_missed_total",
	"prometheus_notifications_dropped_total", "prometheus_notifications_errors_total",
	"prometheus_notifications_queue_length", "prometheus_notifications_queue_capacity",
	"prometheus_notifications_alertmanagers_discovered", "prometheus_notifications_sent_total",
	"prometheus_sd_discovered_targets", "prometheus_target_scrapes_exceeded_sample_limit_total",
	"prometheus_target_scrapes_sample_out_of_order_total", "prometheus_target_scrape_pool_exceeded_target_limit_total",
	"prometheus_config_last_reload_successful",
	"process_resident_memory_bytes", "process_virtual_memory_bytes", "process_cpu_seconds_total",
	"process_open_fds", "process_max_fds", "process_start_time_seconds", "go_goroutines", "go_memstats_heap_inuse_bytes",
}

// Status gathers the ServerStatus of inst.
func Status(c *sshexec.Client, inst *Instance) (*ServerStatus, error) {
	st := &ServerStatus{Metrics: map[string]float64{}, Attention: []Finding{}}

	raw := func(endpoint string) (json.RawMessage, error) {
		var v json.RawMessage
		err := api(c, inst, endpoint, &v)
		return v, err
	}
	var metricsText string
	var g errgroup.Group
	g.SetLimit(4) // SSH servers allow 10 sessions per connection by default; leave room
	g.Go(func() (err error) { st.Build, err = raw("status/buildinfo"); return err })
	g.Go(func() (err error) { st.Runtime, err = raw("status/runtimeinfo"); return err })
	g.Go(func() error { return api(c, inst, "status/flags", &st.Flags) })
	// The rest are nice to have; a failure leaves that section empty.
	g.Go(func() error { st.TSDB, _ = raw("status/tsdb?limit=10"); return nil })
	g.Go(func() error { st.WALReplay, _ = raw("status/walreplay"); return nil })
	g.Go(func() error { st.Alertmanagers, _ = raw("alertmanagers"); return nil })
	g.Go(func() error { metricsText, _ = selfMetricsText(c, inst); return nil })
	if err := g.Wait(); err != nil {
		return nil, err
	}
	if metricsText != "" {
		st.Metrics = parseMetrics(metricsText, selfMetrics)
	}

	var runtime struct {
		CWD             string `json:"CWD"`
		ReloadOK        bool   `json:"reloadConfigSuccess"`
		CorruptionCount int    `json:"corruptionCount"`
	}
	json.Unmarshal(st.Runtime, &runtime)
	dataPath := st.Flags["storage.tsdb.path"]
	if dataPath == "" {
		dataPath = "data/"
	}
	if !path.IsAbs(dataPath) && runtime.CWD != "" {
		dataPath = path.Join(runtime.CWD, dataPath)
	}
	st.Disk = measureDisk(c, inst, path.Clean(dataPath))

	var replay struct {
		Min, Max, Current int
	}
	json.Unmarshal(st.WALReplay, &replay)
	st.Attention = findings(st, runtime.ReloadOK, runtime.CorruptionCount, replay.Current, replay.Max)
	return st, nil
}

func selfMetricsText(c *sshexec.Client, inst *Instance) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%s%s/metrics", inst.Port, inst.RoutePrefix)
	out, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(inst.Container) + " wget -qO- " + sshexec.ShellQuote(url)))
	if err != nil {
		return "", fmt.Errorf("%v %s", err, errOut)
	}
	return out, nil
}

// parseMetrics reads the Prometheus text format and sums every sample of
// each wanted metric name across its labels.
func parseMetrics(text string, wanted []string) map[string]float64 {
	want := map[string]bool{}
	for _, n := range wanted {
		want[n] = true
	}
	out := map[string]float64{}
	for _, line := range strings.Split(text, "\n") {
		if line == "" || line[0] == '#' {
			continue
		}
		end := strings.IndexAny(line, "{ ")
		if end <= 0 {
			continue
		}
		name := line[:end]
		if !want[name] {
			continue
		}
		rest := line[end:]
		if rest[0] == '{' {
			rest = rest[strings.LastIndex(rest, "}")+1:]
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || math.IsNaN(v) {
			continue
		}
		out[name] += v
	}
	return out
}

// measureDisk sizes the TSDB folder, its WAL, its blocks, and the free
// space of the filesystem it lives on, using busybox tools in the image.
// df's row is read counting from its end: busybox df (without -P, or in
// older builds) wraps a long device name — /dev/mapper/ubuntu--vg-... —
// onto a line of its own, which shifted every column and read the "Use%"
// figure as free space, reporting a healthy disk as full.
func measureDisk(c *sshexec.Client, inst *Instance, dataPath string) Disk {
	d := Disk{DataPath: dataPath}
	script := `D="$1"
echo "data $(du -sk "$D" 2>/dev/null | cut -f1)"
echo "wal $(du -sk "$D/wal" 2>/dev/null | cut -f1)"
echo "segments $(ls "$D/wal" 2>/dev/null | grep -c '^[0-9]')"
echo "blocks $(ls "$D" 2>/dev/null | grep -c '^01')"
df -Pk "$D" 2>/dev/null | tail -n +2 | tr '\n' ' ' | awk '{print "fs " $(NF-4) " " $(NF-2)}'`
	out, errOut, err := c.Output(exec(inst, false, script, dataPath))
	if err != nil {
		d.Error = strings.TrimSpace(errOut)
		return d
	}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		n, _ := strconv.ParseInt(f[1], 10, 64)
		switch f[0] {
		case "data":
			d.DataBytes = n * 1024
		case "wal":
			d.WALBytes = n * 1024
		case "segments":
			d.WALSegments = int(n)
		case "blocks":
			d.Blocks = int(n)
		case "fs":
			// Size and free space, or neither: a figure that doesn't parse
			// mustn't read as "0 bytes free".
			size, err1 := strconv.ParseInt(f[1], 10, 64)
			if len(f) > 2 && err1 == nil {
				if free, err := strconv.ParseInt(f[2], 10, 64); err == nil {
					d.FSSizeBytes, d.FSFreeBytes = size*1024, free*1024
				}
			}
		}
	}
	return d
}

// findings turns the collected numbers into plain-language problems.
func findings(st *ServerStatus, reloadOK bool, corruptions, replayCurrent, replayMax int) []Finding {
	m := st.Metrics
	var out []Finding
	add := func(sev, title, detail string) { out = append(out, Finding{sev, title, detail}) }
	count := func(name string) int { return int(m[name]) }

	if !reloadOK {
		add("critical", "Last config reload failed", "Prometheus is still running an older configuration. Check its logs, or apply a corrected config from Rules & targets.")
	}
	if corruptions > 0 || count("prometheus_tsdb_wal_corruptions_total") > 0 {
		add("critical", "Storage corruption detected",
			fmt.Sprintf("%d TSDB and %d WAL corruption(s) since start. Some data may have been lost; check free disk space and the Prometheus logs.",
				corruptions, count("prometheus_tsdb_wal_corruptions_total")))
	}
	if n := count("prometheus_tsdb_wal_writes_failed_total"); n > 0 {
		add("critical", "WAL writes are failing", fmt.Sprintf("%d failed write(s). New samples may not be stored; this is usually a full or read-only disk.", n))
	}
	if st.Disk.FSSizeBytes > 0 {
		free := float64(st.Disk.FSFreeBytes) / float64(st.Disk.FSSizeBytes)
		switch {
		case free < 0.05:
			add("critical", "Disk almost full", fmt.Sprintf("Only %.1f%% free where the TSDB lives. Prometheus stops writing when it's full.", free*100))
		case free < 0.15:
			add("warning", "Disk getting full", fmt.Sprintf("%.0f%% free where the TSDB lives.", free*100))
		}
	}
	for _, c := range []struct{ name, title string }{
		{"prometheus_tsdb_compactions_failed_total", "Compactions have failed"},
		{"prometheus_tsdb_head_truncations_failed_total", "Head truncations have failed"},
		{"prometheus_tsdb_wal_truncations_failed_total", "WAL truncations have failed"},
		{"prometheus_tsdb_checkpoint_creations_failed_total", "WAL checkpoints have failed"},
		{"prometheus_tsdb_reloads_failures_total", "Block reloads have failed"},
	} {
		if n := count(c.name); n > 0 {
			add("warning", c.title, fmt.Sprintf("%d time(s) since start. The WAL and disk use can grow until this is fixed.", n))
		}
	}
	if replayMax > 0 && replayCurrent < replayMax {
		add("warning", "Still replaying the WAL", fmt.Sprintf("Segment %d of %d. Queries may be incomplete until it finishes.", replayCurrent, replayMax))
	}
	if n := count("prometheus_rule_evaluation_failures_total"); n > 0 {
		add("warning", "Rule evaluations are failing", fmt.Sprintf("%d failed evaluation(s) since start. Look for rules with an error in Rules & targets.", n))
	}
	if n := count("prometheus_rule_group_iterations_missed_total"); n > 0 {
		add("warning", "Rule groups are falling behind", fmt.Sprintf("%d evaluation round(s) skipped because the previous one was still running.", n))
	}
	if n := count("prometheus_notifications_dropped_total"); n > 0 {
		add("warning", "Alert notifications were dropped", fmt.Sprintf("%d alert notification(s) never reached Alertmanager.", n))
	}
	if _, ok := m["prometheus_notifications_alertmanagers_discovered"]; ok && count("prometheus_notifications_alertmanagers_discovered") == 0 {
		add("warning", "No Alertmanager connected", "Alerts can fire, but nobody is notified.")
	}
	if max := m["process_max_fds"]; max > 0 && m["process_open_fds"]/max > 0.8 {
		add("warning", "Running out of file handles", fmt.Sprintf("%.0f of %.0f in use.", m["process_open_fds"], max))
	}
	if n := count("prometheus_target_scrapes_exceeded_sample_limit_total"); n > 0 {
		add("warning", "Scrapes hit their sample limit", fmt.Sprintf("%d scrape(s) were rejected for returning too many samples.", n))
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Severity == "critical" && out[b].Severity != "critical" })
	if out == nil {
		out = []Finding{}
	}
	return out
}
