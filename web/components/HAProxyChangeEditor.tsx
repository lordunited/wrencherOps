// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { LineDiff } from "@/components/LineDiff";
import { YamlEditor } from "@/components/YamlEditor";
import type { HAProxyReport, HAProxyTestResult, PromApplyResult, PromFile } from "@/lib/api";

// "file" with proposed opens straight on the review step with the panel's
// suggested content (e.g. an added stats section), still going through
// the same Test → Apply gate as a hand edit.
export type HAProxyEditorMode =
  | { kind: "site" }
  | { kind: "newfile" }
  | { kind: "file"; file: PromFile; proposed?: { content: string; summary: string } }
  | { kind: "delete"; file: PromFile };

const SITE_KINDS = [
  { kind: "proxy", label: "Reverse proxy", hint: "Send requests to one app, e.g. app:3000" },
  { kind: "loadbalance", label: "Load balancer", hint: "Spread requests over several servers" },
  { kind: "redirect", label: "Redirect", hint: "Send every request to another address" },
] as const;

interface Change {
  path: string;
  before: string;
  after: string;
  baseSha: string;
  isNew?: boolean;
  delete?: boolean;
}

const LABEL = "flex flex-col gap-1.5 text-sm font-medium text-text-secondary";

// Every HAProxy change goes through the same steps: edit the file, review
// its before/after, then apply (haproxy -c, then a graceful SIGUSR2
// reload).
export function HAProxyChangeEditor({
  serviceId,
  password,
  report,
  mode,
  onClose,
  onApplied,
}: {
  serviceId: string;
  password: string;
  report: HAProxyReport;
  mode: HAProxyEditorMode;
  onClose: () => void;
  onApplied: () => void;
}) {
  const proposed = mode.kind === "file" ? mode.proposed : undefined;
  const [step, setStep] = useState<"edit" | "review" | "done">(mode.kind === "delete" || proposed ? "review" : "edit");
  const [busy, setBusy] = useState(false);
  const [summary, setSummary] = useState(mode.kind === "delete" ? `Remove ${mode.file.path}` : (proposed?.summary ?? ""));
  const [changes, setChanges] = useState<Change[]>(
    mode.kind === "delete"
      ? [{ path: mode.file.path, before: mode.file.content, after: "", baseSha: mode.file.sha, delete: true }]
      : mode.kind === "file" && proposed
        ? [{ path: mode.file.path, before: mode.file.content, after: proposed.content, baseSha: mode.file.sha }]
        : [],
  );
  const [result, setResult] = useState<PromApplyResult | null>(null);
  const [testResult, setTestResult] = useState<HAProxyTestResult | null>(null);
  const [testing, setTesting] = useState(false);

  const [fileText, setFileText] = useState(mode.kind === "file" ? (proposed?.content ?? mode.file.content) : "");
  const [newFilePath, setNewFilePath] = useState(report.configPaths[0] ?? "");

  // site form
  const [kind, setKind] = useState<(typeof SITE_KINDS)[number]["kind"]>("proxy");
  const [names, setNames] = useState("");
  const [listenPort, setListenPort] = useState("80");
  const [upstream, setUpstream] = useState("");
  const [backends, setBackends] = useState("");
  const [balance, setBalance] = useState("");
  const [healthCheckPath, setHealthCheckPath] = useState("");
  const [redirectTo, setRedirectTo] = useState("");
  const [redirectCode, setRedirectCode] = useState("301");
  const [certPath, setCertPath] = useState("");
  const [forceHttps, setForceHttps] = useState(true);
  const [websocket, setWebsocket] = useState(false);
  const [timeout, setTimeout_] = useState("");
  const [hsts, setHsts] = useState(false);
  const [xFrameOptions, setXFrameOptions] = useState("");
  const [noSniff, setNoSniff] = useState(false);
  const [xssProtection, setXssProtection] = useState(false);
  const [referrerPolicy, setReferrerPolicy] = useState("");
  const [csp, setCsp] = useState("");
  const [cspReportOnly, setCspReportOnly] = useState(false);
  const [permissionsPolicy, setPermissionsPolicy] = useState("");

  const title =
    mode.kind === "site"
      ? "New site"
      : mode.kind === "newfile"
        ? "New config file"
        : mode.kind === "file"
          ? `Edit ${mode.file.path}`
          : `Remove ${mode.file.path}`;

  async function previewSite() {
    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/haproxy/preview`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          files: report.files,
          configPaths: report.configPaths,
          site: {
            kind,
            names: names.split(/[\s,]+/).filter(Boolean),
            listenPort: Number(listenPort) || 80,
            upstream: upstream.trim(),
            backends: backends.split("\n"),
            balance,
            healthCheckPath: healthCheckPath.trim(),
            redirectTo: redirectTo.trim(),
            redirectCode: Number(redirectCode),
            certPath: certPath.trim(),
            forceHttps,
            websocket,
            timeout: timeout.trim(),
            hsts,
            xFrameOptions,
            noSniff,
            xssProtection,
            referrerPolicy,
            contentSecurityPolicy: csp.trim(),
            cspReportOnly,
            permissionsPolicy: permissionsPolicy.trim(),
          },
        }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const body: { changes: Change[]; summary: string } = await res.json();
      setChanges(body.changes);
      setSummary(body.summary);
      setTestResult(null);
      setStep("review");
    } finally {
      setBusy(false);
    }
  }

  function review() {
    if (mode.kind === "site") {
      previewSite();
      return;
    }
    if (mode.kind === "file") {
      setChanges([{ path: mode.file.path, before: mode.file.content, after: fileText, baseSha: mode.file.sha }]);
      setSummary(`Edit ${mode.file.path}`);
      setTestResult(null);
      setStep("review");
      return;
    }
    const p = newFilePath.trim();
    if (!p) return;
    setChanges([{ path: p, before: "", after: fileText, baseSha: "", isNew: true }]);
    setSummary(`Add ${p}`);
    setTestResult(null);
    setStep("review");
  }

  async function test() {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await fetch(`/api/services/${serviceId}/haproxy/test`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          password,
          changes: changes.map((c) => ({ path: c.path, content: c.after, baseSha: c.baseSha, delete: !!c.delete })),
        }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      setTestResult(await res.json());
    } finally {
      setTesting(false);
    }
  }

  async function apply() {
    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/haproxy/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          password,
          summary,
          changes: changes.map((c) => ({ path: c.path, content: c.after, baseSha: c.baseSha, delete: !!c.delete })),
        }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const r: PromApplyResult = await res.json();
      setResult(r);
      setStep("done");
      if (r.ok) {
        toast.success("Applied. HAProxy reloaded without dropping connections.");
        onApplied();
      } else {
        toast.error(r.error ?? "The change was not applied.", { duration: 8000 });
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card flex flex-col gap-4 p-4 sm:p-5" aria-label={title}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="section-title break-all">{title}</h2>
        <ol className="flex items-center gap-1.5 text-xs font-semibold" aria-label="Steps">
          {(["edit", "review", "done"] as const).map((s, i) => (
            <li key={s} className={`rounded-full px-2.5 py-1 ${step === s ? "bg-accent text-on-accent" : "bg-surface-2 text-text-muted"}`}>
              {i + 1}. {s === "edit" ? "Change" : s === "review" ? "Review" : "Result"}
            </li>
          ))}
        </ol>
      </div>

      {step === "edit" && mode.kind === "site" && (
        <>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3" role="radiogroup" aria-label="Site type">
            {SITE_KINDS.map((k) => (
              <button
                key={k.kind}
                type="button"
                role="radio"
                aria-checked={kind === k.kind}
                onClick={() => setKind(k.kind)}
                className={`flex flex-col items-start gap-0.5 rounded-2xl px-3 py-2.5 text-left text-sm transition-colors ${
                  kind === k.kind ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary hover:bg-surface-hover"
                }`}
              >
                <span className="font-bold">{k.label}</span>
                <span className={`text-xs ${kind === k.kind ? "text-on-accent/80" : "text-text-muted"}`}>{k.hint}</span>
              </button>
            ))}
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <label className={`${LABEL} sm:col-span-2`}>
              Domains, separated by spaces — leave empty to answer every request no other site claims
              <input id="names" className="field font-mono" value={names} placeholder="app.example.com www.app.example.com" onChange={(e) => setNames(e.target.value)} />
            </label>
            <label className={LABEL}>
              Listen port (inside the container)
              <input id="listen" className="field font-mono" inputMode="numeric" value={listenPort} onChange={(e) => setListenPort(e.target.value)} />
            </label>

            {kind === "proxy" && (
              <label className={`${LABEL} sm:col-span-3`}>
                Proxy to (host:port)
                <input id="upstream" className="field font-mono" value={upstream} placeholder="app:3000" onChange={(e) => setUpstream(e.target.value)} />
              </label>
            )}
            {kind === "loadbalance" && (
              <>
                <label className={`${LABEL} sm:col-span-2`}>
                  Servers, one per line (host:port, optionally weight=N, backup)
                  <textarea
                    id="backends"
                    className="field min-h-28 font-mono"
                    value={backends}
                    placeholder={"10.0.0.5:8080 weight=2\n10.0.0.6:8080\n10.0.0.7:8080 backup"}
                    onChange={(e) => setBackends(e.target.value)}
                  />
                </label>
                <label className={LABEL}>
                  Balancing
                  <select id="balance" className="field" value={balance} onChange={(e) => setBalance(e.target.value)}>
                    <option value="">Round robin</option>
                    <option value="leastconn">Fewest connections (leastconn)</option>
                    <option value="source">Same client, same server (source)</option>
                  </select>
                </label>
              </>
            )}
            {kind !== "redirect" && (
              <label className={`${LABEL} sm:col-span-3`}>
                Health check path (optional) — without one, a server counts as up when it accepts a connection
                <input
                  id="health-check"
                  className="field font-mono"
                  value={healthCheckPath}
                  placeholder="/health"
                  onChange={(e) => setHealthCheckPath(e.target.value)}
                />
              </label>
            )}
            {kind === "redirect" && (
              <>
                <label className={`${LABEL} sm:col-span-2`}>
                  Redirect to (the path is kept)
                  <input id="redirect" className="field font-mono" value={redirectTo} placeholder="https://www.example.com" onChange={(e) => setRedirectTo(e.target.value)} />
                </label>
                <label className={LABEL}>
                  Status code
                  <select id="code" className="field" value={redirectCode} onChange={(e) => setRedirectCode(e.target.value)}>
                    <option value="301">301 permanent</option>
                    <option value="302">302 temporary</option>
                    <option value="308">308 permanent, keeps method</option>
                    <option value="307">307 temporary, keeps method</option>
                  </select>
                </label>
              </>
            )}
          </div>

          <details className="rounded-2xl bg-surface-2 px-4 py-3">
            <summary className="cursor-pointer text-sm font-semibold text-text-primary">HTTPS and tuning (optional)</summary>
            <div className="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
              <label className={`${LABEL} sm:col-span-2`}>
                Certificate — one PEM file with the certificate and its key, as HAProxy reads it (serves the site on 443)
                <input
                  id="cert"
                  className="field font-mono"
                  value={certPath}
                  placeholder="/usr/local/etc/haproxy/certs/app.pem"
                  onChange={(e) => setCertPath(e.target.value)}
                />
              </label>
              {certPath && (
                <>
                  <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                    <input id="force-https" type="checkbox" checked={forceHttps} onChange={(e) => setForceHttps(e.target.checked)} />
                    Send plain HTTP visitors to HTTPS
                  </label>
                  <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                    <input id="hsts" type="checkbox" checked={hsts} onChange={(e) => setHsts(e.target.checked)} />
                    HSTS, with includeSubDomains (tell browsers to only ever use HTTPS for this domain and every subdomain)
                  </label>
                  {hsts && (
                    <p className="text-xs text-status-warning sm:col-span-2">
                      Every subdomain must already work over HTTPS before enabling this — a subdomain with no valid
                      certificate becomes completely unreachable, and browsers that have seen this header won&apos;t
                      let visitors bypass it.
                    </p>
                  )}
                  <p className="text-xs text-text-muted sm:col-span-2">
                    Port 443 must be published in the stack&apos;s compose file for visitors to reach it.
                  </p>
                </>
              )}
              {kind !== "redirect" && (
                <>
                  <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                    <input id="websocket" type="checkbox" checked={websocket} onChange={(e) => setWebsocket(e.target.checked)} />
                    Keep WebSocket connections open (up to an hour idle)
                  </label>
                  <label className={LABEL}>
                    Server timeout
                    <input id="timeout" className="field font-mono" value={timeout} placeholder="e.g. 60s" onChange={(e) => setTimeout_(e.target.value)} />
                  </label>
                </>
              )}
            </div>
          </details>

          <details className="rounded-2xl bg-surface-2 px-4 py-3">
            <summary className="cursor-pointer text-sm font-semibold text-text-primary">Security headers (optional)</summary>
            <div className="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
              <label className={LABEL}>
                X-Frame-Options
                <select id="x-frame-options" className="field" value={xFrameOptions} onChange={(e) => setXFrameOptions(e.target.value)}>
                  <option value="">Off</option>
                  <option value="SAMEORIGIN">SAMEORIGIN</option>
                  <option value="DENY">DENY</option>
                </select>
              </label>
              <label className={LABEL}>
                Referrer-Policy
                <select id="referrer-policy" className="field" value={referrerPolicy} onChange={(e) => setReferrerPolicy(e.target.value)}>
                  <option value="">Off</option>
                  <option value="no-referrer">no-referrer</option>
                  <option value="strict-origin-when-cross-origin">strict-origin-when-cross-origin</option>
                  <option value="same-origin">same-origin</option>
                  <option value="strict-origin">strict-origin</option>
                  <option value="origin-when-cross-origin">origin-when-cross-origin</option>
                  <option value="no-referrer-when-downgrade">no-referrer-when-downgrade</option>
                  <option value="unsafe-url">unsafe-url</option>
                </select>
              </label>
              <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                <input id="no-sniff" type="checkbox" checked={noSniff} onChange={(e) => setNoSniff(e.target.checked)} />
                X-Content-Type-Options: nosniff
              </label>
              <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                <input id="xss-protection" type="checkbox" checked={xssProtection} onChange={(e) => setXssProtection(e.target.checked)} />
                X-XSS-Protection (legacy — modern browsers ignore it in favor of CSP; still helps IE11/old browsers)
              </label>
              <label className={`${LABEL} sm:col-span-2`}>
                Content-Security-Policy
                <input id="csp" className="field font-mono" value={csp} placeholder="default-src 'self'" onChange={(e) => setCsp(e.target.value)} />
              </label>
              {csp && (
                <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                  <input id="csp-report-only" type="checkbox" checked={cspReportOnly} onChange={(e) => setCspReportOnly(e.target.checked)} />
                  Report only — logs violations to the browser console without blocking anything; use this while testing a
                  policy, then turn it off to enforce
                </label>
              )}
              <label className={`${LABEL} sm:col-span-2`}>
                Permissions-Policy
                <input
                  id="permissions-policy"
                  className="field font-mono"
                  value={permissionsPolicy}
                  placeholder="geolocation=(), camera=(), microphone=()"
                  onChange={(e) => setPermissionsPolicy(e.target.value)}
                />
              </label>
            </div>
          </details>

          <p className="text-xs text-text-muted">
            HAProxy serves every site on a port from one frontend, so the site is added as a host rule on the frontend
            that already listens on that port (or a new one if none does), plus a backend of its own. Its servers are
            health-checked, so their status shows under Backends.
          </p>
        </>
      )}

      {step === "edit" && mode.kind === "file" && (
        <>
          {!mode.file.hostPath && (
            <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-status-warning">
              This file only exists inside the container. Edits are lost the next time the container is recreated.
            </p>
          )}
          <YamlEditor language="haproxy" value={fileText} onChange={setFileText} />
        </>
      )}

      {step === "edit" && mode.kind === "newfile" && (
        <>
          <label className={LABEL}>
            Full path (inside the container)
            <input
              id="new-file-path"
              className="field font-mono"
              value={newFilePath}
              placeholder="/usr/local/etc/haproxy/haproxy.cfg"
              onChange={(e) => setNewFilePath(e.target.value)}
            />
          </label>
          <YamlEditor language="haproxy" value={fileText} onChange={setFileText} />
          <p className="text-xs text-text-muted">
            HAProxy only loads it if it&apos;s one of the paths the container was started with (
            <span className="font-mono">{report.configPaths.join(", ")}</span>) — a file, or inside a directory that was
            passed with <span className="font-mono">-f</span>.
          </p>
        </>
      )}

      {step === "edit" && (
        <div className="flex flex-wrap gap-2">
          <Button onClick={review} disabled={busy || (mode.kind === "newfile" && !newFilePath.trim())}>
            {busy ? "Preparing…" : "Review change"}
          </Button>
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
        </div>
      )}

      {step === "review" && (
        <>
          <p className="text-sm font-semibold text-text-primary">{summary}</p>
          {changes.map((c) => (
            <div key={c.path} className="flex flex-col gap-2">
              <p className="break-all font-mono text-xs text-text-secondary">
                {c.path} {c.isNew && <span className="font-sans font-bold text-accent">new file</span>}
                {c.delete && <span className="font-sans font-bold text-status-critical">will be removed</span>}
              </p>
              <LineDiff before={c.before} after={c.after} />
            </div>
          ))}
          <p className="text-xs text-text-muted">
            Test dry-runs the change with <span className="font-mono">haproxy -c</span> against a scratch copy inside
            the container — nothing on the server is touched. Only once that passes does Apply save a backup of each
            file under /var/backups/devops-panel, write the change for real and reload — gracefully, via SIGUSR2 —
            with the old worker draining its existing connections instead of dropping them.
          </p>
          <div className="flex flex-col gap-2">
            <Button variant="secondary" onClick={test} disabled={testing || busy}>
              {testing ? "Testing…" : "Test configuration (haproxy -c)"}
            </Button>
            {testResult && (
              <div className={`rounded-2xl px-4 py-3 text-sm ${testResult.ok ? "bg-surface-2" : "bg-[color-mix(in_srgb,var(--status-critical)_12%,transparent)]"}`}>
                <p className={`font-bold ${testResult.ok ? "text-status-good" : "text-status-critical"}`}>
                  {testResult.ok ? "haproxy -c passes." : "haproxy -c rejects this change."}
                </p>
                {testResult.output && <pre className="log-panel mt-2 max-h-56 overflow-auto whitespace-pre-wrap p-3">{testResult.output}</pre>}
              </div>
            )}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant={mode.kind === "delete" ? "danger" : "primary"}
              onClick={apply}
              disabled={busy || testing || !testResult?.ok}
              title={!testResult?.ok ? "Test the configuration first" : undefined}
            >
              {busy ? "Applying…" : mode.kind === "delete" ? "Remove and reload" : "Apply to server"}
            </Button>
            <Button variant="secondary" onClick={() => (mode.kind === "delete" ? onClose() : setStep("edit"))} disabled={busy}>
              {mode.kind === "delete" ? "Cancel" : "Back"}
            </Button>
          </div>
        </>
      )}

      {step === "done" && result && (
        <>
          <p className={`text-sm font-bold ${result.ok ? "text-status-good" : "text-status-critical"}`}>
            {result.ok ? "Applied and reloaded." : result.error}
          </p>
          <pre className="log-panel max-h-80 overflow-y-auto whitespace-pre-wrap p-4">{result.log}</pre>
          <div className="flex flex-wrap gap-2">
            {result.ok ? (
              <Button onClick={onClose}>Done</Button>
            ) : (
              <>
                {mode.kind !== "delete" && <Button onClick={() => setStep("edit")}>Change and retry</Button>}
                <Button variant="secondary" onClick={onClose}>
                  Close
                </Button>
              </>
            )}
          </div>
        </>
      )}
    </section>
  );
}
