// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { PromChangeEditor, type EditorMode } from "@/components/PromChangeEditor";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";
import type { DeployJob, PromJob, PromReport, PromRule } from "@/lib/api";
import { loadSnapshot } from "@/lib/snapshots";
import type { StatusRole } from "@/lib/format";

const RULE_STATE_ROLE: Record<string, StatusRole> = {
  firing: "critical",
  pending: "warning",
  inactive: "good",
};

function healthRole(health: string): StatusRole {
  return health === "up" ? "good" : health === "down" ? "critical" : "muted";
}

function formatFor(seconds: number) {
  if (!seconds) return "immediately";
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

// The "Rules & targets" tab of a Prometheus service: what the running
// Prometheus loaded and how it's doing, plus guided changes to scrape jobs
// and alert rules.
export function PrometheusRules({
  serviceId,
  hasSavedPassword,
  sshLabel,
}: {
  serviceId: string;
  hasSavedPassword: boolean;
  sshLabel: string;
}) {
  const [report, setReport] = useState<PromReport | null>(null);
  // When the report on screen is the saved reading rather than a live one:
  // the time it was taken. null once a live reading has arrived.
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const liveArrived = useRef(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [needPassword, setNeedPassword] = useState(!hasSavedPassword);
  const [password, setPassword] = useState("");
  const [editor, setEditor] = useState<EditorMode | null>(null);
  const [openJobs, setOpenJobs] = useState<Record<string, boolean>>({});
  const [moving, setMoving] = useState<DeployJob | null>(null);
  const [filter, setFilter] = useState("");
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({});
  const editorRef = useRef<HTMLDivElement | null>(null);
  const moveTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  const inspect = useCallback(
    async (pw: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`/api/services/${serviceId}/prometheus/inspect`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ password: pw }),
        });
        if (!res.ok) {
          const text = await res.text();
          if ((text.includes("no password supplied") || text.includes("no SSH password is saved"))) {
            setNeedPassword(true);
            return;
          }
          setError(text);
          return;
        }
        setNeedPassword(false);
        liveArrived.current = true;
        setReport(await res.json());
        setSavedAt(null);
      } finally {
        setLoading(false);
      }
    },
    [serviceId],
  );

  // Show the last saved reading at once, then fetch a live one behind it.
  useEffect(() => {
    let cancelled = false;
    (async () => {
      const saved = await loadSnapshot<PromReport>(serviceId, "inspect");
      if (cancelled) return;
      if (saved && !liveArrived.current) {
        setReport(saved.data);
        setSavedAt(saved.fetchedAt);
      }
      if (hasSavedPassword) inspect("");
    })();
    return () => {
      cancelled = true;
    };
  }, [serviceId, hasSavedPassword, inspect]);

  useEffect(() => {
    return () => {
      if (moveTimer.current) clearInterval(moveTimer.current);
    };
  }, []);

  function openEditor(mode: EditorMode) {
    setEditor(mode);
    setTimeout(() => editorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
  }

  async function moveToDisk() {
    if (!confirm("Move the Prometheus config to the server's disk? Prometheus restarts once, which takes a few seconds.")) return;
    const res = await fetch(`/api/services/${serviceId}/prometheus/move-to-disk`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    });
    if (!res.ok) {
      toast.error(await res.text(), { duration: 8000 });
      return;
    }
    const { jobId } = await res.json();
    setMoving({ id: jobId, status: "running", log: [], startedAt: "", updatedAt: "" });
    moveTimer.current = setInterval(async () => {
      const r = await fetch(`/api/deploy/jobs/${jobId}`, { cache: "no-store" });
      if (!r.ok) return;
      const snap: DeployJob = await r.json();
      setMoving(snap);
      if (snap.status === "running") return;
      if (moveTimer.current) clearInterval(moveTimer.current);
      moveTimer.current = null;
      if (snap.status === "success") {
        toast.success("The config now lives on the server's disk.");
        // Give the recreated container a moment to start answering.
        setTimeout(() => inspect(password), 3000);
      }
    }, 1500);
  }

  const passwordPrompt = (
      <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5">
        <label className="flex flex-1 flex-col gap-1.5 text-sm font-medium text-text-secondary">
          SSH password for {sshLabel}
          <input
            id="prom-password"
            type="password"
            className="field"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="off"
          />
        </label>
        <Button onClick={() => inspect(password)} disabled={!password || loading}>
          {loading ? "Inspecting…" : "Inspect Prometheus"}
        </Button>
      </div>
  );

  if (needPassword && !report) {
    return passwordPrompt;
  }

  if (!report) {
    return (
      <div className="card flex flex-col items-start gap-3 p-5">
        {loading ? (
          <p className="text-sm text-text-secondary">Connecting to the server and reading the running Prometheus…</p>
        ) : (
          <>
            <p className="text-sm text-status-critical">{error ?? "Not inspected yet."}</p>
            <Button variant="secondary" onClick={() => inspect(password)}>
              Try again
            </Button>
          </>
        )}
      </div>
    );
  }

  const stale = savedAt !== null;
  const ruleCount = report.ruleGroups.reduce((n, g) => n + g.rules.length, 0);
  const targetCount = report.targetsUp + report.targetsDown;

  // Search across jobs, targets and rules. A job that matches by name shows
  // all its targets; one that matches only by target shows just those.
  const q = filter.trim().toLowerCase();
  const jobs = !q
    ? report.jobs
    : report.jobs.flatMap((j) => {
        if (j.name.toLowerCase().includes(q)) return [j];
        const targets = j.targets.filter(
          (t) =>
            t.instance.toLowerCase().includes(q) ||
            (t.note ?? "").toLowerCase().includes(q) ||
            (t.lastError ?? "").toLowerCase().includes(q),
        );
        return targets.length ? [{ ...j, targets }] : [];
      });
  const groups = !q
    ? report.ruleGroups
    : report.ruleGroups.flatMap((g) => {
        if (g.name.toLowerCase().includes(q)) return [g];
        const rules = g.rules.filter((r) => r.name.toLowerCase().includes(q) || r.query.toLowerCase().includes(q));
        return rules.length ? [{ ...g, rules }] : [];
      });
  const shownTargets = jobs.reduce((n, j) => n + j.targets.length, 0);
  const shownRules = groups.reduce((n, g) => n + g.rules.length, 0);
  const groupKey = (g: { file: string; name: string }) => `${g.file}:${g.name}`;
  const groupOpen = (g: (typeof groups)[number]) =>
    openGroups[groupKey(g)] ?? (!!q || ruleCount <= 25 || g.rules.some((r) => r.state === "firing" || r.state === "pending"));

  return (
    <div className="flex flex-col gap-5">
      {needPassword && passwordPrompt}
      {stale && (
        <div
          className={`flex flex-wrap items-center justify-between gap-2 rounded-2xl px-4 py-3 text-sm ${
            error ? "bg-[color-mix(in_srgb,var(--status-warning)_14%,transparent)]" : "bg-accent-soft"
          }`}
          aria-live="polite"
        >
          <span className="text-text-primary">
            {error ? (
              <>
                <span className="font-semibold">Couldn&apos;t refresh:</span> {error}. Showing the reading saved at{" "}
                {new Date(savedAt).toLocaleString()}.
              </>
            ) : (
              <>
                Showing the reading saved at {new Date(savedAt).toLocaleString()}.{" "}
                <span className="font-semibold">{loading ? "Refreshing…" : ""}</span> Changes open once it&apos;s up to date.
              </>
            )}
          </span>
          {error && (
            <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => inspect(password)} disabled={loading}>
              {loading ? "Refreshing…" : "Try again"}
            </Button>
          )}
        </div>
      )}
      <section className="flex flex-col gap-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="text-sm font-semibold text-text-primary">
              Prometheus {report.version} <span className="font-normal text-text-muted">· {report.container}</span>
            </p>
            <p className="mt-1 break-all font-mono text-xs text-text-secondary">{report.configPath}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusBadge
                role={report.onDisk ? "good" : "warning"}
                label={report.onDisk ? "Config on server disk" : "Config inside container only"}
              />
              <StatusBadge
                role={report.reloadOk ? "good" : "critical"}
                label={report.reloadOk ? `Loaded ${new Date(report.lastReload).toLocaleString()}` : "Last reload failed"}
              />
            </div>
          </div>
          <Button variant="secondary" onClick={() => inspect(password)} disabled={loading}>
            {loading ? "Inspecting…" : "Inspect again"}
          </Button>
        </div>

        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatTile hero label="Targets up" value={`${report.targetsUp} / ${report.targetsUp + report.targetsDown}`} hint={`${report.jobs.length} scrape jobs`} />
          <StatTile label="Targets down" value={String(report.targetsDown)} hint={report.targetsDown ? "Listed first below" : "All answering"} />
          <StatTile label="Alerts firing" value={String(report.alertsFiring)} hint={`${report.alertsPending} pending`} />
          <StatTile label="Alert rules" value={String(ruleCount)} hint={`${report.ruleFiles.length} rule file(s)`} />
        </div>
      </section>

      {!report.onDisk && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">Config lives inside the container</h2>
          <p className="max-w-2xl text-sm text-text-secondary">
            Changes made now would be lost the next time the container is recreated, for example on Redeploy.
            {report.canMoveToDisk
              ? " Move it to the server's disk first. The panel writes the config Prometheus is running now into a folder next to the service and mounts it, so nothing about your monitoring changes."
              : " Mount the config folder from the server in your compose file to make changes permanent."}
          </p>
          {report.canMoveToDisk && !moving && (
            <Button className="w-fit" onClick={moveToDisk}>
              Move config to disk
            </Button>
          )}
          {moving && (
            <>
              <StatusBadge
                role={moving.status === "success" ? "good" : moving.status === "failed" ? "critical" : "warning"}
                label={moving.status === "running" ? "Moving…" : moving.status}
              />
              <pre className="log-panel max-h-64 overflow-y-auto whitespace-pre-wrap p-4">
                {moving.log.length ? moving.log.join("") : "Waiting for output…"}
              </pre>
            </>
          )}
        </section>
      )}

      {(report.jobs.length > 3 || ruleCount > 10) && (
        <div className="sticky top-[calc(3.75rem+env(safe-area-inset-top,0px))] z-20 flex flex-col gap-1">
          <input
            id="prom-filter"
            type="search"
            className="field shadow-card"
            placeholder={`Search ${report.jobs.length} jobs, ${targetCount} targets and ${ruleCount} rules`}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          {q && (
            <p className="px-2 text-xs text-text-muted">
              Showing {jobs.length} job(s), {shownTargets} target(s) and {shownRules} rule(s) matching &ldquo;{filter.trim()}&rdquo;.
            </p>
          )}
        </div>
      )}

      <div ref={editorRef}>
        {editor && (
          <PromChangeEditor
            key={JSON.stringify(editor.kind === "file" ? editor.file.path : editor)}
            serviceId={serviceId}
            password={password}
            report={report}
            mode={editor}
            onClose={() => setEditor(null)}
            onApplied={() => inspect(password)}
          />
        )}
      </div>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="section-title">Scrape jobs and targets</h2>
          <Button onClick={() => openEditor({ kind: "job" })} disabled={stale}>
            + New scrape job
          </Button>
        </div>
        {report.jobs.length === 0 && <p className="text-sm text-text-muted">No scrape jobs yet.</p>}
        {report.jobs.length > 0 && jobs.length === 0 && <p className="text-sm text-text-muted">No job or target matches.</p>}
        <ul className="flex flex-col gap-1.5">
          {jobs.map((job) => (
            <JobRow
              key={job.name}
              job={job}
              open={openJobs[job.name] ?? (!!q || job.targets.some((t) => t.health !== "up"))}
              onToggle={() => setOpenJobs((o) => ({ ...o, [job.name]: !(o[job.name] ?? job.targets.some((t) => t.health !== "up")) }))}
              onAdd={() => openEditor({ kind: "targets", job: job.name })}
              canEdit={!stale}
            />
          ))}
        </ul>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="section-title">Alert rules</h2>
          <Button onClick={() => openEditor({ kind: "rule" })} disabled={stale}>
            + New alert rule
          </Button>
        </div>
        {report.ruleGroups.length === 0 && (
          <p className="rounded-2xl bg-surface-2 px-4 py-6 text-center text-sm text-text-muted">
            No alert rules loaded. Add one to be told when a server, website or certificate needs attention.
          </p>
        )}
        {report.ruleGroups.length > 0 && groups.length === 0 && <p className="text-sm text-text-muted">No rule matches.</p>}
        {groups.map((g) => {
          const open = groupOpen(g);
          const firing = g.rules.filter((r) => r.state === "firing").length;
          const pending = g.rules.filter((r) => r.state === "pending").length;
          return (
            <div key={groupKey(g)} className="flex flex-col gap-1.5">
              <button
                type="button"
                aria-expanded={open}
                onClick={() => setOpenGroups((o) => ({ ...o, [groupKey(g)]: !open }))}
                className="flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl px-1 py-1 text-left hover:bg-surface-2"
              >
                <span aria-hidden className="w-3 text-text-muted">
                  {open ? "▾" : "▸"}
                </span>
                <span className="text-xs font-bold uppercase tracking-[0.07em] text-text-muted">{g.name}</span>
                <span className="font-mono text-xs text-text-muted">· {g.file.split("/").pop()}</span>
                <span className="text-xs text-text-muted">· {g.rules.length} rule(s)</span>
                {firing > 0 && <StatusBadge role="critical" label={`${firing} firing`} />}
                {pending > 0 && <StatusBadge role="warning" label={`${pending} pending`} />}
              </button>
              {open && (
                <ul className="flex flex-col gap-1.5">
                  {g.rules.map((rule) => (
                    <RuleRow key={rule.name} rule={rule} />
                  ))}
                </ul>
              )}
            </div>
          );
        })}
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Files</h2>
        <p className="text-sm text-text-muted">
          Edit a file directly. You still get a before/after review and a promtool check before anything is written.
        </p>
        <ul className="flex flex-col gap-1.5">
          {[report.config, ...report.ruleFiles].map((f) => (
            <li key={f.path} className="flex flex-wrap items-center justify-between gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="min-w-0">
                <p className="break-all font-mono text-sm text-text-primary">{f.path}</p>
                <p className="text-xs text-text-muted">
                  {f.hostPath ? `On the server at ${f.hostPath}` : "Inside the container only"} · {f.content.split("\n").length} lines
                </p>
              </div>
              <Button variant="secondary" onClick={() => openEditor({ kind: "file", file: f })} disabled={stale}>
                Edit
              </Button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function JobRow({
  job,
  open,
  onToggle,
  onAdd,
  canEdit,
}: {
  job: PromJob;
  open: boolean;
  onToggle: () => void;
  onAdd: () => void;
  canEdit: boolean;
}) {
  const up = job.targets.filter((t) => t.health === "up").length;
  const down = job.targets.length - up;
  return (
    <li className="overflow-hidden rounded-2xl bg-surface-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3">
        <button onClick={onToggle} aria-expanded={open} className="flex min-w-0 basis-full items-center gap-3 text-left sm:basis-auto sm:flex-1">
          <span aria-hidden className="w-3 text-text-muted">
            {open ? "▾" : "▸"}
          </span>
          <span className="min-w-0 truncate font-mono text-sm font-bold text-text-primary">{job.name}</span>
          <span className="rounded-full bg-surface-1 px-2 py-0.5 text-[11px] font-semibold text-text-muted">
            {job.kind === "blackbox" ? `blackbox ${job.module ?? ""}` : "exporter"}
          </span>
        </button>
        <span className="ml-6 text-xs font-semibold tabular-nums sm:ml-0">
          <span className="text-status-good">{up} up</span>
          {down > 0 && <span className="text-status-critical"> · {down} down</span>}
        </span>
        <Button variant="secondary" className="ml-auto px-3 py-1 text-xs sm:ml-0" onClick={onAdd} disabled={!canEdit}>
          + Targets
        </Button>
      </div>
      {open && (
        <ul className="flex flex-col gap-px border-t border-border">
          {job.targets.length === 0 && <li className="px-4 py-3 text-sm text-text-muted">No active targets.</li>}
          {job.targets.map((t) => (
            <li key={t.scrapeUrl} className="flex flex-col items-start gap-1 bg-surface-1/60 px-4 py-2.5 sm:flex-row sm:items-center sm:gap-3">
              <StatusBadge role={healthRole(t.health)} label={t.health} />
              <span className="min-w-0 break-all font-mono text-xs text-text-primary">
                {t.instance}
                {t.note && <span className="ml-2 font-sans text-text-muted"># {t.note}</span>}
              </span>
              {t.lastError && <span className="min-w-0 break-words text-xs text-status-critical sm:ml-auto sm:max-w-[50%] sm:text-right">{t.lastError}</span>}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

function RuleRow({ rule }: { rule: PromRule }) {
  const state = rule.type === "recording" ? "recording" : (rule.state ?? "inactive");
  return (
    <li className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-mono text-sm font-bold text-text-primary">{rule.name}</span>
        <StatusBadge role={RULE_STATE_ROLE[state] ?? "muted"} label={rule.active > 0 ? `${state} · ${rule.active}` : state} />
        {rule.health !== "ok" && <StatusBadge role="critical" label={`rule ${rule.health}`} />}
        <span className="ml-auto text-xs text-text-muted">for {formatFor(rule.duration)}</span>
      </div>
      <code className="break-all font-mono text-xs text-text-secondary">{rule.query}</code>
      {rule.lastError && <span className="text-xs text-status-critical">{rule.lastError}</span>}
    </li>
  );
}
