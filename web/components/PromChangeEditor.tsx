// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { LineDiff } from "@/components/LineDiff";
import { PromQLEditor } from "@/components/PromQLEditor";
import { YamlEditor } from "@/components/YamlEditor";
import type { PromApplyResult, PromFile, PromPreview, PromReport } from "@/lib/api";
import { ALERT_TEMPLATES, JOB_PRESETS, parseTargets, type JobPreset } from "@/lib/promTemplates";

export type EditorMode =
  | { kind: "targets"; job: string }
  | { kind: "job" }
  | { kind: "rule" }
  | { kind: "file"; file: PromFile };

const LABEL = "flex flex-col gap-1.5 text-sm font-medium text-text-secondary";
const NEW_FILE = "__new__";

function fileRef(f: PromFile) {
  return { path: f.path, content: f.content, sha: f.sha };
}

// Every change goes through the same three steps: fill in the form, review
// a before/after of each file, then apply. Nothing touches the server
// until Apply.
export function PromChangeEditor({
  serviceId,
  password,
  report,
  mode,
  onClose,
  onApplied,
}: {
  serviceId: string;
  password: string;
  report: PromReport;
  mode: EditorMode;
  onClose: () => void;
  onApplied: () => void;
}) {
  const [step, setStep] = useState<"edit" | "review" | "done">("edit");
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<PromPreview | null>(null);
  const [result, setResult] = useState<PromApplyResult | null>(null);

  // targets / job
  const [targetsText, setTargetsText] = useState("");
  const [preset, setPreset] = useState<JobPreset>(JOB_PRESETS[0]);
  const [jobName, setJobName] = useState("");
  const [module, setModule] = useState("");
  const knownExporter = report.jobs.find((j) => j.exporter)?.exporter ?? "";
  const [exporter, setExporter] = useState(knownExporter);
  const [interval, setInterval_] = useState("");

  // rule
  const [templateId, setTemplateId] = useState(ALERT_TEMPLATES[0].id);
  const first = ALERT_TEMPLATES[0];
  const [alert, setAlert] = useState(first.alert);
  const [expr, setExpr] = useState(first.expr);
  const [forDur, setForDur] = useState(first.for);
  const [severity, setSeverity] = useState<string>(first.severity);
  const [summary, setSummary] = useState(first.summary);
  const [description, setDescription] = useState(first.description);
  const defaultRuleFile =
    report.ruleFiles.find((f) => f.path.endsWith("/panel-rules.yml"))?.path ?? report.ruleFiles[0]?.path ?? NEW_FILE;
  const [ruleFile, setRuleFile] = useState(defaultRuleFile);
  const [group, setGroup] = useState("");

  // file
  const [fileText, setFileText] = useState(mode.kind === "file" ? mode.file.content : "");

  const groupsInFile = useMemo(
    () => report.ruleGroups.filter((g) => g.file === ruleFile).map((g) => g.name),
    [report.ruleGroups, ruleFile],
  );

  function applyTemplate(id: string) {
    const t = ALERT_TEMPLATES.find((x) => x.id === id);
    if (!t) return;
    setTemplateId(id);
    setAlert(t.alert);
    setExpr(t.expr);
    setForDur(t.for);
    setSeverity(t.severity);
    setSummary(t.summary);
    setDescription(t.description);
  }

  function choosePreset(p: JobPreset) {
    setPreset(p);
    setModule(p.module);
    if (!jobName || JOB_PRESETS.some((x) => defaultName(x) === jobName)) setJobName(defaultName(p));
  }

  function defaultName(p: JobPreset) {
    const base = { exporter: "exporters", "blackbox-http": "blackbox-http", "blackbox-tcp": "blackbox-tcp", "blackbox-ssl": "blackbox-ssl" }[p.kind];
    let name = base;
    for (let i = 2; report.jobs.some((j) => j.name === name); i++) name = `${base}-${i}`;
    return name;
  }

  const title =
    mode.kind === "targets"
      ? `Add targets to ${mode.job}`
      : mode.kind === "job"
        ? "New scrape job"
        : mode.kind === "rule"
          ? "New alert rule"
          : `Edit ${mode.file.path}`;

  async function runPreview() {
    if (mode.kind === "file") {
      setPreview({
        summary: `Edit ${mode.file.path}`,
        changes: [{ path: mode.file.path, before: mode.file.content, after: fileText, baseSha: mode.file.sha, isNew: false }],
      });
      setStep("review");
      return;
    }

    let body: Record<string, unknown> = { config: fileRef(report.config) };
    if (mode.kind === "targets") {
      body = { ...body, kind: "targets", job: mode.job, targets: parseTargets(targetsText) };
    } else if (mode.kind === "job") {
      body = {
        ...body,
        kind: "job",
        newJob: {
          name: jobName || defaultName(preset),
          kind: preset.kind,
          module: preset.kind === "exporter" ? "" : module || preset.module,
          exporterAddress: exporter,
          scrapeInterval: interval.trim(),
          targets: parseTargets(targetsText),
        },
      };
    } else {
      const chosen = report.ruleFiles.find((f) => f.path === ruleFile);
      body = {
        ...body,
        kind: "rule",
        ruleFile: chosen ? fileRef(chosen) : null,
        group: group.trim() || "panel",
        rule: {
          alert,
          expr,
          for: forDur.trim(),
          labels: { severity },
          annotations: { summary, description },
        },
      };
    }

    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/prometheus/preview`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      setPreview(await res.json());
      setStep("review");
    } finally {
      setBusy(false);
    }
  }

  async function apply() {
    if (!preview) return;
    setBusy(true);
    try {
      const res = await fetch(`/api/services/${serviceId}/prometheus/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          password,
          summary: preview.summary,
          changes: preview.changes.map((c) => ({ path: c.path, content: c.after, baseSha: c.baseSha })),
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
        toast.success("Applied. Prometheus reloaded the new configuration.");
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
        <h2 className="section-title">{title}</h2>
        <ol className="flex items-center gap-1.5 text-xs font-semibold" aria-label="Steps">
          {(["edit", "review", "done"] as const).map((s, i) => (
            <li
              key={s}
              className={`rounded-full px-2.5 py-1 ${
                step === s ? "bg-accent text-on-accent" : "bg-surface-2 text-text-muted"
              }`}
            >
              {i + 1}. {s === "edit" ? "Change" : s === "review" ? "Review" : "Result"}
            </li>
          ))}
        </ol>
      </div>

      {step === "edit" && (
        <>
          {mode.kind === "job" && (
            <>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4" role="radiogroup" aria-label="Job type">
                {JOB_PRESETS.map((p) => (
                  <button
                    key={p.kind}
                    type="button"
                    role="radio"
                    aria-checked={preset.kind === p.kind}
                    onClick={() => choosePreset(p)}
                    className={`flex flex-col items-start gap-0.5 rounded-2xl px-3 py-2.5 text-left text-sm transition-colors ${
                      preset.kind === p.kind ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary hover:bg-surface-hover"
                    }`}
                  >
                    <span className="font-bold">{p.label}</span>
                    <span className={`text-xs ${preset.kind === p.kind ? "text-on-accent/80" : "text-text-muted"}`}>{p.hint}</span>
                  </button>
                ))}
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <label className={LABEL}>
                  Job name
                  <input id="job-name" className="field font-mono" value={jobName} placeholder={defaultName(preset)} onChange={(e) => setJobName(e.target.value)} />
                </label>
                <label className={LABEL}>
                  Scrape interval (optional)
                  <input id="job-interval" className="field font-mono" value={interval} placeholder="uses the global setting" onChange={(e) => setInterval_(e.target.value)} />
                </label>
                {preset.kind !== "exporter" && (
                  <>
                    <label className={LABEL}>
                      Blackbox exporter address
                      <input id="job-exporter" className="field font-mono" value={exporter} placeholder="172.22.241.41:9115" onChange={(e) => setExporter(e.target.value)} />
                      {knownExporter && <span className="text-xs text-text-muted">Pre-filled from your existing blackbox jobs.</span>}
                    </label>
                    <label className={LABEL}>
                      Module
                      <input id="job-module" className="field font-mono" value={module || preset.module} onChange={(e) => setModule(e.target.value)} />
                    </label>
                  </>
                )}
              </div>
            </>
          )}

          {(mode.kind === "targets" || mode.kind === "job") && (
            <label className={LABEL}>
              Targets, one per line. Add a note after #.
              <textarea
                id="targets"
                className="field min-h-40 font-mono"
                value={targetsText}
                placeholder={mode.kind === "job" ? preset.placeholder : "172.22.241.34:9100 # vpn node exporter"}
                onChange={(e) => setTargetsText(e.target.value)}
              />
              <span className="text-xs text-text-muted">
                {parseTargets(targetsText).length} target(s). Targets the job already has are skipped.
              </span>
            </label>
          )}

          {mode.kind === "rule" && (
            <>
              <div className="flex flex-wrap gap-2" aria-label="Templates">
                {ALERT_TEMPLATES.map((t) => (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => applyTemplate(t.id)}
                    aria-pressed={templateId === t.id}
                    className={`rounded-full px-3 py-1.5 text-xs font-semibold transition-colors ${
                      templateId === t.id ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary hover:bg-surface-hover"
                    }`}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                <label className={`${LABEL} sm:col-span-2`}>
                  Alert name
                  <input id="rule-alert" className="field font-mono" value={alert} onChange={(e) => setAlert(e.target.value)} />
                </label>
                <label className={LABEL}>
                  Severity
                  <select id="rule-severity" className="field" value={severity} onChange={(e) => setSeverity(e.target.value)}>
                    <option value="critical">critical</option>
                    <option value="warning">warning</option>
                    <option value="info">info</option>
                  </select>
                </label>
                <div className={`${LABEL} sm:col-span-3`}>
                  <label htmlFor="rule-expr">Condition (PromQL). The alert fires while this returns results.</label>
                  <PromQLEditor id="rule-expr" serviceId={serviceId} value={expr} onChange={setExpr} />
                </div>
                <label className={LABEL}>
                  For how long before firing
                  <input id="rule-for" className="field font-mono" value={forDur} placeholder="5m" onChange={(e) => setForDur(e.target.value)} />
                </label>
                <label className={`${LABEL} sm:col-span-2`}>
                  Summary
                  <input id="rule-summary" className="field" value={summary} onChange={(e) => setSummary(e.target.value)} />
                </label>
                <label className={`${LABEL} sm:col-span-3`}>
                  Description
                  <input id="rule-description" className="field" value={description} onChange={(e) => setDescription(e.target.value)} />
                </label>
                <label className={`${LABEL} sm:col-span-2`}>
                  Rule file
                  <select id="rule-file" className="field font-mono" value={ruleFile} onChange={(e) => setRuleFile(e.target.value)}>
                    {report.ruleFiles.map((f) => (
                      <option key={f.path} value={f.path}>
                        {f.path}
                      </option>
                    ))}
                    {!report.ruleFiles.some((f) => f.path.endsWith("/panel-rules.yml")) && (
                      <option value={NEW_FILE}>New file: panel-rules.yml</option>
                    )}
                  </select>
                </label>
                <label className={LABEL}>
                  Group
                  <input id="rule-group" className="field font-mono" list="rule-groups" value={group} placeholder={groupsInFile[0] ?? "panel"} onChange={(e) => setGroup(e.target.value || "")} />
                  <datalist id="rule-groups">
                    {groupsInFile.map((g) => (
                      <option key={g} value={g} />
                    ))}
                  </datalist>
                </label>
              </div>
            </>
          )}

          {mode.kind === "file" && (
            <>
              {!mode.file.hostPath && (
                <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-status-warning">
                  This file only exists inside the container. Edits are lost the next time the container is recreated.
                </p>
              )}
              <YamlEditor value={fileText} onChange={setFileText} />
            </>
          )}

          <div className="flex flex-wrap gap-2">
            <Button onClick={runPreview} disabled={busy}>
              {busy ? "Preparing…" : "Review change"}
            </Button>
            <Button variant="secondary" onClick={onClose} disabled={busy}>
              Cancel
            </Button>
          </div>
        </>
      )}

      {step === "review" && preview && (
        <>
          <p className="text-sm font-semibold text-text-primary">{preview.summary}</p>
          {preview.skipped && preview.skipped.length > 0 && (
            <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-text-secondary">
              Already in the job, skipped: <span className="font-mono">{preview.skipped.join(", ")}</span>
            </p>
          )}
          {preview.changes.map((c) => (
            <div key={c.path} className="flex flex-col gap-2">
              <p className="break-all font-mono text-xs text-text-secondary">
                {c.path} {c.isNew && <span className="font-sans font-bold text-accent">new file</span>}
              </p>
              <LineDiff before={c.before} after={c.after} />
            </div>
          ))}
          <p className="text-xs text-text-muted">
            Apply checks the change with promtool, saves a backup of each file, writes it, and reloads Prometheus
            without restarting it. If Prometheus rejects it, the old files are put back.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button onClick={apply} disabled={busy}>
              {busy ? "Applying…" : "Apply to server"}
            </Button>
            <Button variant="secondary" onClick={() => setStep("edit")} disabled={busy}>
              Back
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
                <Button onClick={() => setStep("edit")}>Change and retry</Button>
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
