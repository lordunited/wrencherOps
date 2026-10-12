// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { StatusBadge } from "@/components/StatusBadge";
import type { StatusRole } from "@/lib/format";

const STATE: Record<string, { label: string; role: StatusRole }> = {
  running: { label: "running", role: "good" },
  paused: { label: "paused", role: "warning" },
  exited: { label: "stopped", role: "muted" },
  created: { label: "created, not started", role: "muted" },
  restarting: { label: "restarting", role: "critical" },
  dead: { label: "dead", role: "critical" },
  removing: { label: "removing", role: "muted" },
};

// ComposeStatus shows "docker compose ls" status — "exited(1), paused(1),
// running(1)" — as one colored chip per state, so a paused or stopped part
// of a project stands out.
export function ComposeStatus({ status }: { status: string }) {
  const parts = status
    .split(",")
    .map((p) => /^\s*([a-z]+)\((\d+)\)\s*$/.exec(p))
    .filter((m): m is RegExpExecArray => m !== null);
  if (parts.length === 0) return <StatusBadge role="muted" label={status} />;
  const order = ["running", "paused", "restarting", "dead", "exited", "created", "removing"];
  parts.sort((a, b) => order.indexOf(a[1]) - order.indexOf(b[1]));
  return (
    <span className="flex flex-wrap gap-1">
      {parts.map(([, state, n]) => {
        const s = STATE[state] ?? { label: state, role: "muted" as StatusRole };
        return <StatusBadge key={state} role={s.role} label={`${n} ${s.label}`} />;
      })}
    </span>
  );
}
