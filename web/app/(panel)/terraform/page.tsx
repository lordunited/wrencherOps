// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import Link from "next/link";
import { StatusBadge } from "@/components/StatusBadge";
import { TfNewWorkspace } from "@/components/TfNewWorkspace";
import { SummaryChips, TF_KIND, tfStatusRole } from "@/components/TfRunView";
import { getTfTemplates, getTfWorkspaces } from "@/lib/api";
import { timeAgo } from "@/lib/format";

export default async function TerraformPage() {
  const [workspaces, templates] = await Promise.all([getTfWorkspaces(), getTfTemplates()]);
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">Terraform</h1>
          <p className="mt-1 text-sm text-text-secondary">
            Workspaces of Terraform code, planned and applied from the panel: a plan first, then exactly that plan applied. Runs
            use OpenTofu, Terraform&apos;s open-source fork — the same .tf code and providers.
          </p>
        </div>
        <WriteOnly>
          <TfNewWorkspace templates={templates} />
        </WriteOnly>
      </div>
      {workspaces.length === 0 ? (
        <div className="card px-6 py-12 text-center text-sm text-text-muted">No workspaces yet. Upload your .tf code, or start from a template.</div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {workspaces.map((w) => (
            <Link key={w.id} href={`/terraform/${w.id}`} className="card flex flex-col gap-2 p-5 transition-transform hover:-translate-y-0.5">
              <span className="truncate text-base font-bold text-text-primary">{w.name}</span>
              {w.description && <span className="line-clamp-2 text-xs text-text-secondary">{w.description}</span>}
              <span className="text-xs text-text-muted">{w.ownBackend ? `state in its ${w.ownBackend} backend` : w.stateSerial ? `state serial ${w.stateSerial}` : "no state yet"}</span>
              <div className="mt-auto flex flex-wrap items-center gap-2 pt-1">
                {w.lastRun ? (
                  <>
                    <StatusBadge role={tfStatusRole(w.lastRun.status)} label={`${TF_KIND[w.lastRun.kind]} ${w.lastRun.status}`} />
                    {w.lastRun.kind !== "apply" && <SummaryChips summary={w.lastRun.summary} />}
                    <span className="text-xs text-text-muted">{timeAgo(w.lastRun.startedAt)}</span>
                  </>
                ) : (
                  <span className="text-xs text-text-muted">never planned</span>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
