// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import Link from "next/link";
import { AddServerForm } from "@/components/AddServerForm";
import { AutoRefresh } from "@/components/AutoRefresh";
import { RefreshServersButton } from "@/components/RefreshServersButton";
import { ServerMetrics } from "@/components/ServerMetrics";
import { ServersTabs } from "@/components/ServersTabs";
import { StatusBadge } from "@/components/StatusBadge";
import { getWatchedServers } from "@/lib/api";
import { timeAgo } from "@/lib/format";

export default async function ServersPage() {
  const servers = await getWatchedServers();
  const openAlerts = servers.reduce((n, s) => n + s.alerts.length, 0);

  return (
    <div className="flex flex-col gap-5">
      <AutoRefresh seconds={30} />
      <ServersTabs active="servers" alertCount={openAlerts} />
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">Servers</h1>
          <p className="mt-1 text-sm text-text-secondary">
            Every server the panel deploys to, and any you add: who&apos;s logged in, failed logins, disks, kernel errors,
            connections and load — read over SSH every minute.
          </p>
        </div>
        <div className="flex flex-wrap items-start gap-2">
          {servers.length > 0 && <RefreshServersButton />}
          <WriteOnly>
            <AddServerForm />
          </WriteOnly>
        </div>
      </div>

      {servers.length === 0 ? (
        <div className="card px-6 py-12 text-center text-sm text-text-muted">
          No servers yet. Servers you deploy to appear here on their own; add another with its SSH login.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {servers.map((s) => {
            const critical = s.alerts.filter((a) => a.severity === "critical").length;
            const warning = s.alerts.length - critical;
            return (
              <Link key={s.id} href={`/servers/${s.id}`} className="card flex flex-col gap-3 p-4 transition-transform hover:-translate-y-0.5 sm:p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="min-w-0 truncate text-base font-bold text-text-primary">{s.name}</span>
                  {s.name !== s.host && <span className="font-mono text-xs text-text-secondary">{s.host}</span>}
                  <span className="ml-auto flex items-center gap-1.5">
                    {critical > 0 && <StatusBadge role="critical" label={`${critical} critical`} />}
                    {warning > 0 && <StatusBadge role="warning" label={`${warning} warning`} />}
                    {s.alerts.length === 0 && s.snapshot && !s.checkError && <StatusBadge role="good" label="ok" />}
                  </span>
                </div>
                <span className="font-mono text-xs text-text-muted">
                  {s.username}@{s.host}
                  {s.port !== 22 ? `:${s.port}` : ""}
                  {s.snapshot?.os ? ` · ${s.snapshot.os}` : ""} · checked {timeAgo(s.checkedAt)}
                </span>
                {s.checkError && <p className="break-words text-xs text-status-critical">{s.checkError}</p>}
                {s.snapshot ? <ServerMetrics snap={s.snapshot} /> : !s.checkError && <p className="text-xs text-text-muted">Checking…</p>}
                {s.alerts.slice(0, 3).map((a) => (
                  <p key={a.id} className={`text-xs ${a.severity === "critical" ? "text-status-critical" : "text-status-warning"}`}>
                    ● {a.message}
                  </p>
                ))}
                {s.alerts.length > 3 && <p className="text-xs text-text-muted">+{s.alerts.length - 3} more</p>}
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}
