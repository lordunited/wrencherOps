// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import type { WatchedServer } from "@/lib/api";

// The dashboard's server panel: every server with open alerts gets its own
// card listing them; the rest are shown as one row of "ok" chips.
export function ServersOverview({ servers }: { servers: WatchedServer[] }) {
  if (servers.length === 0) return null;
  const alerting = servers
    .filter((s) => s.alerts.length > 0)
    .sort((a, b) => Number(b.alerts.some((x) => x.severity === "critical")) - Number(a.alerts.some((x) => x.severity === "critical")));
  const fine = servers.filter((s) => s.alerts.length === 0);

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="section-title">Servers</h2>
        <Link href="/servers" className="text-xs font-semibold text-accent hover:underline">
          All servers →
        </Link>
      </div>
      {alerting.length > 0 && (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {alerting.map((s) => {
            const critical = s.alerts.some((a) => a.severity === "critical");
            return (
              <Link
                key={s.id}
                href={`/servers/${s.id}`}
                className="card flex flex-col gap-2 border-l-4 p-4 transition-colors hover:bg-surface-hover"
                style={{ borderLeftColor: ROLE_COLOR[critical ? "critical" : "warning"] }}
              >
                <div className="flex items-center gap-2">
                  <span className="truncate font-bold text-text-primary">{s.name}</span>
                  <span className="ml-auto">
                    <StatusBadge role={critical ? "critical" : "warning"} label={`${s.alerts.length} alert${s.alerts.length === 1 ? "" : "s"}`} />
                  </span>
                </div>
                {s.alerts.slice(0, 4).map((a) => (
                  <p key={a.id} className="text-xs text-text-secondary">
                    <span style={{ color: ROLE_COLOR[a.severity === "critical" ? "critical" : "warning"] }}>●</span> {a.message}
                  </p>
                ))}
                {s.alerts.length > 4 && <p className="text-xs text-text-muted">+{s.alerts.length - 4} more</p>}
              </Link>
            );
          })}
        </div>
      )}
      {fine.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {fine.map((s) => (
            <Link
              key={s.id}
              href={`/servers/${s.id}`}
              className="inline-flex items-center gap-1.5 rounded-full bg-surface-1 px-3 py-1 text-xs font-semibold text-text-primary shadow-card hover:bg-surface-hover"
            >
              <span
                className="inline-block h-1.5 w-1.5 rounded-full"
                style={{ backgroundColor: ROLE_COLOR[s.checkError ? "critical" : s.snapshot ? "good" : "muted"] }}
                aria-hidden
              />
              {s.name}
            </Link>
          ))}
        </div>
      )}
    </section>
  );
}
