// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import { Findings } from "@/components/TfStandards";
import type { TfRun, TfSummary } from "@/lib/api";
import { formatDuration } from "@/lib/format";
import type { StatusRole } from "@/lib/format";
import { TF_KIND, tfStatusRole } from "@/lib/terraform";
import { LocalTime } from "@/components/LocalTime";

const ACTION: Record<string, { sign: string; role: StatusRole; label: string }> = {
  create: { sign: "+", role: "good", label: "create" },
  update: { sign: "~", role: "warning", label: "update in place" },
  delete: { sign: "-", role: "critical", label: "destroy" },
  replace: { sign: "±", role: "serious", label: "replace (destroy, then create)" },
  read: { sign: "<=", role: "muted", label: "read" },
  import: { sign: "→", role: "good", label: "import (bring the existing object under management)" },
};

export function hasChanges(s: TfSummary) {
  return s.add + s.change + s.destroy + s.replace + (s.import ?? 0) > 0;
}

// SummaryChips is a plan's counts: +2 ~1 -0 ±1.
export function SummaryChips({ summary }: { summary: TfSummary }) {
  if (!hasChanges(summary)) return <span className="text-xs text-text-muted">no changes</span>;
  return (
    <span className="inline-flex flex-wrap gap-x-2 font-mono text-xs font-bold">
      {summary.add > 0 && <span style={{ color: ROLE_COLOR.good }}>+{summary.add}</span>}
      {summary.change > 0 && <span style={{ color: ROLE_COLOR.warning }}>~{summary.change}</span>}
      {summary.destroy > 0 && <span style={{ color: ROLE_COLOR.critical }}>-{summary.destroy}</span>}
      {summary.replace > 0 && <span style={{ color: ROLE_COLOR.serious }}>±{summary.replace}</span>}
      {(summary.import ?? 0) > 0 && <span style={{ color: ROLE_COLOR.good }}>→{summary.import}</span>}
    </span>
  );
}

