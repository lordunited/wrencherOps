// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ROLE_COLOR } from "@/components/StatusBadge";
import type { ServerSnapshot } from "@/lib/api";
import { usageRole } from "@/lib/format";
import type { StatusRole } from "@/lib/format";

function Metric({ label, value, role }: { label: string; value: string; role: StatusRole }) {
  return (
    <div className="flex flex-col rounded-xl bg-surface-2 px-2.5 py-1.5">
      <span className="text-[10px] font-semibold uppercase tracking-wide text-text-muted">{label}</span>
      <span className="flex items-center gap-1.5 font-mono text-sm font-bold text-text-primary">
        <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: ROLE_COLOR[role] }} aria-hidden />
        {value}
      </span>
    </div>
  );
}

// The headline figures of a server's last check, each with a status dot.
export function ServerMetrics({ snap }: { snap: ServerSnapshot }) {
  const fullest = (snap.disks ?? []).reduce((m, d) => (d.usedPercent > m.usedPercent ? d : m), { usedPercent: -1, mount: "" });
  const perCpu = snap.load5 / Math.max(snap.cpus, 1);
  return (
    <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-5">
      <Metric
        label={fullest.mount ? `Disk ${fullest.mount}` : "Disk"}
        value={fullest.usedPercent >= 0 ? `${Math.round(fullest.usedPercent)}%` : "—"}
        role={fullest.usedPercent >= 0 ? usageRole(fullest.usedPercent) : "muted"}
      />
      <Metric label="Memory" value={`${Math.round(snap.memory.usedPercent)}%`} role={usageRole(snap.memory.usedPercent, 85, 95)} />
      <Metric label="Load / CPU" value={perCpu.toFixed(2)} role={perCpu >= 2 ? "critical" : perCpu >= 1 ? "warning" : "good"} />
      <Metric
        label="Failed logins /h"
        value={snap.authSource ? String(snap.failed.lastHour) : "—"}
        role={!snap.authSource ? "muted" : snap.failed.lastHour >= 20 ? "critical" : snap.failed.lastHour > 0 ? "warning" : "good"}
      />
      <Metric
        label="Connections"
        value={snap.connections.readable ? String(snap.connections.established) : "—"}
        role={!snap.connections.readable ? "muted" : snap.connections.established >= 1000 ? "warning" : "good"}
      />
    </div>
  );
}
