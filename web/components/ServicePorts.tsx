// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import type { PortStatus } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const STATE: Record<PortStatus["state"], { role: StatusRole; label: string }> = {
  open: { role: "good", label: "open" },
  closed: { role: "critical", label: "not reachable" },
  local: { role: "muted", label: "server only" },
  published: { role: "muted", label: "published" },
  stopped: { role: "warning", label: "container stopped" },
};

function mapping(p: PortStatus) {
  return `${p.hostIp ? `${p.hostIp}:` : ""}${p.hostPort} → ${p.containerPort}/${p.protocol}`;
}

// Every port the stack publishes (its compose's "ports:", as Docker applied
// them) from the last health check, and whether the panel could connect to
// each one. Renders nothing until a check has seen published ports.
export function ServicePorts({ ports, host, checkedAt }: { ports?: PortStatus[]; host: string; checkedAt?: string }) {
  if (!ports || ports.length === 0) return null;
  const open = ports.filter((p) => p.state === "open").length;
  const closed = ports.filter((p) => p.state === "closed").length;

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 className="section-title">Ports</h2>
        <span className="text-xs text-text-muted">
          {open} open{closed > 0 ? ` · ${closed} not reachable` : ""} · tried from the panel
          {checkedAt ? ` at ${new Date(checkedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : ""}
        </span>
      </div>
      <ul className="flex flex-col gap-1.5">
        {ports.map((p) => {
          const s = STATE[p.state] ?? STATE.published;
          return (
            <li key={`${p.container}-${p.hostIp}-${p.hostPort}-${p.protocol}`} className="flex flex-col gap-1 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="font-mono text-sm font-bold text-text-primary">{mapping(p)}</span>
                <span className="min-w-0 break-all text-xs text-text-secondary">{p.service}</span>
                <span className="ml-auto flex items-center gap-2">
                  {p.state === "open" && p.protocol === "tcp" && (
                    <a
                      href={`http://${host}:${p.hostPort}`}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs font-semibold text-accent hover:underline"
                    >
                      Open ↗
                    </a>
                  )}
                  <StatusBadge role={s.role} label={s.label} />
                </span>
              </div>
              {p.detail && <p className="text-xs text-text-muted">{p.detail}</p>}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

// PortChips is the compact form for lists: one chip per host port, with
// its state as the dot's color and in the tooltip.
export function PortChips({ ports }: { ports?: PortStatus[] }) {
  if (!ports || ports.length === 0) return null;
  // One chip per host port and protocol, even when several addresses bind it.
  const seen = new Set<string>();
  const unique = ports.filter((p) => {
    const k = `${p.hostPort}/${p.protocol}`;
    if (seen.has(k)) return false;
    seen.add(k);
    return true;
  });
  return (
    <span className="flex flex-wrap items-center gap-1" aria-label="Published ports">
      {unique.map((p) => {
        const s = STATE[p.state] ?? STATE.published;
        return (
          <span
            key={`${p.hostPort}/${p.protocol}`}
            title={`${mapping(p)} · ${p.service} · ${s.label}${p.detail ? ` — ${p.detail}` : ""}`}
            className="inline-flex items-center gap-1 rounded-full bg-surface-1 px-2 py-0.5 font-mono text-[11px] text-text-secondary"
          >
            <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: ROLE_COLOR[s.role] }} aria-hidden />
            {p.hostPort}
            {p.protocol !== "tcp" ? `/${p.protocol}` : ""}
            <span className="sr-only"> {s.label}</span>
          </span>
        );
      })}
    </span>
  );
}
