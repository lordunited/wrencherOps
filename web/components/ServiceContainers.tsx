// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ROLE_COLOR } from "@/components/StatusBadge";
import type { ContainerState, ServiceContainerStats } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const ROLE: Record<ContainerState["role"], { label: string; role: StatusRole }> = {
  running: { label: "running", role: "good" },
  starting: { label: "starting", role: "warning" },
  unhealthy: { label: "unhealthy", role: "critical" },
  paused: { label: "paused", role: "warning" },
  stopped: { label: "stopped", role: "muted" },
  job: { label: "setup job done", role: "muted" },
  failing: { label: "failing", role: "critical" },
};

function roleOf(c: ContainerState) {
  return ROLE[c.role] ?? { label: c.state, role: "muted" as StatusRole };
}

// ContainerSummary is a stack's containers in one line, for lists:
// "2 running · 1 paused · 1 stopped", each with its color.
export function ContainerSummary({ containers }: { containers?: ContainerState[] }) {
  if (!containers || containers.length === 0) return null;
  const counts = new Map<string, number>();
  for (const c of containers) {
    const k = c.role === "starting" ? "running" : c.role;
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  // Only worth showing when something isn't simply running.
  if (counts.size === 1 && counts.has("running")) return null;
  const order: ContainerState["role"][] = ["running", "paused", "stopped", "failing", "unhealthy", "job"];
  return (
    <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-text-secondary">
      {order
        .filter((r) => counts.has(r))
        .map((r) => (
          <span key={r} className="inline-flex items-center gap-1">
            <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: ROLE_COLOR[ROLE[r].role] }} aria-hidden />
            {counts.get(r)} {r === "job" ? "setup job done" : ROLE[r].label}
          </span>
        ))}
    </span>
  );
}

// ServiceContainers is every container of a stack at the last check — each
// one's state (running, paused, stopped, a finished setup job, failing) and
// Docker's own status — with live CPU and memory for the running ones.
export function ServiceContainers({ containers, stats }: { containers?: ContainerState[]; stats?: ServiceContainerStats[] }) {
  const byName = new Map((stats ?? []).map((s) => [s.name, s]));
  // Older checks only recorded running containers' stats.
  const list: ContainerState[] =
    containers && containers.length > 0
      ? containers
      : (stats ?? []).map((s) => ({ name: s.name, service: "", state: "running", status: "", role: "running" as const }));
  if (list.length === 0) return null;

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 className="section-title">Containers</h2>
        <ContainerSummary containers={containers} />
      </div>
      <ul className="flex flex-col gap-1.5">
        {list.map((c) => {
          const r = roleOf(c);
          const s = byName.get(c.name);
          return (
            <li key={c.name} className="flex flex-col gap-1 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                <span className="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-semibold" style={{ backgroundColor: `color-mix(in srgb, ${ROLE_COLOR[r.role]} 14%, transparent)` }}>
                  <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: ROLE_COLOR[r.role] }} aria-hidden />
                  {r.label}
                </span>
                <span className="min-w-0 break-all font-mono font-bold text-text-primary">{c.name}</span>
                {c.service && c.service !== c.name && <span className="text-xs text-text-muted">{c.service}</span>}
                {c.status && <span className="ml-auto font-mono text-xs text-text-secondary">{c.status}</span>}
              </div>
              {c.image && (
                <p className="flex min-w-0 items-baseline gap-1.5 text-xs">
                  <span className="text-text-muted">image</span>
                  <span className="break-all font-mono text-text-secondary">{c.image}</span>
                </p>
              )}
              {s && (
                <div className="flex flex-wrap gap-x-4 font-mono text-xs text-text-secondary">
                  <span>CPU {s.cpu}</span>
                  <span>
                    Mem {s.memory} ({s.memPercent})
                  </span>
                  {s.netIO && <span className="text-text-muted">Net {s.netIO}</span>}
                  <span className="text-text-muted">{s.pids} PIDs</span>
                </div>
              )}
              {c.note && c.role !== "stopped" && <p className="text-xs text-text-muted">{c.note}</p>}
              {c.role === "paused" && <p className="text-xs text-text-muted">Paused: its processes are frozen. Unpause it (docker compose unpause) to resume.</p>}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
