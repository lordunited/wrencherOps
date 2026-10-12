// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { AnsibleRunView } from "@/components/AnsibleRunView";
import { getAnsibleRun } from "@/lib/api";

export default async function AnsibleRunPage(props: PageProps<"/ansible/runs/[id]">) {
  const { id } = await props.params;
  const run = await getAnsibleRun(id);
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-4 text-sm">
        <Link href="/ansible/runs" className="text-text-secondary hover:text-text-primary">
          ← Run history
        </Link>
        <Link href={`/ansible/${run.playbookId}`} className="text-text-secondary hover:text-text-primary">
          Open the playbook →
        </Link>
      </div>
      {run.extraVars.trim() && (
        <div className="card flex flex-col gap-1 p-4">
          <span className="text-xs font-semibold text-text-secondary">Extra vars</span>
          <pre className="whitespace-pre-wrap font-mono text-xs text-text-primary">{run.extraVars}</pre>
        </div>
      )}
      <section className="card p-4 sm:p-5">
        <AnsibleRunView key={run.id} initial={run} />
      </section>
    </div>
  );
}
