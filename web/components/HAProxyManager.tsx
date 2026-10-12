// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/Button";
import { HAProxyChangeEditor, type HAProxyEditorMode } from "@/components/HAProxyChangeEditor";
import { StatTile } from "@/components/StatTile";
import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import type { HAProxyBackendServer, HAProxyReport, HAProxyRoute, HAProxySection } from "@/lib/api";
import { loadSnapshot } from "@/lib/snapshots";
import type { StatusRole } from "@/lib/format";

function serverStatusRole(status: string): StatusRole {
  const s = status.toUpperCase();
  if (s.startsWith("UP")) return "good";
  if (s.startsWith("DOWN")) return "critical";
  if (s.startsWith("DRAIN") || s.startsWith("NOLB")) return "warning";
  return "muted"; // MAINT, "no check", anything else
}

// A backend's overall health, from its servers' own statuses rather than
// HAProxy's aggregate BACKEND row — verified against a real container that
// the aggregate says UP for a backend with one of two servers down, and
// even for one with no servers at all, so it can't tell "degraded" or
// "empty" apart from healthy.
function backendHealth(servers: HAProxyBackendServer[], hasStatsPage: boolean): { role: StatusRole; label: string } {
  if (servers.length === 0) {
    return { role: "muted", label: hasStatsPage ? "no servers" : "unknown — no stats page" };
  }
  const up = servers.filter((s) => s.status.toUpperCase().startsWith("UP")).length;
  const down = servers.filter((s) => s.status.toUpperCase().startsWith("DOWN")).length;
  if (up === 0 && down === 0) return { role: "muted", label: "not health-checked" };
  if (down === 0) return { role: "good", label: `${up}/${servers.length} up` };
  if (up === 0) return { role: "critical", label: `0/${servers.length} up` };
  return { role: "warning", label: `${up}/${servers.length} up` };
}

function HealthDot({ role, label }: { role: StatusRole; label: string }) {
  return (
    <span
      role="img"
      aria-label={`Backend health: ${label}`}
      title={label}
      className="inline-block h-2.5 w-2.5 shrink-0 rounded-full"
      style={{ backgroundColor: ROLE_COLOR[role] }}
    />
  );
}

// A tag for a config term, shown exactly as it's written in the config.
function Term({ children, title }: { children: React.ReactNode; title?: string }) {
  return (
    <span title={title} className="rounded-full bg-surface-1 px-2 py-0.5 font-mono text-[11px] font-semibold text-text-secondary">
      {children}
    </span>
  );
}

function probeLabel(probe: HAProxyReport["probe"]) {
  switch (probe.kind) {
    case "monitor-uri":
      return `monitor-uri ${probe.path} on :${probe.port} (${probe.section})`;
    case "stats":
      return `stats page ${probe.path} on :${probe.port} (${probe.section})`;
    case "port":
      return `port ${probe.port} reachability only (${probe.section})`;
    default:
      return "no bind found to probe";
  }
}

// What a frontend route matches, the way Nginx's panel shows a server
// block's names: its hosts, else its raw condition, else "anything else".
function routeMatch(r: HAProxyRoute) {
  if (r.default) return "any other request";
  if (r.hosts?.length) return r.hosts.join(" ");
  return r.condition ?? "every request";
}

// The change that turns on a stats page: a listen section appended to the
// main config file, bound to loopback only (the panel reads it from inside
// the container, so it never needs to be reachable from outside) with its
// own "mode http", since HAProxy silently ignores "stats" in TCP mode. The
// port and section name skip any already used in the config.
function statsPageChange(report: HAProxyReport): HAProxyEditorMode | null {
  const file = report.files.find((f) => f.path === report.configPaths[0]) ?? report.files[0];
  if (!file) return null;
  const used = new Set(
    report.sections.flatMap((s) => (s.binds ?? []).map((b) => Number(/:(\d+)/.exec(b)?.[1]))).filter((p) => !Number.isNaN(p)),
  );
  let port = 8404;
  while (used.has(port)) port++;
  const name = report.sections.some((s) => s.name === "stats") ? "panel_stats" : "stats";
  const block = `\nlisten ${name}\n    bind 127.0.0.1:${port}\n    mode http\n    stats enable\n    stats uri /haproxy?stats\n`;
  return {
    kind: "file",
    file,
    proposed: { content: file.content.replace(/\s*$/, "\n") + block, summary: `Enable the stats page in ${file.path}` },
  };
}

