// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import type { TfBlock, TfConcept, TfFinding, TfRule, TfStandardView } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const SEVERITY: Record<TfFinding["severity"], { role: StatusRole; label: string }> = {
  error: { role: "critical", label: "enforced" },
  warning: { role: "warning", label: "advice" },
  info: { role: "muted", label: "info" },
};

const CONCEPT_ROLE: Record<TfConcept["status"], StatusRole> = { ok: "good", error: "critical", warning: "warning", unused: "muted" };

function Chip({ children, role = "muted" }: { children: React.ReactNode; role?: StatusRole }) {
  return (
    <span className="rounded-full bg-surface-1 px-2 py-0.5 text-[10px] font-semibold" style={{ color: role === "muted" ? undefined : ROLE_COLOR[role] }}>
      {children}
    </span>
  );
}

function metaChips(b: TfBlock) {
  return (
    <>
      {b.count && <Chip>count</Chip>}
      {b.forEach && <Chip>for_each</Chip>}
      {b.provider && <Chip>provider {b.provider}</Chip>}
      {b.dependsOn && b.dependsOn.length > 0 && <Chip role="warning">depends_on {b.dependsOn.length}</Chip>}
      {b.lifecycle?.preventDestroy && <Chip role="good">prevent_destroy</Chip>}
      {b.lifecycle?.createBeforeDestroy && <Chip>create_before_destroy</Chip>}
      {b.lifecycle?.ignoreChanges && <Chip role={b.lifecycle.ignoreAll ? "warning" : "muted"}>ignore_changes{b.lifecycle.ignoreAll ? " = all" : ""}</Chip>}
      {b.lifecycle?.replaceTriggeredBy && <Chip>replace_triggered_by</Chip>}
      {b.provisioners?.map((p) => (
        <Chip key={p} role="warning">
          provisioner {p}
        </Chip>
      ))}
      {b.dynamic?.map((d) => (
        <Chip key={d}>dynamic {d}</Chip>
      ))}
    </>
  );
}

// Findings is what in the code breaks the standard, enforced rules first.
export function Findings({ findings, onOpen, compact = false }: { findings: TfFinding[]; onOpen?: (file: string, line?: number) => void; compact?: boolean }) {
  if (findings.length === 0) return null;
  return (
    <ul className="flex flex-col gap-1">
      {findings.map((f, i) => (
        <li key={i} className={`flex flex-wrap items-center gap-x-2 gap-y-1 rounded-xl bg-surface-2 px-3 ${compact ? "py-1" : "py-1.5"} text-xs`}>
          <StatusBadge role={SEVERITY[f.severity].role} label={SEVERITY[f.severity].label} />
          <span className="font-mono font-semibold text-text-secondary" title={f.concept}>
            {f.rule}
          </span>
          <span className="min-w-0 flex-1 text-text-primary">{f.message}</span>
          {f.file &&
            (onOpen ? (
              <button type="button" className="font-mono text-accent hover:underline" onClick={() => onOpen(f.file!, f.line)}>
                {f.file}
                {f.line ? `:${f.line}` : ""}
              </button>
            ) : (
              <span className="font-mono text-text-muted">
                {f.file}
                {f.line ? `:${f.line}` : ""}
              </span>
            ))}
        </li>
      ))}
    </ul>
  );
}

function List({ title, empty, children, count }: { title: string; empty?: string; children: React.ReactNode; count: number }) {
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-[11px] font-bold uppercase tracking-[0.08em] text-text-muted">
        {title} <span className="font-normal">({count})</span>
      </h4>
      {count === 0 ? <p className="text-xs text-text-muted">{empty ?? "None."}</p> : <ul className="flex flex-col gap-1">{children}</ul>}
    </div>
  );
}

const ROW = "flex flex-wrap items-center gap-1.5 rounded-xl bg-surface-2 px-3 py-1.5 text-xs";

