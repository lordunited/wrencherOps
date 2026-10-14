// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { runStatusRole } from "@/lib/ansible";
import { StatusBadge } from "@/components/StatusBadge";
import { getAnsibleRuns } from "@/lib/api";
import { formatDuration, timeAgo } from "@/lib/format";

export default async function AnsibleRunsPage() {
  const runs = await getAnsibleRuns();
  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="page-title">Run history</h1>
        <p className="mt-1 text-sm text-text-secondary">Every playbook run: what ran, where, and how it went.</p>
      </div>
      {runs.length === 0 ? (
        <div className="card px-6 py-12 text-center text-sm text-text-muted">No runs yet.</div>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {runs.map((r) => {
            const changed = r.recap.reduce((n, h) => n + h.changed, 0);
            return (
              <li key={r.id}>
                <Link href={`/ansible/runs/${r.id}`} className="card flex flex-wrap items-center gap-2 px-4 py-3 text-sm hover:bg-surface-hover">
                  <StatusBadge role={r.mode === "check" ? "muted" : "warning"} label={r.mode === "check" ? "dry run" : "apply"} />
                  <StatusBadge role={runStatusRole(r.status)} label={r.status} />
                  <span className="font-semibold text-text-primary">{r.playbookName}</span>
                  <span className="text-xs text-text-secondary">on {r.targets.map((t) => t.name).join(", ")}</span>
                  {r.recap.length > 0 && <span className="text-xs text-text-muted">{changed} change{changed === 1 ? "" : "s"}</span>}
                  {r.error && <span className="text-xs text-status-critical">{r.error}</span>}
                  <span className="ml-auto text-xs text-text-muted">
                    {timeAgo(r.startedAt)}
                    {r.finishedAt ? ` · ${formatDuration(r.startedAt, r.finishedAt)}` : ""}
                  </span>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
