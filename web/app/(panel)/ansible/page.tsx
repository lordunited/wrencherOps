// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import Link from "next/link";
import { runStatusRole } from "@/lib/ansible";
import { NewPlaybookForm } from "@/components/NewPlaybookForm";
import { StatusBadge } from "@/components/StatusBadge";
import { UploadPlaybook } from "@/components/UploadPlaybook";
import { getAnsibleTemplates, getPlaybooks } from "@/lib/api";
import { timeAgo } from "@/lib/format";

export default async function AnsiblePage() {
  const [playbooks, templates] = await Promise.all([getPlaybooks(), getAnsibleTemplates()]);

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">Ansible</h1>
          <p className="mt-1 text-sm text-text-secondary">
            Playbooks kept in the panel, run on the servers you pick from the Servers list with their saved logins — a dry
            run (--check --diff) first, then for real.
          </p>
        </div>
        <div className="flex flex-wrap items-start gap-2">
          <WriteOnly>
            <UploadPlaybook />
            <NewPlaybookForm templates={templates} />
          </WriteOnly>
        </div>
      </div>
      {playbooks.length === 0 ? (
        <div className="card px-6 py-12 text-center text-sm text-text-muted">No playbooks yet. Upload one, or start one from a template.</div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {playbooks.map((p) => (
            <Link key={p.id} href={`/ansible/${p.id}`} className="card flex flex-col gap-2 p-5 transition-transform hover:-translate-y-0.5">
              <span className="truncate text-base font-bold text-text-primary">{p.name}</span>
              {p.description && <span className="line-clamp-2 text-xs text-text-secondary">{p.description}</span>}
              <div className="mt-auto flex flex-wrap items-center gap-2 pt-1">
                {p.lastRun ? (
                  <>
                    <StatusBadge role={runStatusRole(p.lastRun.status)} label={`${p.lastRun.mode === "check" ? "dry run" : "apply"} ${p.lastRun.status}`} />
                    <span className="text-xs text-text-muted">{timeAgo(p.lastRun.startedAt)}</span>
                  </>
                ) : (
                  <span className="text-xs text-text-muted">never run</span>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
