// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { AnsibleRun } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const STATUS_ROLE: Record<AnsibleRun["status"], StatusRole> = { running: "warning", success: "good", failed: "critical", canceled: "muted" };

export function runStatusRole(s: AnsibleRun["status"]): StatusRole {
  return STATUS_ROLE[s] ?? "muted";
}
