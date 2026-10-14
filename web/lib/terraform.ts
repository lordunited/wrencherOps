// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { TfRun } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const STATUS_ROLE: Record<TfRun["status"], StatusRole> = { running: "warning", success: "good", failed: "critical", canceled: "muted" };
export const tfStatusRole = (s: TfRun["status"]) => STATUS_ROLE[s] ?? "muted";

export const TF_KIND: Record<TfRun["kind"], string> = { plan: "plan", "destroy-plan": "destroy plan", apply: "apply" };

// TfConceptAction is what a concept's button does in the workspace, for the
// concepts that are something the panel runs rather than code to add.
export type TfConceptAction = "plan" | "destroy-plan" | "apply" | "replace" | "import" | "state" | "variables" | "runs" | "rules";

// CONCEPT_ACTIONS are the run-side concepts' buttons. The code-side ones
// add an example instead (the backend's Snippets); Providers picks one.
// write: the action changes something, so a viewer doesn't get it.
export const CONCEPT_ACTIONS: Record<string, { label: string; action: TfConceptAction; write: boolean }> = {
  "State File": { label: "Open the state", action: "state", write: false },
  Init: { label: "Plan (it runs init first)", action: "plan", write: true },
  Plan: { label: "Plan", action: "plan", write: true },
  Apply: { label: "Apply the reviewed plan", action: "apply", write: true },
  Destroy: { label: "Plan destroy", action: "destroy-plan", write: true },
  Taint: { label: "Choose resources to replace", action: "replace", write: true },
  Import: { label: "Add an import block", action: "import", write: true },
  Workflows: { label: "Start: plan", action: "plan", write: true },
  "TFC/TFE": { label: "Open the run history", action: "runs", write: false },
  Sentinel: { label: "Show the rules", action: "rules", write: false },
  "Workspace Variables": { label: "Open the variables", action: "variables", write: false },
};

// isImportAddress is whether to can be an import block's "to": a root
// module resource, type.name (e.g. aws_instance.web).
export function isImportAddress(to: string): boolean {
  return /^[A-Za-z][A-Za-z0-9_-]*\.[A-Za-z_][A-Za-z0-9_-]*$/.test(to);
}

// importBlock is an import block for the editor: Terraform's config-driven
// import (plan shows it, apply brings the object under management).
export function importBlock(to: string, id: string): string {
  return `import {\n  to = ${to}\n  id = ${JSON.stringify(id)}\n}\n`;
}