// TfStandards is a workspace's code against the panel's Terraform
// standard: what breaks it, the code's structure, and the 30 concepts.
export function TfStandards({ std, rules, onOpen }: { std: TfStandardView | null; rules: TfRule[]; onOpen: (file: string, line?: number) => void }) {
  if (!std) return <p className="card p-5 text-sm text-text-muted">Checking the code…</p>;
  const errors = std.findings.filter((f) => f.severity === "error");
  const advice = std.findings.filter((f) => f.severity !== "error");
  const fnCalls = Object.entries(std.functions).sort((a, b) => b[1] - a[1]);

  return (
    <div className="flex flex-col gap-5">
      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="section-title">Standard</h2>
          {std.parseErrors && std.parseErrors.length > 0 ? (
            <StatusBadge role="critical" label="code doesn't parse" />
          ) : std.blocking ? (
            <StatusBadge role="critical" label={`plan blocked: ${errors.length} enforced rule${errors.length === 1 ? "" : "s"} broken`} />
          ) : (
            <StatusBadge role="good" label="meets the standard" />
          )}
          {advice.length > 0 && <span className="text-xs text-text-muted">{advice.length} note{advice.length === 1 ? "" : "s"}</span>}
        </div>
        <p className="text-xs text-text-muted">
          Checked as you edit, and again before every plan: a broken <b>enforced</b> rule blocks the plan (policy as code, like
          Sentinel); <b>advice</b> doesn&apos;t. Formatting (TF010) is checked by the run itself — use Format in the code tab.
        </p>
        {std.parseErrors && std.parseErrors.length > 0 && <Findings findings={std.parseErrors.map((f) => ({ ...f, rule: "parse" }))} onOpen={onOpen} />}
        <Findings findings={std.findings} onOpen={onOpen} />
      </section>

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <h2 className="section-title">Structure</h2>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
          <span>
            required_version: <b className="font-mono text-text-primary">{std.requiredVersion || "—"}</b>
          </span>
          <span>
            backend: <b className="font-mono text-text-primary">{std.backend || "none (the panel keeps the state)"}</b>
          </span>
          <span>
            terraform.workspace: <b className="text-text-primary">{std.usesWorkspace ? "used" : "not used"}</b>
          </span>
          {std.tfvarsFiles && std.tfvarsFiles.length > 0 && (
            <span>
              tfvars: <b className="font-mono text-text-primary">{std.tfvarsFiles.join(", ")}</b>
            </span>
          )}
        </div>
        <div className="grid gap-4 lg:grid-cols-2">
          <List title="Providers" count={std.providers.length}>
            {std.providers.map((p) => (
              <li key={p.name} className={ROW}>
                <span className="font-mono font-semibold text-text-primary">{p.name}</span>
                {p.source && <span className="font-mono text-text-muted">{p.source}</span>}
                {p.version ? <Chip role="good">{p.version}</Chip> : <Chip role="critical">unpinned</Chip>}
                {p.aliases?.map((a) => (
                  <Chip key={a}>alias {a}</Chip>
                ))}
              </li>
            ))}
          </List>
          <List title="Modules" count={std.modules.length}>
            {std.modules.map((m) => (
              <li key={m.address} className={ROW}>
                <button type="button" className="font-mono font-semibold text-text-primary hover:underline" onClick={() => onOpen(m.file, m.line)}>
                  {m.name}
                </button>
                <span className="break-all font-mono text-text-muted">{m.source}</span>
                {m.remote ? <Chip role={m.version || m.source?.includes("ref=") ? "good" : "critical"}>{m.version || (m.source?.includes("ref=") ? "ref pinned" : "remote, unpinned")}</Chip> : <Chip>local</Chip>}
                {metaChips(m)}
              </li>
            ))}
          </List>
          <List title="Resources" count={std.resources.length} empty="No resources yet.">
            {std.resources.map((r) => (
              <li key={r.address} className={ROW}>
                <button type="button" className="font-mono font-semibold text-text-primary hover:underline" onClick={() => onOpen(r.file, r.line)}>
                  {r.address}
                </button>
                {metaChips(r)}
              </li>
            ))}
          </List>
          <List title="Data sources" count={std.dataSources.length}>
            {std.dataSources.map((r) => (
              <li key={r.address} className={ROW}>
                <button type="button" className="font-mono font-semibold text-text-primary hover:underline" onClick={() => onOpen(r.file, r.line)}>
                  {r.address}
                </button>
                {metaChips(r)}
              </li>
            ))}
          </List>
          <List title="Variables" count={std.variables.length}>
            {std.variables.map((v) => (
              <li key={v.name} className={ROW}>
                <button type="button" className="font-mono font-semibold text-text-primary hover:underline" onClick={() => onOpen(v.file, v.line)}>
                  {v.name}
                </button>
                {v.type ? <span className="font-mono text-text-muted">{v.type}</span> : <Chip role="critical">no type</Chip>}
                {v.sensitive && <Chip role="good">sensitive</Chip>}
                {v.hasDefault ? <Chip>default</Chip> : <Chip role="warning">required</Chip>}
                {v.validations ? <Chip>validation {v.validations}</Chip> : null}
                {v.description && <span className="min-w-0 flex-1 truncate text-text-secondary">{v.description}</span>}
              </li>
            ))}
          </List>
          <List title="Outputs" count={std.outputs.length}>
            {std.outputs.map((o) => (
              <li key={o.name} className={ROW}>
                <button type="button" className="font-mono font-semibold text-text-primary hover:underline" onClick={() => onOpen(o.file, o.line)}>
                  {o.name}
                </button>
                {o.sensitive && <Chip role="good">sensitive</Chip>}
                {o.description && <span className="min-w-0 flex-1 truncate text-text-secondary">{o.description}</span>}
              </li>
            ))}
          </List>
          <List title="Local values" count={std.locals.length}>
            <li className={ROW}>
              {std.locals.map((l) => (
                <span key={l} className="font-mono text-text-primary">
                  local.{l}
                </span>
              ))}
            </li>
          </List>
          <List title="Import blocks" count={std.imports.length} empty="None. Add one with Import in the code tab.">
            {std.imports.map((im) => (
              <li key={im.to + im.id} className={ROW}>
                <span className="font-mono font-semibold text-text-primary">{im.to}</span>
                <span className="text-text-muted">←</span>
                <span className="font-mono text-text-secondary">{im.id}</span>
              </li>
            ))}
          </List>
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-text-secondary">
          <span>
            interpolations <b className="text-text-primary">{std.interpolations}</b>
          </span>
          <span>
            conditionals (ternary) <b className="text-text-primary">{std.ternaries}</b>
          </span>
          <span>
            for expressions <b className="text-text-primary">{std.forExpressions}</b>
          </span>
          <span>
            functions{" "}
            <b className="text-text-primary">
              {fnCalls.length === 0 ? "none" : fnCalls.map(([n, c]) => `${n}${c > 1 ? `×${c}` : ""}`).join(", ")}
            </b>
          </span>
          {std.moved ? (
            <span>
              moved/removed blocks <b className="text-text-primary">{std.moved}</b>
            </span>
          ) : null}
        </div>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div>
          <h2 className="section-title">The 30 concepts</h2>
          <p className="mt-1 text-xs text-text-muted">How the panel handles each, and what this workspace&apos;s code does with it.</p>
        </div>
        <ol className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {std.concepts.map((c, i) => (
            <li key={c.name} className="flex flex-col gap-1 rounded-2xl bg-surface-2 p-3">
              <div className="flex items-center gap-2">
                <span className="w-5 text-right font-mono text-[11px] text-text-muted">{i + 1}</span>
                <span className="inline-block h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: ROLE_COLOR[CONCEPT_ROLE[c.status]] }} aria-hidden />
                <span className="text-sm font-bold text-text-primary">{c.name}</span>
                {c.enforced && <span className="ml-auto text-[10px] font-bold uppercase tracking-wide text-accent">enforced</span>}
              </div>
              <p className="pl-7 text-xs text-text-secondary">
                {c.status === "error" ? <b className="text-status-critical">Broken — see the findings above. </b> : null}
                {c.here ? <span className="text-text-primary">{c.here}. </span> : <span className="text-text-muted">Not used here. </span>}
              </p>
              <p className="pl-7 text-[11px] text-text-muted">{c.panel}</p>
              {c.rules && c.rules.length > 0 && <p className="pl-7 font-mono text-[10px] text-text-muted">{c.rules.join(" · ")}</p>}
            </li>
          ))}
        </ol>
      </section>

      {rules.length > 0 && (
        <details className="card p-4 sm:p-5">
          <summary className="cursor-pointer text-sm font-bold text-text-primary">The rules ({rules.length})</summary>
          <ul className="mt-3 flex flex-col gap-1.5">
            {rules.map((r) => (
              <li key={r.id} className="flex flex-col gap-0.5 rounded-xl bg-surface-2 px-3 py-2 text-xs">
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-mono font-bold text-text-primary">{r.id}</span>
                  <StatusBadge role={SEVERITY[r.severity].role} label={SEVERITY[r.severity].label} />
                  <span className="font-semibold text-text-primary">{r.title}</span>
                  <span className="ml-auto text-text-muted">{r.concept}</span>
                </span>
                <span className="text-text-secondary">{r.why}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
