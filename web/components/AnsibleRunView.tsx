// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { AnsibleRun } from "@/lib/api";
import { runStatusRole } from "@/lib/ansible";
import { formatDuration } from "@/lib/format";
import { LocalTime } from "@/components/LocalTime";

// AnsibleRunView shows one run — its result per server and its log — and
// follows it live while it runs.
export function AnsibleRunView({ initial, onFinished }: { initial: AnsibleRun; onFinished?: (run: AnsibleRun) => void }) {
  const canWrite = useCanWrite("ansible.playbooks"); // cancelling is the playbooks tab's to do
  const [run, setRun] = useState(initial);
  const logRef = useRef<HTMLPreElement | null>(null);
  const stick = useRef(true);
  const finished = useRef(initial.status !== "running");

  useEffect(() => {
    if (initial.status !== "running") return;
    const t = setInterval(async () => {
      const res = await fetch(`/api/ansible/runs/${initial.id}`, { cache: "no-store" });
      if (!res.ok) return;
      const next: AnsibleRun = await res.json();
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

  async function cancel() {
    const res = await fetch(`/api/ansible/runs/${run.id}/cancel`, { method: "POST" });
    if (!res.ok) toast.error(await res.text());
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <StatusBadge role={run.mode === "check" ? "muted" : "warning"} label={run.mode === "check" ? "dry run" : "apply"} />
        <StatusBadge role={runStatusRole(run.status)} label={run.status} />
        <span className="text-sm font-semibold text-text-primary">{run.playbookName}</span>
        <span className="text-xs text-text-muted">
          on {run.targets.map((t) => t.name).join(", ")} · started <LocalTime value={run.startedAt} style="ago" />
          {run.finishedAt ? ` · took ${formatDuration(run.startedAt, run.finishedAt)}` : ""}
        </span>
        {run.status === "running" && canWrite && (
          <Button variant="danger" className="ml-auto px-3 py-1 text-xs" onClick={cancel}>
            Cancel
          </Button>
        )}
      </div>
      {run.error && <p className="text-sm text-status-critical">{run.error}</p>}
      {run.recap.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[30rem] text-left text-xs">
            <thead className="text-text-muted">
              <tr>
                {["Server", "ok", "changed", "failed", "unreachable", "skipped"].map((h) => (
                  <th key={h} className="py-1 pr-3 font-semibold">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="font-mono">
              {run.recap.map((h) => (
                <tr key={h.host} className="border-t border-border">
                  <td className="py-1 pr-3 font-bold text-text-primary">{h.host}</td>
                  <td className="py-1 pr-3 text-status-good">{h.ok}</td>
                  <td className={`py-1 pr-3 ${h.changed ? "text-status-warning" : "text-text-muted"}`}>{h.changed}</td>
                  <td className={`py-1 pr-3 ${h.failed ? "text-status-critical" : "text-text-muted"}`}>{h.failed}</td>
                  <td className={`py-1 pr-3 ${h.unreachable ? "text-status-critical" : "text-text-muted"}`}>{h.unreachable}</td>
                  <td className="py-1 pr-3 text-text-muted">{h.skipped}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
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