function sectionKey(s: HAProxySection, i: number) {
  return `${s.kind}:${s.name ?? ""}:${s.file ?? ""}:${s.line ?? i}`;
}

// The HAProxy config tab, laid out like the Nginx one: the running
// configuration as HAProxy itself resolves it, live backend health, and
// guided, tested changes to it.
export function HAProxyManager({
  serviceId,
  hasSavedPassword,
  sshLabel,
}: {
  serviceId: string;
  hasSavedPassword: boolean;
  sshLabel: string;
}) {
  const [report, setReport] = useState<HAProxyReport | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const liveArrived = useRef(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [needPassword, setNeedPassword] = useState(!hasSavedPassword);
  const [password, setPassword] = useState("");
  const [editor, setEditor] = useState<HAProxyEditorMode | null>(null);
  const [filter, setFilter] = useState("");
  const [showErrors, setShowErrors] = useState(false);
  const editorRef = useRef<HTMLDivElement | null>(null);

  const inspect = useCallback(
    async (pw: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`/api/services/${serviceId}/haproxy/inspect`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ password: pw }),
        });
        if (res.status === 401) {
          setError("Your session has expired. Sign in again.");
          return;
        }
        if (!res.ok) {
          const text = (await res.text()).trim();
          if ((text.includes("no password supplied") || text.includes("no SSH password is saved"))) {
            setNeedPassword(true);
            return;
          }
          setError(text.startsWith("<") ? `The panel answered with HTTP ${res.status}.` : text);
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

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const saved = await loadSnapshot<HAProxyReport>(serviceId, "haproxy");
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

  function openEditor(mode: HAProxyEditorMode) {
    setEditor(mode);
    setTimeout(() => editorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
  }

  const passwordPrompt = (
    <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5">
      <label className="flex flex-1 flex-col gap-1.5 text-sm font-medium text-text-secondary">
        SSH password for {sshLabel}
        <input id="haproxy-password" type="password" className="field" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="off" />
      </label>
      <Button onClick={() => inspect(password)} disabled={!password || loading}>
        {loading ? "Reading…" : "Read HAProxy config"}
      </Button>
    </div>
  );

  if (needPassword && !report) return passwordPrompt;
  if (!report) {
    return (
      <div className="card flex flex-col items-start gap-3 p-5">
        {loading ? (
          <p className="text-sm text-text-secondary">Connecting to the server and reading the running HAProxy configuration…</p>
        ) : (
          <>
            <p className="text-sm text-status-critical">{error ?? "Not read yet."}</p>
            <Button variant="secondary" onClick={() => inspect(password)}>
              Try again
            </Button>
          </>
        )}
      </div>
    );
  }

  // Older saved readings predate some fields; treat missing as empty.
  const stale = savedAt !== null;
  const sections = report.sections ?? [];
  const live = report.backendServers ?? [];
  const allFrontends = sections.filter((s) => s.kind === "frontend" || s.kind === "listen");
  const allBackends = sections.filter((s) => s.kind === "backend");
  const settingSections = sections.filter((s) => (s.kind === "global" || s.kind === "defaults") && (s.settings?.length ?? 0) > 0);
  const others = sections.filter((s) => !["global", "defaults", "frontend", "listen", "backend"].includes(s.kind));
  const healthOf = (s: HAProxySection) => backendHealth(live.filter((b) => b.backend === s.name), report.hasStatsPage);
  const unhealthy = allBackends.map((s) => ({ s, h: healthOf(s) })).filter(({ h }) => h.role === "critical" || h.role === "warning");
  const serverCount = allBackends.reduce((n, b) => n + (b.servers?.length ?? 0), 0);
  const downCount = live.filter((b) => b.status.toUpperCase().startsWith("DOWN")).length;
  const tlsCount = allFrontends.filter((s) => s.tls).length;
  const fileOf = (p?: string) => report.files.find((f) => f.path === p);
  const statsChange = report.hasStatsPage ? null : statsPageChange(report);

  const q = filter.trim().toLowerCase();
  const matches = (values: (string | undefined)[]) => values.some((v) => v?.toLowerCase().includes(q));
  const frontends = !q
    ? allFrontends
    : allFrontends.filter((s) => matches([s.name, s.file, ...(s.binds ?? []), ...(s.routes ?? []).flatMap((r) => [r.backend, ...(r.hosts ?? [])])]));
  const backends = !q ? allBackends : allBackends.filter((s) => matches([s.name, s.file, ...(s.servers ?? [])]));

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
                <span className="font-semibold">Couldn&apos;t refresh:</span> {error}. Showing the reading saved at {new Date(savedAt).toLocaleString()}.
              </>
            ) : (
              <>
                Showing the reading saved at {new Date(savedAt).toLocaleString()}. <span className="font-semibold">{loading ? "Refreshing…" : ""}</span>{" "}
                Changes open once it&apos;s up to date.
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
              {report.version ? `HAProxy ${report.version}` : "HAProxy"} <span className="font-normal text-text-muted">· {report.container}</span>
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusBadge role={report.testOk ? "good" : "critical"} label={report.testOk ? "haproxy -c passes" : "haproxy -c fails"} />
              <StatusBadge role={report.healthy ? "good" : "critical"} label={report.healthy ? "health probe passes" : "health probe fails"} />
              {report.stats.cpu && <StatusBadge role="muted" label={`CPU ${report.stats.cpu}`} />}
              {report.stats.memory && <StatusBadge role="muted" label={`Memory ${report.stats.memory}`} />}
              {report.stats.pids && <StatusBadge role="muted" label={`${report.stats.pids} processes`} />}
            </div>
          </div>
          <Button variant="secondary" onClick={() => inspect(password)} disabled={loading}>
            {loading ? "Reading…" : "Read again"}
          </Button>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatTile hero label="Frontends" value={String(allFrontends.length)} hint={`${tlsCount} with HTTPS`} />
          <StatTile
            label="Backends"
            value={String(allBackends.length)}
            hint={report.hasStatsPage ? `${serverCount} servers, ${downCount} down` : `${serverCount} servers`}
          />
          <StatTile label="Config files" value={String(report.files.length)} hint="as the container runs with -f" />
          <StatTile label="Recent errors" value={String(report.recentErrors.length)} hint="in the last 400 log lines" />
        </div>
      </section>

      {(!report.testOk || report.recentErrors.length > 0 || unhealthy.length > 0 || statsChange) && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">Needs attention</h2>
          {!report.testOk && (
            <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <p className="text-sm font-bold text-status-critical">The configuration on disk fails haproxy -c</p>
              <p className="text-sm text-text-secondary">
                HAProxy keeps serving what it last loaded, but a reload or restart would fail. Fix the file named below.
              </p>
              <pre className="log-panel max-h-48 overflow-auto whitespace-pre-wrap p-3">{report.testOutput}</pre>
            </div>
          )}
          {unhealthy.map(({ s, h }) => (
            <div key={`unhealthy:${s.name}`} className="flex flex-col gap-1 rounded-2xl bg-surface-2 px-4 py-3">
              <p className="flex items-center gap-2 text-sm font-bold text-text-primary">
                <HealthDot role={h.role} label={h.label} />
                <span className="font-mono">{s.name}</span>
                <span className={h.role === "critical" ? "text-status-critical" : "text-status-warning"}>{h.label}</span>
              </p>
              {live
                .filter((b) => b.backend === s.name && b.status.toUpperCase().startsWith("DOWN"))
                .map((b) => (
                  <p key={b.name} className="break-all font-mono text-xs text-text-secondary">
                    {b.name} is down{b.lastCheck ? ` — ${b.lastCheck}` : b.checkDesc ? ` — ${b.checkDesc}` : ""}
                  </p>
                ))}
            </div>
          ))}
          {statsChange && (
            <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl bg-surface-2 px-4 py-3">
              <p className="min-w-0 flex-1 text-sm text-text-secondary">
                <span className="font-bold text-status-warning">No stats page is enabled,</span> so backend health can&apos;t be
                shown — HAProxy only reports whether each server is up there.
              </p>
              <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => openEditor(statsChange)} disabled={stale}>
                Enable stats page
              </Button>
            </div>
          )}
          {report.recentErrors.length > 0 && (
            <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <button type="button" className="text-left text-sm font-bold text-status-warning" onClick={() => setShowErrors((v) => !v)} aria-expanded={showErrors}>
                {report.recentErrors.length} recent error{report.recentErrors.length === 1 ? "" : "s"} in the HAProxy log {showErrors ? "▾" : "▸"}
              </button>
              {showErrors && <pre className="log-panel max-h-64 overflow-auto whitespace-pre-wrap p-3">{report.recentErrors.join("\n")}</pre>}
            </div>
          )}
        </section>
      )}

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Health probe</h2>
        <p className="text-sm text-text-secondary">
          What Sync checks, determined from the config itself: a <span className="font-mono">monitor-uri</span> is
          preferred (it answers with no backend involved), then the stats page, then whether a bound port accepts
          connections at all.
        </p>
        <div className={`rounded-2xl px-4 py-3 text-sm ${report.healthy ? "bg-surface-2" : "bg-[color-mix(in_srgb,var(--status-critical)_12%,transparent)]"}`}>
          <p className="font-mono text-xs text-text-secondary">{probeLabel(report.probe)}</p>
          <p className={`mt-1 font-semibold ${report.healthy ? "text-status-good" : "text-status-critical"}`}>{report.healthDetail}</p>
        </div>
      </section>

      {(allFrontends.length > 3 || allBackends.length > 3) && (
        <input
          id="haproxy-filter"
          type="search"
          className="field sticky top-[calc(3.75rem+env(safe-area-inset-top,0px))] z-20 shadow-card"
          placeholder={`Search ${allFrontends.length} frontends and ${allBackends.length} backends by domain, port or server`}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      )}

      <div ref={editorRef}>
        {editor && report && (
          <HAProxyChangeEditor
            key={
              editor.kind === "site" || editor.kind === "newfile"
                ? editor.kind
                : `${editor.kind}:${editor.file.path}${editor.kind === "file" && editor.proposed ? ":proposed" : ""}`
            }
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
          <h2 className="section-title">Frontends</h2>
          <Button onClick={() => openEditor({ kind: "site" })} disabled={stale}>
            + New site
          </Button>
        </div>
        {frontends.length === 0 && <p className="text-sm text-text-muted">{q ? "No frontend matches." : "No frontends."}</p>}
        <ul className="flex flex-col gap-1.5">
          {frontends.map((s, i) => {
            const file = fileOf(s.file);
            const routes = s.routes ?? [];
            return (
              <li key={sectionKey(s, i)} className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="break-all font-mono text-sm font-bold text-text-primary">
                    {s.kind === "listen" ? "listen " : ""}
                    {s.name}
                  </span>
                  {s.tls && <StatusBadge role="good" label="HTTPS" />}
                  {s.mode === "tcp" && <Term>mode tcp</Term>}
                  {s.statsEnabled && <Term>stats {s.statsUri || "/haproxy?stats"}</Term>}
                  {s.monitorUri && <Term>monitor-uri {s.monitorUri}</Term>}
                  <span className="ml-auto font-mono text-xs text-text-muted">bind {(s.binds ?? []).join(", ") || "none"}</span>
                </div>
                {routes.length > 0 && (
                  <ul className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
                    {routes.map((r, j) => (
                      <li key={j} className="break-all font-mono text-xs text-text-secondary">
                        <span className={r.default ? "text-text-muted" : "text-text-primary"}>{routeMatch(r)}</span> → {r.backend}
                      </li>
                    ))}
                  </ul>
                )}
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="break-all font-mono text-[11px] text-text-muted">
                    {s.file ? `${s.file}:${s.line}` : ""}
                    {(s.certificates ?? []).length > 0 && ` · cert ${(s.certificates ?? []).join(", ")}`}
                  </span>
                  {file && (
                    <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => openEditor({ kind: "file", file })} disabled={stale}>
                      Edit file
                    </Button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Backends</h2>
        {backends.length === 0 && (
          <p className="text-sm text-text-muted">{q ? "No backend matches." : "No backends. A new site creates one."}</p>
        )}
        <ul className="flex flex-col gap-1.5">
          {backends.map((s, i) => {
            const servers = live.filter((b) => b.backend === s.name);
            const byName = new Map(servers.map((b) => [b.name, b]));
            const health = healthOf(s);
            return (
              <li key={sectionKey(s, i)} className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <HealthDot role={health.role} label={health.label} />
                  <span className="font-mono text-sm font-bold text-text-primary">{s.name}</span>
                  <span className="text-xs text-text-muted">{health.label}</span>
                  {(s.servers?.length ?? 0) > 0 && <Term>{s.balance || "roundrobin"}</Term>}
                  {s.healthCheck && <Term>{s.healthCheck}</Term>}
                  {s.redirect && <Term>redirect {s.redirect}</Term>}
                  {s.mode === "tcp" && <Term>mode tcp</Term>}
                  <span className="ml-auto font-mono text-[11px] text-text-muted">{s.file ? `${s.file}:${s.line}` : ""}</span>
                </div>
                {(s.servers?.length ?? 0) > 0 && (
                  <ul className="flex flex-col gap-1">
                    {(s.servers ?? []).map((line) => {
                      const serverLive = byName.get(line.split(/\s+/, 1)[0]);
                      return (
                        <li key={line} className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-xs text-text-secondary">server {line}</span>
                          {serverLive && (
                            <>
                              <StatusBadge role={serverStatusRole(serverLive.status)} label={serverLive.status} />
                              {serverLive.status.toUpperCase().startsWith("DOWN") && serverLive.lastCheck && (
                                <span className="text-xs text-status-critical">{serverLive.lastCheck}</span>
                              )}
                            </>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                )}
              </li>
            );
          })}
        </ul>
      </section>

      {settingSections.length > 0 && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">Global settings</h2>
          {settingSections.map((s, i) => (
            <div key={sectionKey(s, i)} className="flex flex-col gap-2">
              <p className="font-mono text-xs font-bold text-text-primary">
                {s.kind}
                {s.name ? ` ${s.name}` : ""}
              </p>
              <dl className="grid grid-cols-[minmax(9rem,max-content)_minmax(0,1fr)] gap-x-6 gap-y-2 text-sm">
                {(s.settings ?? []).map((st, j) => (
                  <div key={`${st.name}:${j}`} className="contents">
                    <dt className="font-mono text-xs text-text-secondary">{st.name}</dt>
                    <dd className="break-all font-mono text-xs text-text-primary">{st.value || "on"}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </section>
      )}

      {others.length > 0 && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">Other sections</h2>
          <ul className="flex flex-col gap-1.5">
            {others.map((s, i) => (
              <li key={sectionKey(s, i)} className="flex flex-wrap items-center justify-between gap-2 rounded-2xl bg-surface-2 px-4 py-3">
                <span className="font-mono text-sm text-text-primary">
                  {s.kind} {s.name}
                </span>
                <span className="font-mono text-[11px] text-text-muted">{s.file ? `${s.file}:${s.line}` : ""}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="section-title">Config files</h2>
          <Button variant="secondary" onClick={() => openEditor({ kind: "newfile" })} disabled={stale}>
            + New config file
          </Button>
        </div>
        <p className="text-sm text-text-muted">
          Every file this container was started with (<span className="font-mono">{report.configPaths.join(", ")}</span>).
          Edits get a before/after review and a <span className="font-mono">haproxy -c</span> dry run before HAProxy
          reloads.
        </p>
        <ul className="flex flex-col gap-1.5">
          {report.files.map((f) => (
            <li key={f.path} className="flex flex-wrap items-center justify-between gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="min-w-0">
                <p className="break-all font-mono text-sm text-text-primary">{f.path}</p>
                <p className="text-xs text-text-muted">
                  {f.hostPath ? `On the server at ${f.hostPath}` : "Inside the container only"} · {f.content.split("\n").length} lines
                </p>
              </div>
              <div className="flex gap-2">
                <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => openEditor({ kind: "file", file: f })} disabled={stale}>
                  Edit
                </Button>
                {!report.configPaths.includes(f.path) && (
                  <Button variant="danger" className="px-3 py-1 text-xs" onClick={() => openEditor({ kind: "delete", file: f })} disabled={stale}>
                    Remove
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
