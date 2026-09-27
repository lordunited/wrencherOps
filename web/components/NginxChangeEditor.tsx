"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { LineDiff } from "@/components/LineDiff";
import { YamlEditor } from "@/components/YamlEditor";
import type { NginxReport, PromApplyResult, PromFile } from "@/lib/api";

export type NginxEditorMode = { kind: "server" } | { kind: "file"; file: PromFile } | { kind: "delete"; file: PromFile };

interface Change {
  path: string;
  before: string;
  after: string;
  baseSha: string;
  isNew?: boolean;
  delete?: boolean;
}

const LABEL = "flex flex-col gap-1.5 text-sm font-medium text-text-secondary";

const SERVER_KINDS = [
  { kind: "proxy", label: "Reverse proxy", hint: "Send requests to one app, e.g. http://app:3000" },
  { kind: "loadbalance", label: "Load balancer", hint: "Spread requests over several backends" },
  { kind: "redirect", label: "Redirect", hint: "Send every request to another address" },
] as const;

// Every Nginx change goes through the same steps: describe it, review the
// before/after of each file, then apply (nginx -t, then a graceful reload).
export function NginxChangeEditor({
  serviceId,
  password,
  report,
  mode,
  onClose,
  onApplied,
}: {
  serviceId: string;
  password: string;
  report: NginxReport;
  mode: NginxEditorMode;
  onClose: () => void;
  onApplied: () => void;
}) {
  const [step, setStep] = useState<"edit" | "review" | "done">(mode.kind === "delete" ? "review" : "edit");
  const [busy, setBusy] = useState(false);
  const [summary, setSummary] = useState(mode.kind === "delete" ? `Remove ${mode.file.path}` : "");
  const [changes, setChanges] = useState<Change[]>(
    mode.kind === "delete"
      ? [{ path: mode.file.path, before: mode.file.content, after: "", baseSha: mode.file.sha, delete: true }]
      : [],
  );
  const [result, setResult] = useState<PromApplyResult | null>(null);

  // server form
  const [kind, setKind] = useState<(typeof SERVER_KINDS)[number]["kind"]>("proxy");
  const [names, setNames] = useState("");
  const [listenPort, setListenPort] = useState("80");
  const [upstream, setUpstream] = useState("");
  const [backends, setBackends] = useState("");
  const [method, setMethod] = useState("");
  const [redirectTo, setRedirectTo] = useState("");
  const [redirectCode, setRedirectCode] = useState("301");
  const [certPath, setCertPath] = useState("");
  const [keyPath, setKeyPath] = useState("");
  const [forceHttps, setForceHttps] = useState(true);
  const [websocket, setWebsocket] = useState(false);
  const [maxBody, setMaxBody] = useState("");
  const [timeout, setTimeout_] = useState("");

  // file editor
  const [fileText, setFileText] = useState(mode.kind === "file" ? mode.file.content : "");

  const title =
    mode.kind === "server" ? "New server block" : mode.kind === "file" ? `Edit ${mode.file.path}` : `Remove ${mode.file.path}`;

  async function review() {
    if (mode.kind === "file") {
      setChanges([{ path: mode.file.path, before: mode.file.content, after: fileText, baseSha: mode.file.sha }]);
      setSummary(`Edit ${mode.file.path}`);
      setStep("review");
      return;
    }
    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/nginx/preview`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          newFileDir: report.newFileDir,
          existingPaths: report.files.map((f) => f.path),
          server: {
            kind,
            names: names.split(/[\s,]+/).filter(Boolean),
            listenPort: Number(listenPort) || 80,
            upstream,
            backends: backends.split("\n"),
            method,
            redirectTo,
            redirectCode: Number(redirectCode),
            certPath: certPath.trim(),
            keyPath: keyPath.trim(),
            forceHttps,
            websocket,
            maxBodySize: maxBody.trim(),
            timeout: timeout.trim(),
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
      setStep("review");
    } finally {
      setBusy(false);
    }
  }

  async function apply() {
    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/nginx/apply`, {
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
        toast.success("Applied. Nginx reloaded without dropping connections.");
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

      {step === "edit" && mode.kind === "server" && (
        <>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-3" role="radiogroup" aria-label="Server type">
            {SERVER_KINDS.map((k) => (
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
              Domains (server_name), separated by spaces
              <input id="names" className="field font-mono" value={names} placeholder="app.example.com www.app.example.com" onChange={(e) => setNames(e.target.value)} />
            </label>
            <label className={LABEL}>
              Listen port (inside the container)
              <input id="listen" className="field font-mono" inputMode="numeric" value={listenPort} onChange={(e) => setListenPort(e.target.value)} />
            </label>

            {kind === "proxy" && (
              <label className={`${LABEL} sm:col-span-3`}>
                Proxy to (proxy_pass)
                <input id="upstream" className="field font-mono" value={upstream} placeholder="http://app:3000" onChange={(e) => setUpstream(e.target.value)} />
              </label>
            )}
            {kind === "loadbalance" && (
              <>
                <label className={`${LABEL} sm:col-span-2`}>
                  Backends, one per line (host:port, optionally weight=N, backup)
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
                  <select id="method" className="field" value={method} onChange={(e) => setMethod(e.target.value)}>
                    <option value="">Round robin</option>
                    <option value="least_conn">Fewest connections (least_conn)</option>
                    <option value="ip_hash">Same client, same backend (ip_hash)</option>
                  </select>
                </label>
              </>
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
              <label className={LABEL}>
                Certificate file (ssl_certificate)
                <input id="cert" className="field font-mono" value={certPath} placeholder="/etc/nginx/certs/app.crt" onChange={(e) => setCertPath(e.target.value)} />
              </label>
              <label className={LABEL}>
                Key file (ssl_certificate_key)
                <input id="key" className="field font-mono" value={keyPath} placeholder="/etc/nginx/certs/app.key" onChange={(e) => setKeyPath(e.target.value)} />
              </label>
              {certPath && (
                <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                  <input id="force-https" type="checkbox" checked={forceHttps} onChange={(e) => setForceHttps(e.target.checked)} />
                  Send plain HTTP visitors to HTTPS
                </label>
              )}
              {kind !== "redirect" && (
                <label className="flex items-center gap-2 text-sm text-text-primary sm:col-span-2">
                  <input id="websocket" type="checkbox" checked={websocket} onChange={(e) => setWebsocket(e.target.checked)} />
                  Allow WebSockets (pass Upgrade headers)
                </label>
              )}
              <label className={LABEL}>
                Max request body (client_max_body_size)
                <input id="max-body" className="field font-mono" value={maxBody} placeholder="e.g. 50m" onChange={(e) => setMaxBody(e.target.value)} />
              </label>
              {kind !== "redirect" && (
                <label className={LABEL}>
                  Proxy timeout
                  <input id="timeout" className="field font-mono" value={timeout} placeholder="e.g. 60s" onChange={(e) => setTimeout_(e.target.value)} />
                </label>
              )}
            </div>
          </details>
          <p className="text-xs text-text-muted">
            It&apos;s written as its own file in <span className="font-mono">{report.newFileDir}</span>
            {!report.newFileDirOnDisk && <span className="text-status-warning"> (inside the container only; lost if the container is recreated)</span>}.
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
          <YamlEditor language="nginx" value={fileText} onChange={setFileText} />
        </>
      )}

      {step === "edit" && (
        <div className="flex flex-wrap gap-2">
          <Button onClick={review} disabled={busy}>
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
            Apply saves a backup of each file under /var/backups/devops-panel on the server, writes the change and runs{" "}
            <span className="font-mono">nginx -t</span>. Only if the test passes does Nginx reload, gracefully, without
            dropping connections. If it fails, the previous files are put back and Nginx keeps running as before.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant={mode.kind === "delete" ? "danger" : "primary"} onClick={apply} disabled={busy}>
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
