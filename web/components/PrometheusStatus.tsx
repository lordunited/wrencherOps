"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/Button";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";
import type { PromNameValue, PromServerStatus } from "@/lib/api";
import { loadSnapshot } from "@/lib/snapshots";

// ---- formatting --------------------------------------------------------

function bytes(n?: number) {
  if (n === undefined || !Number.isFinite(n)) return "—";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
}

function num(n?: number) {
  if (n === undefined || !Number.isFinite(n)) return "—";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 1 }).format(n);
}

function compact(n?: number) {
  if (n === undefined || !Number.isFinite(n)) return "—";
  return new Intl.NumberFormat("en-US", { notation: "compact", maximumFractionDigits: 1 }).format(n);
}

function duration(ms: number) {
  if (!Number.isFinite(ms) || ms < 0) return "—";
  const s = Math.floor(ms / 1000);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${s}s`;
}

function when(value?: string | number) {
  if (value === undefined || value === "") return "—";
  const d = new Date(value);
  return Number.isNaN(d.getTime()) || d.getFullYear() < 2000 ? "—" : d.toLocaleString();
}

// ---- layout pieces -----------------------------------------------------

function Card({ title, children, className = "" }: { title: string; children: React.ReactNode; className?: string }) {
  return (
    <section className={`card flex min-w-0 flex-col gap-3 p-4 sm:p-5 ${className}`}>
      <h2 className="section-title">{title}</h2>
      {children}
    </section>
  );
}

type RowTone = "good" | "warn" | "bad" | undefined;

function Rows({ rows }: { rows: [string, React.ReactNode, RowTone?][] }) {
  return (
    <dl className="grid grid-cols-[minmax(8.5rem,1fr)_minmax(0,auto)] gap-x-4 gap-y-2 text-sm">
      {rows.map(([label, value, tone]) => (
        <div key={label} className="contents">
          <dt className="text-text-secondary">{label}</dt>
          <dd
            className={`min-w-0 break-words text-right font-mono text-[13px] tabular-nums ${
              tone === "bad" ? "font-bold text-status-critical" : tone === "warn" ? "font-bold text-status-warning" : tone === "good" ? "text-status-good" : "text-text-primary"
            }`}
          >
            {value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

const failTone = (n?: number): RowTone => (n && n > 0 ? "bad" : undefined);

function TopTable({ title, rows, format }: { title: string; rows?: PromNameValue[]; format: (n: number) => string }) {
  const max = Math.max(1, ...(rows ?? []).map((r) => r.value));
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <p className="text-xs font-bold uppercase tracking-[0.07em] text-text-muted">{title}</p>
      {!rows || rows.length === 0 ? (
        <p className="text-sm text-text-muted">No data.</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {rows.map((r) => (
            <li key={r.name} className="flex flex-col gap-0.5">
              <div className="flex items-baseline justify-between gap-3 text-xs">
                <span className="min-w-0 truncate font-mono text-text-primary" title={r.name}>
                  {r.name}
                </span>
                <span className="shrink-0 font-mono tabular-nums text-text-secondary">{format(r.value)}</span>
              </div>
              <div className="h-1.5 rounded-full bg-surface-2">
                <div className="h-1.5 rounded-full bg-accent" style={{ width: `${Math.max(2, (r.value / max) * 100)}%` }} />
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ---- the tab -------------------------------------------------------------

// Live internals of the service's Prometheus, read from its own APIs,
// metrics and data folder.
export function PrometheusStatus({ serviceId }: { serviceId: string }) {
  const [status, setStatus] = useState<PromServerStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [checkedAt, setCheckedAt] = useState<Date | null>(null);
  // True while the status on screen is the saved reading, not a live one.
  const [fromSaved, setFromSaved] = useState(false);
  const liveArrived = useRef(false);
  const [flagFilter, setFlagFilter] = useState("");
  const [showFlags, setShowFlags] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/services/${serviceId}/prometheus/status`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: "" }),
      });
      if (res.status === 401) {
        setError("Your session has expired. Sign in again.");
        return;
      }
      if (!res.ok) {
        const text = (await res.text()).trim();
        setError(text.startsWith("<") ? `The panel answered with HTTP ${res.status}.` : text);
        return;
      }
      liveArrived.current = true;
      setStatus(await res.json());
      setCheckedAt(new Date());
      setFromSaved(false);
    } finally {
      setLoading(false);
    }
  }, [serviceId]);

  // Show the last saved reading at once, then fetch a live one behind it.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const saved = await loadSnapshot<PromServerStatus>(serviceId, "status");
      if (cancelled) return;
      if (saved && !liveArrived.current) {
        setStatus(saved.data);
        setCheckedAt(new Date(saved.fetchedAt));
        setFromSaved(true);
      }
      load();
    })();
    return () => {
      cancelled = true;
    };
  }, [serviceId, load]);

  const flags = useMemo(() => {
    const q = flagFilter.trim().toLowerCase();
    return Object.entries(status?.flags ?? {})
      .filter(([k, v]) => !q || k.toLowerCase().includes(q) || v.toLowerCase().includes(q))
      .sort(([a], [b]) => a.localeCompare(b));
  }, [status, flagFilter]);

  if (!status) {
    return (
      <div className="card flex flex-col items-start gap-3 p-5">
        <p className={`text-sm ${error ? "text-status-critical" : "text-text-secondary"}`}>
          {error ?? "Reading the server's status…"}
        </p>
        {error && (
          <Button variant="secondary" onClick={load} disabled={loading}>
            Try again
          </Button>
        )}
      </div>
    );
  }

  const m = status.metrics;
  const rt = status.runtime;
  const head = status.tsdb?.headStats;
  // Uptime as of when this status was read.
  const uptimeMs = checkedAt ? checkedAt.getTime() - new Date(rt.startTime).getTime() : NaN;
  const freeShare = status.disk.fsSizeBytes > 0 ? status.disk.fsFreeBytes / status.disk.fsSizeBytes : undefined;
  const replay = status.walReplay;
  const replayDone = !replay || replay.max <= 0 || replay.current >= replay.max;
  const fsyncAvgMs =
    m.prometheus_tsdb_wal_fsync_duration_seconds_count > 0
      ? (m.prometheus_tsdb_wal_fsync_duration_seconds_sum / m.prometheus_tsdb_wal_fsync_duration_seconds_count) * 1000
      : undefined;
  const active = status.alertmanagers?.activeAlertmanagers ?? [];

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-text-secondary">
          <span className="font-semibold text-text-primary">Prometheus {status.build.version}</span> · up {duration(uptimeMs)}
          {checkedAt && (
            <>
              {" "}
              · {fromSaved ? "saved reading from" : "checked"} {checkedAt.toLocaleString()}
            </>
          )}
          {fromSaved && (
            <span className={`ml-2 font-semibold ${error ? "text-status-warning" : "text-accent"}`}>
              {error ? `Couldn't refresh: ${error}` : loading ? "Refreshing…" : ""}
            </span>
          )}
        </p>
        <Button variant="secondary" onClick={load} disabled={loading}>
          {loading ? "Refreshing…" : "Refresh"}
        </Button>
      </div>

      {status.attention.length === 0 ? (
        <p className="rounded-2xl bg-[color-mix(in_srgb,var(--status-good)_12%,transparent)] px-4 py-3 text-sm text-text-primary">
          <span className="font-bold text-status-good">Nothing needs attention.</span> Storage, WAL, rules and alerting all
          look healthy.
        </p>
      ) : (
        <Card title={`Needs attention (${status.attention.length})`}>
          <ul className="flex flex-col gap-2">
            {status.attention.map((f) => (
              <li key={f.title} className="flex flex-col gap-1 rounded-2xl bg-surface-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <StatusBadge role={f.severity === "critical" ? "critical" : "warning"} label={f.severity} />
                  <span className="text-sm font-bold text-text-primary">{f.title}</span>
                </div>
                <p className="text-sm text-text-secondary">{f.detail}</p>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile hero label="Series in memory" value={compact(head?.numSeries ?? m.prometheus_tsdb_head_series)} hint={`${compact(head?.chunkCount)} chunks`} />
        <StatTile label="Data on disk" value={bytes(status.disk.dataBytes)} hint={`${status.disk.blocks} blocks + WAL`} />
        <StatTile
          label="Free disk"
          value={freeShare === undefined ? "—" : `${(freeShare * 100).toFixed(0)}%`}
          hint={`${bytes(status.disk.fsFreeBytes)} of ${bytes(status.disk.fsSizeBytes)}`}
        />
        <StatTile label="Memory" value={bytes(m.process_resident_memory_bytes)} hint={`${num(rt.goroutineCount)} goroutines`} />
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Card title="TSDB and head block">
          <Rows
            rows={[
              ["Series", num(head?.numSeries)],
              ["Label pairs", num(head?.numLabelPairs)],
              ["Chunks", num(head?.chunkCount)],
              ["Head starts", head ? when(head.minTime) : "—"],
              ["Head ends", head ? when(head.maxTime) : "—"],
              ["Head time span", head ? duration(head.maxTime - head.minTime) : "—"],
              ["Samples appended", compact(m.prometheus_tsdb_head_samples_appended_total)],
              ["Persisted blocks", `${num(m.prometheus_tsdb_blocks_loaded)} · ${bytes(m.prometheus_tsdb_storage_blocks_bytes)}`],
              ["Retention", `${rt.storageRetention || "—"}`],
              ["Blocks deleted by time / size", `${num(m.prometheus_tsdb_time_retentions_total)} / ${num(m.prometheus_tsdb_size_retentions_total)}`],
              ["Compactions", num(m.prometheus_tsdb_compactions_total)],
              ["Failed compactions", num(m.prometheus_tsdb_compactions_failed_total), failTone(m.prometheus_tsdb_compactions_failed_total)],
              ["Failed head truncations", num(m.prometheus_tsdb_head_truncations_failed_total), failTone(m.prometheus_tsdb_head_truncations_failed_total)],
              ["Out-of-order samples", compact(m.prometheus_tsdb_out_of_order_samples_total)],
              ["Corruptions", num(rt.corruptionCount), failTone(rt.corruptionCount)],
              ["Data folder", <span key="p" className="break-all">{status.disk.dataPath}</span>],
            ]}
          />
        </Card>

        <Card title="Write-ahead log (WAL)">
          <Rows
            rows={[
              ["Size on disk", bytes(status.disk.walBytes || m.prometheus_tsdb_wal_storage_size_bytes)],
              ["Segments", num(status.disk.walSegments)],
              ["Current segment", num(m.prometheus_tsdb_wal_segment_current)],
              [
                "Startup replay",
                replay && replay.max > 0 ? (replayDone ? "finished" : `segment ${replay.current} of ${replay.max}`) : "finished",
                replayDone ? "good" : "warn",
              ],
              ["Average fsync", fsyncAvgMs === undefined ? "—" : `${fsyncAvgMs.toFixed(1)} ms`, fsyncAvgMs && fsyncAvgMs > 100 ? "warn" : undefined],
              ["Page flushes", compact(m.prometheus_tsdb_wal_page_flushes_total)],
              ["Truncations", num(m.prometheus_tsdb_wal_truncations_total)],
              ["Failed truncations", num(m.prometheus_tsdb_wal_truncations_failed_total), failTone(m.prometheus_tsdb_wal_truncations_failed_total)],
              ["Checkpoints created", num(m.prometheus_tsdb_checkpoint_creations_total)],
              ["Failed checkpoints", num(m.prometheus_tsdb_checkpoint_creations_failed_total), failTone(m.prometheus_tsdb_checkpoint_creations_failed_total)],
              ["Failed writes", num(m.prometheus_tsdb_wal_writes_failed_total), failTone(m.prometheus_tsdb_wal_writes_failed_total)],
              ["WAL corruptions", num(m.prometheus_tsdb_wal_corruptions_total), failTone(m.prometheus_tsdb_wal_corruptions_total)],
            ]}
          />
        </Card>

        <Card title="Runtime">
          <Rows
            rows={[
              ["Started", when(rt.startTime)],
              ["Uptime", duration(uptimeMs)],
              ["Config reload", rt.reloadConfigSuccess ? `OK · ${when(rt.lastConfigTime)}` : "Failed", rt.reloadConfigSuccess ? "good" : "bad"],
              ["Working directory", <span key="c" className="break-all">{rt.CWD}</span>],
              ["Resident memory", bytes(m.process_resident_memory_bytes)],
              ["Heap in use", bytes(m.go_memstats_heap_inuse_bytes)],
              ["CPU time used", m.process_cpu_seconds_total ? duration(m.process_cpu_seconds_total * 1000) : "—"],
              ["Goroutines", num(rt.goroutineCount)],
              ["GOMAXPROCS / GOGC", `${num(rt.GOMAXPROCS)} / ${rt.GOGC || "—"}`],
              ["Open file handles", `${num(m.process_open_fds)} of ${num(m.process_max_fds)}`],
            ]}
          />
        </Card>

        <Card title="Rules and alerting">
          <Rows
            rows={[
              ["Rule evaluations", compact(m.prometheus_rule_evaluations_total)],
              ["Failed evaluations", num(m.prometheus_rule_evaluation_failures_total), failTone(m.prometheus_rule_evaluation_failures_total)],
              ["Missed rule rounds", num(m.prometheus_rule_group_iterations_missed_total), failTone(m.prometheus_rule_group_iterations_missed_total)],
              ["Alertmanagers connected", num(active.length), active.length === 0 ? "warn" : "good"],
              ["Notifications sent", compact(m.prometheus_notifications_sent_total)],
              ["Notifications dropped", num(m.prometheus_notifications_dropped_total), failTone(m.prometheus_notifications_dropped_total)],
              ["Notification errors", num(m.prometheus_notifications_errors_total), failTone(m.prometheus_notifications_errors_total)],
              ["Notification queue", `${num(m.prometheus_notifications_queue_length)} of ${num(m.prometheus_notifications_queue_capacity)}`],
              ["Discovered targets", num(m.prometheus_sd_discovered_targets)],
              ["Scrapes over sample limit", num(m.prometheus_target_scrapes_exceeded_sample_limit_total), failTone(m.prometheus_target_scrapes_exceeded_sample_limit_total)],
            ]}
          />
          {active.length > 0 && (
            <ul className="flex flex-col gap-1">
              {active.map((a) => (
                <li key={a.url} className="break-all font-mono text-xs text-text-secondary">
                  {a.url}
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Build">
          <Rows
            rows={[
              ["Version", status.build.version],
              ["Revision", <span key="r" className="break-all">{status.build.revision?.slice(0, 12)}</span>],
              ["Branch", status.build.branch],
              ["Built", status.build.buildDate],
              ["Go", status.build.goVersion],
            ]}
          />
        </Card>

        <Card title="Where the series come from" className="lg:col-span-2">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
            <TopTable title="Series by metric name" rows={status.tsdb?.seriesCountByMetricName} format={num} />
            <TopTable title="Series by label pair" rows={status.tsdb?.seriesCountByLabelValuePair} format={num} />
            <TopTable title="Distinct values by label" rows={status.tsdb?.labelValueCountByLabelName} format={num} />
            <TopTable title="Memory by label" rows={status.tsdb?.memoryInBytesByLabelName} format={bytes} />
          </div>
        </Card>

        <Card title={`Startup flags (${Object.keys(status.flags).length})`} className="lg:col-span-2">
          {!showFlags ? (
            <Button variant="secondary" className="w-fit" onClick={() => setShowFlags(true)}>
              Show all flags
            </Button>
          ) : (
            <>
              <input
                id="flag-filter"
                type="search"
                className="field"
                placeholder="Filter flags, e.g. retention or web."
                value={flagFilter}
                onChange={(e) => setFlagFilter(e.target.value)}
              />
              <div className="table-card shadow-none">
                <table className="w-full border-collapse text-sm">
                  <tbody>
                    {flags.map(([k, v]) => (
                      <tr key={k} className="border-b border-gridline">
                        <td className="font-mono text-xs text-text-secondary">{k}</td>
                        <td className="break-all font-mono text-xs text-text-primary">{v || <span className="text-text-muted">empty</span>}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </Card>
      </div>
    </div>
  );
}
