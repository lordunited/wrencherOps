// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { TfRunView } from "@/components/TfRunView";
import { getTfRun } from "@/lib/api";

export default async function TerraformRunPage(props: PageProps<"/terraform/runs/[id]">) {
  const { id } = await props.params;
  const run = await getTfRun(id);
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap gap-4 text-sm">
        <Link href="/terraform/runs" className="text-text-secondary hover:text-text-primary">
          ← Run history
        </Link>
        <Link href={`/terraform/${run.workspaceId}`} className="text-text-secondary hover:text-text-primary">
          Open {run.workspaceName} →
        </Link>
      </div>
      <section className="card p-4 sm:p-5">
        <TfRunView key={run.id} initial={run} workspaceName={run.workspaceName} />
      </section>
    </div>
  );
}
