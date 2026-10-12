// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { StatusBadge } from "@/components/StatusBadge";
import { SummaryChips, TF_KIND, tfStatusRole } from "@/components/TfRunView";
import { getTfRuns } from "@/lib/api";
import { formatDuration, timeAgo } from "@/lib/format";

export default async function TerraformRunsPage() {
  const runs = await getTfRuns();
  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="page-title">Run history</h1>
        <p className="mt-1 text-sm text-text-secondary">Every plan and apply, in every workspace.</p>
      </div>
      {runs.length === 0 ? (
        <div className="card px-6 py-12 text-center text-sm text-text-muted">No runs yet.</div>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {runs.map((r) => (
            <li key={r.id}>
              <Link href={`/terraform/runs/${r.id}`} className="card flex flex-wrap items-center gap-2 px-4 py-3 text-sm hover:bg-surface-hover">
                <StatusBadge role={r.kind === "apply" ? "warning" : r.kind === "destroy-plan" ? "critical" : "muted"} label={TF_KIND[r.kind]} />
                <StatusBadge role={tfStatusRole(r.status)} label={r.status} />
                <span className="font-semibold text-text-primary">{r.workspaceName}</span>
                <SummaryChips summary={r.summary} />
                {r.error && <span className="text-xs text-status-critical">{r.error}</span>}
                <span className="ml-auto text-xs text-text-muted">
                  {timeAgo(r.startedAt)}
                  {r.finishedAt ? ` · ${formatDuration(r.startedAt, r.finishedAt)}` : ""}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