// TfRunView shows one run — what the plan changes, its log — follows it
// live while it runs, and, for a plan that can be applied, applies it.
export function TfRunView({
  initial,
  workspaceName,
  onFinished,
  onApplied,
}: {
  initial: TfRun;
  workspaceName: string;
  onFinished?: (run: TfRun) => void;
  onApplied?: (apply: TfRun) => void;
}) {
  const [run, setRun] = useState(initial);
  const canWrite = useCanWrite("terraform.workspaces"); // applying is the workspaces tab's to do
  const [approve, setApprove] = useState("");
  const [busy, setBusy] = useState(false);
  const logRef = useRef<HTMLPreElement | null>(null);
  const stick = useRef(true);
  const finished = useRef(initial.status !== "running");

  useEffect(() => {
    if (initial.status !== "running") return;
    const t = setInterval(async () => {
      const res = await fetch(`/api/terraform/runs/${initial.id}`, { cache: "no-store" });
      if (!res.ok) return;
      const next: TfRun = await res.json();
      setRun(next);
      if (next.status !== "running" && !finished.current) {
        finished.current = true;
        clearInterval(t);
        onFinished?.(next);
      }
    }, 1500);
    return () => clearInterval(t);
  }, [initial.id, initial.status, onFinished]);

  useEffect(() => {
    const el = logRef.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [run.log]);

  async function apply() {
    setBusy(true);
    try {
      const res = await fetch(`/api/terraform/runs/${run.id}/apply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ approve }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 12000 });
        return;
      }
      const a: TfRun = await res.json();
      setRun({ ...run, appliedByRunId: a.id });
      onApplied?.(a);
    } finally {
      setBusy(false);
    }
  }

  async function cancel() {
    const res = await fetch(`/api/terraform/runs/${run.id}/cancel`, { method: "POST" });
    if (!res.ok) toast.error(await res.text());
    else toast.message("Stopping… OpenTofu is interrupted so it can save what it did.");
  }

  const isPlan = run.kind === "plan" || run.kind === "destroy-plan";
  const canApply = canWrite && isPlan && run.status === "success" && !run.appliedByRunId && hasChanges(run.summary);
  const destroy = run.kind === "destroy-plan";
  // A plan that destroys or replaces anything needs the name typed, like a
  // destroy plan.
  const destructive = destroy || run.summary.destroy + run.summary.replace > 0;
  const advice = (run.checks ?? []).filter((f) => f.severity === "warning");

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge role={run.kind === "apply" ? "warning" : destroy ? "critical" : "muted"} label={TF_KIND[run.kind]} />
        <StatusBadge role={tfStatusRole(run.status)} label={run.status} />
        {run.status !== "running" && run.kind !== "apply" && <SummaryChips summary={run.summary} />}
        <span className="text-xs text-text-muted">
          {run.startedBy ? `${run.startedBy} · ` : ""}started <LocalTime value={run.startedAt} style="ago" />
          {run.finishedAt ? ` · took ${formatDuration(run.startedAt, run.finishedAt)}` : ""}
        </span>
        {run.status === "running" && canWrite && (
          <Button variant="danger" className="ml-auto px-3 py-1 text-xs" onClick={cancel}>
            Cancel
          </Button>
        )}
      </div>
      {run.error && <p className="text-sm text-status-critical">{run.error}</p>}
      {run.options?.replace && run.options.replace.length > 0 && (
        <p className="text-xs text-text-secondary">
          Planned with replace: <span className="font-mono text-text-primary">{run.options.replace.join(", ")}</span>
        </p>
      )}
      {advice.length > 0 && run.kind !== "apply" && (
        <details className="text-xs text-text-secondary">
          <summary className="cursor-pointer">
            {advice.length} note{advice.length === 1 ? "" : "s"} from the standard when planned
          </summary>
          <div className="mt-1">
            <Findings findings={advice} compact />
          </div>
        </details>
      )}

      {run.summary.changes && run.summary.changes.length > 0 && (
        <ul className="flex flex-col gap-0.5 rounded-xl bg-surface-2 p-2 font-mono text-xs">
          {run.summary.changes.map((c) => {
            const a = ACTION[c.action] ?? ACTION.read;
            return (
              <li key={c.address + c.action} className="flex items-center gap-2" title={a.label}>
                <span className="w-5 text-center font-bold" style={{ color: ROLE_COLOR[a.role] }}>
                  {a.sign}
                </span>
                <span className="text-text-primary">{c.address}</span>
                <span className="text-[10px] text-text-muted">{a.label}</span>
              </li>
            );
          })}
        </ul>
      )}

      {canApply && (
        <div className={`flex flex-col gap-2 rounded-2xl p-3 ${destructive ? "border-l-4 bg-surface-2" : "bg-surface-2"}`} style={destructive ? { borderLeftColor: ROLE_COLOR.critical } : undefined}>
          {destructive ? (
            <p className="text-sm text-text-primary">
              <b className="text-status-critical">
                ⚠ This {destroy || run.summary.destroy > 0 ? `destroys ${run.summary.destroy} real resource${run.summary.destroy === 1 ? "" : "s"}` : ""}
                {!destroy && run.summary.destroy > 0 && run.summary.replace > 0 ? " and " : ""}
                {!destroy && run.summary.replace > 0 ? `replaces ${run.summary.replace} (destroyed, then created again)` : ""}.
              </b>{" "}
              To approve, type the workspace&apos;s name: <b className="font-mono">{workspaceName}</b>
            </p>
          ) : (
            <p className="text-sm text-text-primary">
              Apply exactly this plan: the changes above, nothing else. If anything changed since (the code, or the state
              after another apply), it&apos;s refused — plan again.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {destructive && <input className="field max-w-xs font-mono" value={approve} onChange={(e) => setApprove(e.target.value)} autoComplete="off" />}
            <Button variant={destructive ? "danger" : "primary"} onClick={apply} disabled={busy || (destructive && approve.trim() !== workspaceName)}>
              {busy ? "Starting…" : destroy ? "Destroy" : "Apply this plan"}
            </Button>
          </div>
        </div>
      )}
      {isPlan && run.appliedByRunId && <p className="text-xs text-status-good">✓ Applied (run {run.appliedByRunId.slice(0, 8)})</p>}

      <pre
        ref={logRef}
        onScroll={(e) => {
          const el = e.currentTarget;
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
        }}
        className="max-h-[32rem] overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-2 p-3 font-mono text-[11px] text-text-secondary"
      >
        {run.log || (run.status === "running" ? "Starting…" : "(no output)")}
      </pre>
    </div>
  );
}
