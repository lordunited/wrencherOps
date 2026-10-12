// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { TfWorkspaceEditor } from "@/components/TfWorkspaceEditor";
import { getTfRun, getTfRuns, getTfWorkspace } from "@/lib/api";

export default async function TerraformWorkspacePage(props: PageProps<"/terraform/[id]">) {
  const { id } = await props.params;
  const [workspace, runs] = await Promise.all([getTfWorkspace(id), getTfRuns(id)]);
  // The run shown first comes with its log: one still going, or the latest plan waiting to be applied.
  const first = runs.find((r) => r.status === "running") ?? runs.find((r) => r.kind !== "apply" && r.status === "success" && !r.appliedByRunId);
  if (first) runs[runs.indexOf(first)] = await getTfRun(first.id);
  return <TfWorkspaceEditor key={workspace.id} initial={workspace} initialRuns={runs} />;
}
