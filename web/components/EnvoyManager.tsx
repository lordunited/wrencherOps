// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { LineDiff } from "@/components/LineDiff";
import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import { YamlEditor } from "@/components/YamlEditor";
import type { EnvoyReport } from "@/lib/api";
import { loadSnapshot } from "@/lib/snapshots";
import { LocalTime } from "@/components/LocalTime";

const LABEL = "flex flex-col gap-1 text-xs text-text-secondary";

function Section({ title, hint, children, action }: { title: string; hint?: string; children: React.ReactNode; action?: React.ReactNode }) {
  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 className="section-title">{title}</h2>
        {hint && <span className="text-xs text-text-muted">{hint}</span>}
        {action && <span className="ml-auto">{action}</span>}
      </div>
      {children}
    </section>
  );
}

function Dot({ ok, title }: { ok: boolean; title?: string }) {
  return <span className="inline-block h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: ROLE_COLOR[ok ? "good" : "critical"] }} title={title} aria-label={title} />;
}

type Mode = "route" | "file";

interface Pending {
  summary: string;
  path: string;
  before: string;
  after: string;
  baseSha: string;
}

// EnvoyManager is an Envoy's tab: its live state (routes, clusters, each
// endpoint's health from the admin API) and its bootstrap config — changed
// by adding a route or editing the file, always validated by Envoy itself
// before it's applied. Applying restarts Envoy, and rolls back if it
// doesn't come up ready.
export function EnvoyManager({ serviceId, hasSavedPassword, sshLabel }: { serviceId: string; hasSavedPassword: boolean; sshLabel: string }) {
  const [report, setReport] = useState<EnvoyReport | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const liveArrived = useRef(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [needPassword, setNeedPassword] = useState(!hasSavedPassword);
  const [password, setPassword] = useState("");
  const [mode, setMode] = useState<Mode | null>(null);
  const [pending, setPending] = useState<Pending | null>(null);
  const [test, setTest] = useState<{ ok: boolean; output: string } | null>(null);
  const [busy, setBusy] = useState<"preview" | "test" | "apply" | null>(null);
  const [applyLog, setApplyLog] = useState<{ ok: boolean; log: string } | null>(null);
  const [route, setRoute] = useState({ domains: "*", pathPrefix: "/", upstreams: "", clusterName: "", healthPath: "", listener: "" });
  const [draft, setDraft] = useState("");
  const editorRef = useRef<HTMLDivElement | null>(null);

  const inspect = useCallback(
    async (pw: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`/api/services/${serviceId}/envoy/inspect`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ password: pw }),
        });
        if (!res.ok) {
          const text = (await res.text()).trim();
          if (text.includes("no SSH password is saved") || text.includes("no password supplied")) {
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
      const saved = await loadSnapshot<EnvoyReport>(serviceId, "envoy");
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

  function open(m: Mode) {
    setMode(m);
    setPending(null);
    setTest(null);
    setApplyLog(null);
    if (m === "file" && report?.files[0]) setDraft(report.files[0].content);
    setTimeout(() => editorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
  }

  async function preview() {
    if (!report?.files[0]) return;
    setBusy("preview");
    setTest(null);
    try {
      const res = await fetch(`/api/services/${serviceId}/envoy/preview`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          file: report.files[0],
          route: {
            domains: route.domains.split(/[\s,]+/).filter(Boolean),
            pathPrefix: route.pathPrefix,
            upstreams: route.upstreams.split(/[\s,]+/).filter(Boolean),
            clusterName: route.clusterName,
            healthPath: route.healthPath,
            listener: route.listener,
          },
        }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const body: { summary: string; changes: { path: string; before: string; after: string; baseSha: string }[] } = await res.json();
      setPending({ summary: body.summary, ...body.changes[0] });
    } finally {
      setBusy(null);
    }
  }

  function fromFile() {
    const f = report?.files[0];
    if (!f) return;
    if (draft === f.content) {
      toast.message("Nothing changed in the file.");
      return;
    }
    setPending({ summary: `Edited ${f.path}`, path: f.path, before: f.content, after: draft, baseSha: f.sha });
    setTest(null);
  }

  async function runTest() {
    if (!pending) return;
    setBusy("test");
    try {
      const res = await fetch(`/api/services/${serviceId}/envoy/test`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ change: { path: pending.path, content: pending.after, baseSha: pending.baseSha } }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      setTest(await res.json());
    } finally {
      setBusy(null);
    }
  }

  async function apply() {
    if (!pending) return;
    setBusy("apply");
    try {
      const res = await fetch(`/api/services/${serviceId}/envoy/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ summary: pending.summary, change: { path: pending.path, content: pending.after, baseSha: pending.baseSha } }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const body: { ok: boolean; log: string; error?: string } = await res.json();
      setApplyLog({ ok: body.ok, log: body.log });
      if (body.ok) {
        toast.success("Applied. Envoy is live with the new config.");
        setPending(null);
        setTest(null);
        setMode(null);
        inspect("");
      } else {
        toast.error(body.error ?? "Applying failed.", { duration: 12000 });
      }
    } finally {
      setBusy(null);
    }
  }

  const passwordPrompt = (
    <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5">
      <label className="flex flex-1 flex-col gap-1.5 text-sm font-medium text-text-secondary">
        SSH password for {sshLabel}
        <input type="password" className="field" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="off" />
      </label>
      <Button onClick={() => inspect(password)} disabled={!password || loading}>
        {loading ? "Reading…" : "Read Envoy"}
      </Button>
    </div>
  );

  if (needPassword && !report) return passwordPrompt;
  if (!report) {
    return (
      <div className="card px-6 py-10 text-center text-sm text-text-muted">
        {error ? <span className="text-status-critical">{error}</span> : loading ? "Reading Envoy…" : "Nothing read yet."}
        {error && (
          <div className="mt-3">
            <Button variant="secondary" onClick={() => inspect("")}>
              Try again
            </Button>
          </div>
        )}
      </div>
    );
  }

  const cfg = report.config;
  const live = new Map(report.liveClusters.map((c) => [c.name, c]));
  const httpListeners = cfg.listeners.filter((l) => l.kind === "http" && !l.rds);
  const dynamicLive = report.liveClusters.filter((c) => c.dynamic);

  return (
    <div className="flex flex-col gap-4">
      {savedAt && (
        <p className="text-xs text-text-muted">
          Showing the reading from <LocalTime value={savedAt} style="ago" />
          {loading ? " — reading Envoy now…" : ""}
        </p>
      )}
      {error && <p className="text-sm text-status-critical">{error}</p>}

      <Section
        title="Envoy"
        hint={`${report.container} · ${report.image}`}
        action={
          <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => inspect(password)} disabled={loading}>
            {loading ? "Reading…" : "Refresh"}
          </Button>
        }
      >
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge role={report.healthy ? "good" : "critical"} label={report.healthy ? "healthy" : "unhealthy"} />
          <span className="text-sm text-text-primary">{report.healthDetail}</span>
        </div>
        <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs">
          <dt className="text-text-muted">Version</dt>
          <dd className="font-mono text-text-secondary">{report.version || "—"}</dd>
          <dt className="text-text-muted">State</dt>
          <dd className="font-mono text-text-secondary">
            {report.state || "—"}
            {report.uptime && ` · up ${report.uptime}`}
          </dd>
          <dt className="text-text-muted">Config</dt>
          <dd className="break-all font-mono text-text-secondary">
            {report.configPath}
            {report.files[0]?.hostPath && <span className="font-sans text-status-good"> · on disk at {report.files[0].hostPath}</span>}
          </dd>
          <dt className="text-text-muted">Admin API</dt>
          <dd className="font-mono text-text-secondary">
            {report.admin || "none in the config"}
            {report.admin && !report.adminReachable && <span className="font-sans text-status-critical"> · not answering</span>}
          </dd>
          <dt className="text-text-muted">Validation</dt>
          <dd className={report.testOk ? "text-status-good" : "text-status-critical"}>
            {report.testOk ? "envoy --mode validate accepts the current config" : "the current config doesn't validate — see below"}
          </dd>
          <dt className="text-text-muted">Resources</dt>
          <dd className="font-mono text-text-secondary">
            CPU {report.stats.cpu || "—"} · Mem {report.stats.memory || "—"} · Net {report.stats.netIO || "—"}
          </dd>
        </dl>
        {cfg.dynamic && <p className="rounded-xl bg-surface-2 px-3 py-2 text-xs text-text-secondary">{cfg.dynamic}</p>}
        {cfg.parseError && <p className="text-xs text-status-critical">The config couldn&apos;t be read as YAML: {cfg.parseError}</p>}
        {!report.testOk && <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-[11px] text-status-critical">{report.testOutput}</pre>}
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => open("route")} disabled={httpListeners.length === 0} title={httpListeners.length === 0 ? "No HTTP listener with routes in this file" : undefined}>
            + Add a route
          </Button>
          <Button variant="secondary" onClick={() => open("file")}>
            Edit envoy.yaml
          </Button>
        </div>
      </Section>

      <Section title="Listeners and routes" hint={`${cfg.listeners.length} listener${cfg.listeners.length === 1 ? "" : "s"}`}>
        {cfg.listeners.length === 0 ? (
          <p className="text-sm text-text-muted">No listeners in the file{cfg.dynamic ? " — they come from the control plane." : "."}</p>
        ) : (
          cfg.listeners.map((l) => (
            <div key={l.name + l.port} className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 p-3">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-bold text-text-primary">{l.name || "(unnamed)"}</span>
                <span className="font-mono text-xs text-text-secondary">
                  {l.address}:{l.port}
                </span>
                <StatusBadge role="muted" label={l.kind.toUpperCase() + (l.tls ? " + TLS" : "")} />
                {l.rds && <span className="text-xs text-text-muted">routes from RDS ({l.rds})</span>}
                {l.tcpCluster && <span className="font-mono text-xs text-text-secondary">→ {l.tcpCluster}</span>}
              </div>
              {l.routes.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[32rem] text-left text-xs">
                    <thead className="text-text-muted">
                      <tr>
                        <th className="py-1 pr-3 font-semibold">Domains</th>
                        <th className="py-1 pr-3 font-semibold">Match</th>
                        <th className="py-1 font-semibold">Goes to</th>
                      </tr>
                    </thead>
                    <tbody className="font-mono text-text-primary">
                      {l.routes.map((r, i) => {
                        const c = r.cluster ? live.get(r.cluster) : undefined;
                        const up = c ? c.endpoints.filter((e) => e.healthy).length : 0;
                        return (
                          <tr key={i} className="border-t border-border">
                            <td className="py-1 pr-3">{(r.domains ?? []).join(", ")}</td>
                            <td className="py-1 pr-3 text-text-secondary">{r.match}</td>
                            <td className="py-1">
                              {r.target}
                              {c && (
                                <span className="ml-2 font-sans text-[11px]" style={{ color: ROLE_COLOR[up === c.endpoints.length && up > 0 ? "good" : up > 0 ? "warning" : "critical"] }}>
                                  {up}/{c.endpoints.length} healthy
                                </span>
                              )}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          ))
        )}
      </Section>

      <Section title="Clusters" hint={report.adminReachable ? "endpoint health live from the admin API" : "admin API not reachable — config only"}>
        {cfg.clusters.length === 0 && dynamicLive.length === 0 ? (
          <p className="text-sm text-text-muted">No clusters yet. Add a route to create one.</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {[...cfg.clusters.map((c) => ({ name: c.name, meta: c })), ...dynamicLive.map((c) => ({ name: c.name, meta: undefined }))].map(({ name, meta }) => {
              const lc = live.get(name);
              return (
                <li key={name} className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 p-3">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-bold text-text-primary">{name}</span>
                    {meta ? (
                      <span className="font-mono text-xs text-text-muted">
                        {meta.type} · {meta.lbPolicy}
                        {meta.tls ? " · TLS" : ""}
                        {meta.healthCheck ? ` · ${meta.healthCheck}` : " · no health check"}
                      </span>
                    ) : (
                      <span className="text-xs text-text-muted">from the control plane</span>
                    )}
                  </div>
                  <ul className="flex flex-col gap-1">
                    {(lc?.endpoints ?? (meta?.endpoints ?? []).map((a) => ({ address: a, healthy: true, status: "", cxActive: 0, rqTotal: 0, rqError: 0, unknown: true }))).map((e) => (
                      <li key={e.address} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 font-mono text-xs">
                        {"unknown" in e ? <span className="inline-block h-2 w-2 rounded-full bg-surface-1" aria-hidden /> : <Dot ok={e.healthy} title={e.status} />}
                        <span className="text-text-primary">{e.address}</span>
                        {!("unknown" in e) && (
                          <>
                            <span className={e.healthy ? "text-text-secondary" : "text-status-critical"}>{e.status}</span>
                            <span className="text-text-muted">
                              {e.cxActive} conn · {e.rqTotal} req · {e.rqError} err
                            </span>
                          </>
                        )}
                      </li>
                    ))}
                  </ul>
                </li>
              );
            })}
          </ul>
        )}
      </Section>

      {report.recentErrors.length > 0 && (
        <Section title="Recent warnings and errors" hint="from its log">
          <pre className="max-h-56 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-[11px] text-text-secondary">
            {report.recentErrors.join("\n")}
          </pre>
        </Section>
      )}

      {mode && (
        <div ref={editorRef}>
          <Section
            title={mode === "route" ? "Add a route" : "Edit envoy.yaml"}
            hint="validated by Envoy before anything is written · applying restarts Envoy"
            action={
              <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setMode(null)} disabled={busy !== null}>
                Close
              </Button>
            }
          >
            {mode === "route" ? (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-6">
                  <label className={`${LABEL} sm:col-span-3`}>
                    Domains (comma or space separated, * for any)
                    <input className="field font-mono" value={route.domains} onChange={(e) => setRoute({ ...route, domains: e.target.value })} />
                  </label>
                  <label className={`${LABEL} sm:col-span-1`}>
                    Path prefix
                    <input className="field font-mono" value={route.pathPrefix} onChange={(e) => setRoute({ ...route, pathPrefix: e.target.value })} />
                  </label>
                  <label className={`${LABEL} sm:col-span-2`}>
                    Cluster name (optional)
                    <input className="field font-mono" value={route.clusterName} onChange={(e) => setRoute({ ...route, clusterName: e.target.value })} placeholder="from the domain" />
                  </label>
                  <label className={`${LABEL} sm:col-span-4`}>
                    Backends (host:port, one or more)
                    <input className="field font-mono" value={route.upstreams} onChange={(e) => setRoute({ ...route, upstreams: e.target.value })} placeholder="web:8080, 10.0.0.5:8080" />
                  </label>
                  <label className={`${LABEL} sm:col-span-2`}>
                    Health check path (optional)
                    <input className="field font-mono" value={route.healthPath} onChange={(e) => setRoute({ ...route, healthPath: e.target.value })} placeholder="/healthz" />
                  </label>
                  {httpListeners.length > 1 && (
                    <label className={`${LABEL} sm:col-span-2`}>
                      Listener
                      <select className="field" value={route.listener} onChange={(e) => setRoute({ ...route, listener: e.target.value })}>
                        <option value="">first HTTP listener</option>
                        {httpListeners.map((l) => (
                          <option key={l.name} value={l.name}>
                            {l.name} (:{l.port})
                          </option>
                        ))}
                      </select>
                    </label>
                  )}
                </div>
                <p className="text-xs text-text-muted">
                  Creates a cluster for the backends (STATIC for IPs, STRICT_DNS for names, round-robin) and a route to it.
                  Domains that match an existing virtual host get the route there, ahead of its catch-all; new domains get
                  their own virtual host.
                </p>
                <Button className="w-fit" onClick={preview} disabled={busy !== null || !route.upstreams.trim()}>
                  {busy === "preview" ? "Preparing…" : "Preview the change"}
                </Button>
              </>
            ) : (
              <>
                <YamlEditor value={draft} onChange={setDraft} language="yaml" readOnly={busy !== null} />
                <Button className="w-fit" onClick={fromFile} disabled={busy !== null}>
                  Review the change
                </Button>
              </>
            )}

            {pending && (
              <div className="flex flex-col gap-3 border-t border-border pt-3">
                <p className="text-sm font-semibold text-text-primary">{pending.summary}</p>
                <LineDiff before={pending.before} after={pending.after} />
                {test && (
                  <pre className={`max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-[11px] ${test.ok ? "text-status-good" : "text-status-critical"}`}>
                    {test.ok ? "✓ Envoy accepts this config.\n\n" : "✗ Envoy rejects this config:\n\n"}
                    {test.output}
                  </pre>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button variant="secondary" onClick={runTest} disabled={busy !== null}>
                    {busy === "test" ? "Validating…" : "Test (validate only)"}
                  </Button>
                  <Button onClick={apply} disabled={busy !== null || (test !== null && !test.ok)}>
                    {busy === "apply" ? "Validating, writing, restarting…" : "Apply"}
                  </Button>
                </div>
                <p className="text-xs text-text-muted">
                  Apply validates again, keeps the previous file under /var/backups/devops-panel, writes the new one and
                  restarts Envoy (open connections are closed). If Envoy isn&apos;t ready within 40 seconds, the previous file
                  is put back and Envoy restarted on it.
                </p>
              </div>
            )}
            {applyLog && (
              <pre className={`max-h-72 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-[11px] ${applyLog.ok ? "text-text-secondary" : "text-status-critical"}`}>
                {applyLog.log}
              </pre>
            )}
          </Section>
        </div>
      )}
    </div>
  );
}
