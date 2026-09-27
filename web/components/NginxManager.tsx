"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/Button";
import { NginxChangeEditor, type NginxEditorMode } from "@/components/NginxChangeEditor";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";
import type { NginxReport, NginxServer } from "@/lib/api";
import { loadSnapshot } from "@/lib/snapshots";

const MAIN_CONFIG = "/etc/nginx/nginx.conf";

// A tag for a config term, shown exactly as it's written in the config.
function Term({ children }: { children: React.ReactNode }) {
  return <span className="rounded-full bg-surface-1 px-2 py-0.5 font-mono text-[11px] font-semibold text-text-secondary">{children}</span>;
}

function target(s: NginxServer) {
  if (s.return) return `return ${s.return}`;
  if (s.proxyPass) return `→ ${s.proxyPass}`;
  const root = s.locations.find((l) => l.match === "/") ?? s.locations[0];
  if (!root) return "";
  if (root.proxyPass) return `→ ${root.proxyPass}`;
  if (root.return) return `return ${root.return}`;
  if (root.root || root.alias) return `files from ${root.root || root.alias}`;
  return "";
}

// The Nginx config tab: the running configuration as Nginx itself sees it,
// and guided, tested changes to it.
export function NginxManager({
  serviceId,
  hasSavedPassword,
  sshLabel,
}: {
  serviceId: string;
  hasSavedPassword: boolean;
  sshLabel: string;
}) {
  const [report, setReport] = useState<NginxReport | null>(null);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const liveArrived = useRef(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [needPassword, setNeedPassword] = useState(!hasSavedPassword);
  const [password, setPassword] = useState("");
  const [editor, setEditor] = useState<NginxEditorMode | null>(null);
  const [filter, setFilter] = useState("");
  const [showErrors, setShowErrors] = useState(false);
  const editorRef = useRef<HTMLDivElement | null>(null);

  const inspect = useCallback(
    async (pw: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await fetch(`/api/services/${serviceId}/nginx/inspect`, {
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
          if (text.includes("no password supplied")) {
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
      const saved = await loadSnapshot<NginxReport>(serviceId, "nginx");
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

  function openEditor(mode: NginxEditorMode) {
    setEditor(mode);
    setTimeout(() => editorRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
  }

  const passwordPrompt = (
    <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5">
      <label className="flex flex-1 flex-col gap-1.5 text-sm font-medium text-text-secondary">
        SSH password for {sshLabel}
        <input id="nginx-password" type="password" className="field" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="off" />
      </label>
      <Button onClick={() => inspect(password)} disabled={!password || loading}>
        {loading ? "Reading…" : "Read Nginx config"}
      </Button>
    </div>
  );

  if (needPassword && !report) return passwordPrompt;
  if (!report) {
    return (
      <div className="card flex flex-col items-start gap-3 p-5">
        {loading ? (
          <p className="text-sm text-text-secondary">Connecting to the server and reading the running Nginx configuration…</p>
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

  const stale = savedAt !== null;
  const ov = report.overview;
  const q = filter.trim().toLowerCase();
  const servers = !q
    ? ov.servers
    : ov.servers.filter(
        (s) =>
          s.names.some((n) => n.toLowerCase().includes(q)) ||
          s.listen.some((l) => l.includes(q)) ||
          s.file.toLowerCase().includes(q) ||
          s.locations.some((l) => `${l.match} ${l.proxyPass ?? ""} ${l.return ?? ""}`.toLowerCase().includes(q)),
      );
  const upstreams = !q
    ? ov.upstreams
    : ov.upstreams.filter((u) => u.name.toLowerCase().includes(q) || u.servers.some((s) => s.includes(q)));
  const httpsCount = ov.servers.filter((s) => s.ssl).length;
  const g = ov.globals;
  const globals: [string, string | undefined][] = [
    ["worker_processes", g.workerProcesses],
    ["worker_connections", g.workerConnections],
    ["client_max_body_size", g.clientMaxBodySize ?? "1m (default)"],
    ["keepalive_timeout", g.keepaliveTimeout],
    ["server_tokens", g.serverTokens ?? "on (default)"],
    ["gzip", g.gzip ?? "off (default)"],
    ["sendfile", g.sendfile],
    ["error_log", g.errorLog],
    ["log formats", g.logFormats?.join(", ")],
    ["includes", g.includes?.join(", ")],
  ];

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
              {report.version || "Nginx"} <span className="font-normal text-text-muted">· {report.container}</span>
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusBadge role={report.testOk ? "good" : "critical"} label={report.testOk ? "nginx -t passes" : "nginx -t fails"} />
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
          <StatTile hero label="Server blocks" value={String(ov.servers.length)} hint={`${httpsCount} with HTTPS`} />
          <StatTile label="Upstreams" value={String(ov.upstreams.length)} hint={`${ov.upstreams.reduce((n, u) => n + u.servers.length, 0)} backends`} />
          <StatTile label="Config files" value={String(report.files.length)} hint="as nginx -T lists them" />
          <StatTile label="Recent errors" value={String(report.recentErrors.length)} hint="in the last 400 log lines" />
        </div>
      </section>

      {(!report.testOk || report.recentErrors.length > 0 || (ov.parseErrors?.length ?? 0) > 0 || !report.newFileDirOnDisk) && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">Needs attention</h2>
          {!report.testOk && (
            <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <p className="text-sm font-bold text-status-critical">The configuration on disk fails nginx -t</p>
              <p className="text-sm text-text-secondary">
                Nginx keeps serving what it last loaded, but a reload or restart would fail. Fix the file named below.
              </p>
              <pre className="log-panel max-h-48 overflow-auto whitespace-pre-wrap p-3">{report.testOutput}</pre>
            </div>
          )}
          {!report.newFileDirOnDisk && (
            <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-text-secondary">
              <span className="font-bold text-status-warning">{report.newFileDir} is inside the container only.</span> Changes there are lost
              when the container is recreated. Mount it from the server in the compose file to keep them.
            </p>
          )}
          {(ov.parseErrors ?? []).map((e) => (
            <p key={e} className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-text-secondary">
              <span className="font-bold text-status-warning">Couldn&apos;t read the structure of</span> <span className="font-mono">{e}</span>. It still
              appears under Config files and can be edited.
            </p>
          ))}
          {report.recentErrors.length > 0 && (
            <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
              <button type="button" className="text-left text-sm font-bold text-status-warning" onClick={() => setShowErrors((v) => !v)} aria-expanded={showErrors}>
                {report.recentErrors.length} recent error{report.recentErrors.length === 1 ? "" : "s"} in the Nginx log {showErrors ? "▾" : "▸"}
              </button>
              {showErrors && <pre className="log-panel max-h-64 overflow-auto whitespace-pre-wrap p-3">{report.recentErrors.join("\n")}</pre>}
            </div>
          )}
        </section>
      )}

      {(ov.servers.length > 3 || ov.upstreams.length > 3) && (
        <input
          id="nginx-filter"
          type="search"
          className="field sticky top-[calc(3.75rem+env(safe-area-inset-top,0px))] z-20 shadow-card"
          placeholder={`Search ${ov.servers.length} server blocks and ${ov.upstreams.length} upstreams by domain, port, path or target`}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
      )}

      <div ref={editorRef}>
        {editor && report && (
          <NginxChangeEditor
            key={editor.kind === "server" ? "server" : `${editor.kind}:${editor.file.path}`}
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
          <h2 className="section-title">Server blocks</h2>
          <Button onClick={() => openEditor({ kind: "server" })} disabled={stale}>
            + New server block
          </Button>
        </div>
        {servers.length === 0 && <p className="text-sm text-text-muted">{q ? "No server block matches." : "No server blocks."}</p>}
        <ul className="flex flex-col gap-1.5">
          {servers.map((s) => {
            const file = report.files.find((f) => f.path === s.file);
            return (
              <li key={`${s.file}:${s.line}`} className="flex flex-col gap-2 rounded-2xl bg-surface-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="break-all font-mono text-sm font-bold text-text-primary">{s.names.length ? s.names.join(" ") : "(no server_name)"}</span>
                  {s.context === "stream" && <Term>stream (TCP/UDP)</Term>}
                  {s.ssl && <StatusBadge role="good" label="HTTPS" />}
                  {s.defaultServer && <Term>default_server</Term>}
                  <span className="ml-auto font-mono text-xs text-text-muted">listen {s.listen.join(", ") || "80"}</span>
                </div>
                <p className="break-all font-mono text-xs text-text-secondary">{target(s)}</p>
                {s.locations.length > 1 && (
                  <ul className="flex flex-col gap-0.5 border-l-2 border-border pl-3">
                    {s.locations.map((l, i) => (
                      <li key={i} className="break-all font-mono text-xs text-text-secondary">
                        <span className="text-text-primary">location {l.match}</span>
                        {l.proxyPass && ` → ${l.proxyPass}`}
                        {l.return && ` return ${l.return}`}
                        {(l.root || l.alias) && ` files from ${l.root || l.alias}`}
                      </li>
                    ))}
                  </ul>
                )}
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="break-all font-mono text-[11px] text-text-muted">
                    {s.file}:{s.line}
                    {s.certificate && ` · cert ${s.certificate}`}
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
        <h2 className="section-title">Upstreams</h2>
        {upstreams.length === 0 && (
          <p className="text-sm text-text-muted">{q ? "No upstream matches." : "No upstream groups. A new load balancer server block creates one."}</p>
        )}
        <ul className="flex flex-col gap-1.5">
          {upstreams.map((u) => (
            <li key={`${u.file}:${u.line}`} className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-sm font-bold text-text-primary">{u.name}</span>
                <Term>{u.method}</Term>
                {u.keepalive && <Term>keepalive {u.keepalive}</Term>}
                <span className="ml-auto font-mono text-[11px] text-text-muted">
                  {u.file}:{u.line}
                </span>
              </div>
              <ul className="flex flex-col gap-0.5">
                {u.servers.map((b) => (
                  <li key={b} className="font-mono text-xs text-text-secondary">
                    server {b}
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Global settings</h2>
        <dl className="grid grid-cols-[minmax(9rem,max-content)_minmax(0,1fr)] gap-x-6 gap-y-2 text-sm">
          {globals
            .filter(([, v]) => v)
            .map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="font-mono text-xs text-text-secondary">{k}</dt>
                <dd className="break-all font-mono text-xs text-text-primary">{v}</dd>
              </div>
            ))}
        </dl>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Config files</h2>
        <p className="text-sm text-text-muted">
          Every file Nginx loads, as <span className="font-mono">nginx -T</span> lists them. Edits get a before/after review and an{" "}
          <span className="font-mono">nginx -t</span> check before Nginx reloads.
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
                {f.path !== MAIN_CONFIG && (
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
