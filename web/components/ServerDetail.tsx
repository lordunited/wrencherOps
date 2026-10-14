// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { ServerMetrics } from "@/components/ServerMetrics";
import { ROLE_COLOR, StatusBadge } from "@/components/StatusBadge";
import type { ServerAuthEvent, ServerCount, WatchedServer } from "@/lib/api";
import { bytesLabel, uptimeLabel, usageRole } from "@/lib/format";
import { LocalTime } from "@/components/LocalTime";

const LABEL = "flex flex-col gap-1 text-sm text-text-secondary";

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h2 className="section-title">{title}</h2>
        {hint && <span className="text-xs text-text-muted">{hint}</span>}
      </div>
      {children}
    </section>
  );
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-text-muted">{children}</p>;
}

function Counts({ title, items }: { title: string; items: ServerCount[] | null }) {
  if (!items || items.length === 0) return null;
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs font-semibold text-text-secondary">{title}</span>
      <div className="flex flex-wrap gap-1.5">
        {items.map((c) => (
          <span key={c.key} className="rounded-full bg-surface-2 px-2.5 py-0.5 font-mono text-xs text-text-primary">
            {c.key} <span className="text-text-muted">×{c.count}</span>
          </span>
        ))}
      </div>
    </div>
  );
}

function AuthTable({ events, failed }: { events: ServerAuthEvent[]; failed?: boolean }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[28rem] text-left text-xs">
        <thead className="text-text-muted">
          <tr>
            <th className="py-1 pr-3 font-semibold">When</th>
            <th className="py-1 pr-3 font-semibold">User</th>
            <th className="py-1 pr-3 font-semibold">From</th>
            <th className="py-1 font-semibold">Method</th>
          </tr>
        </thead>
        <tbody className="font-mono text-text-primary">
          {events.map((e, i) => (
            <tr key={i} className="border-t border-border">
              <td className="py-1 pr-3 text-text-secondary">
                <LocalTime value={e.time} style="ago" />
              </td>
              <td className="py-1 pr-3">
                {e.user || "(empty)"}
                {failed && e.invalid && <span className="ml-1 font-sans text-[10px] text-text-muted">no such user</span>}
              </td>
              <td className="py-1 pr-3">
                {e.from}
                {e.panel && <span className="ml-1 font-sans text-[10px] text-text-muted">this panel</span>}
              </td>
              <td className="py-1 text-text-secondary">{e.method ?? ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// One server: its alerts and everything its last check read, with a
// Refresh that checks it now, and its login to edit.
export function ServerDetail({ initial }: { initial: WatchedServer }) {
  const router = useRouter();
  const [server, setServer] = useState(initial);
  const canWrite = useCanWrite();
  const [busy, setBusy] = useState<"check" | "save" | "remove" | null>(null);
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ name: initial.name, username: initial.username, password: "" });
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [showPanelLogins, setShowPanelLogins] = useState(false);
  const snap = server.snapshot;
  const panelLogins = snap?.logins?.filter((l) => l.panel).length ?? 0;
  const logins = (snap?.logins ?? []).filter((l) => showPanelLogins || !l.panel);

  async function refresh() {
    setBusy("check");
    try {
      const res = await fetch(`/api/servers/${server.id}/check`, { method: "POST" });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      setServer(await res.json());
    } finally {
      setBusy(null);
    }
  }

  async function save() {
    setBusy("save");
    try {
      const res = await fetch(`/api/servers/${server.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(form),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      setServer(await res.json());
      setEditing(false);
      setForm((f) => ({ ...f, password: "" }));
      toast.success("Saved.");
    } finally {
      setBusy(null);
    }
  }

  async function remove() {
    setBusy("remove");
    try {
      const res = await fetch(`/api/servers/${server.id}`, { method: "DELETE" });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      toast.success(`Stopped watching ${server.name}.`);
      router.push("/servers");
      router.refresh();
    } finally {
      setBusy(null);
    }
  }

  async function ack(id: string) {
    const res = await fetch(`/api/servers/alerts/${id}/ack`, { method: "POST" });
    if (res.ok) {
      setServer((s) => ({ ...s, alerts: s.alerts.map((a) => (a.id === id ? { ...a, acknowledgedAt: new Date().toISOString() } : a)) }));
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <Link href="/servers" className="text-sm text-text-secondary hover:text-text-primary">
        ← Servers
      </Link>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="page-title">{server.name}</h1>
          <p className="mt-1 font-mono text-sm text-text-secondary">
            {server.username}@{server.host}
            {server.port !== 22 ? `:${server.port}` : ""}
            {snap ? ` · ${snap.hostname} · ${snap.os || "Linux"} · kernel ${snap.kernel} · ${snap.cpus} CPU${snap.cpus === 1 ? "" : "s"} · up ${uptimeLabel(snap.uptimeSeconds)}` : ""}
          </p>
          <p className="mt-1 text-xs text-text-muted">Checked <LocalTime value={server.checkedAt} style="ago" empty="never" /> · every minute</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={refresh} disabled={busy !== null}>
            {busy === "check" ? "Checking…" : "Refresh now"}
          </Button>
          {canWrite && (
            <>
              <Button variant="secondary" onClick={() => setEditing((e) => !e)} disabled={busy !== null}>
                Edit login
              </Button>
              <Button variant="danger" onClick={() => setConfirmRemove(true)} disabled={busy !== null}>
                Remove
              </Button>
            </>
          )}
        </div>
      </div>

      {confirmRemove && (
        <div className="card flex flex-wrap items-center gap-3 px-4 py-3 text-sm">
          <span className="text-text-primary">
            Stop watching {server.name}? Its alerts are resolved. Nothing on the server changes, and services deployed to
            it stay.
          </span>
          <Button variant="danger" className="px-3 py-1 text-xs" onClick={remove} disabled={busy !== null}>
            {busy === "remove" ? "Removing…" : "Remove"}
          </Button>
          <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setConfirmRemove(false)}>
            Cancel
          </Button>
        </div>
      )}

      {editing && (
        <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5">
          <label className={`${LABEL} flex-1`}>
            Name
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="field" />
          </label>
          <label className={`${LABEL} sm:w-40`}>
            SSH user
            <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="field" />
          </label>
          <label className={`${LABEL} sm:w-48`}>
            New password
            <input
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              className="field"
              autoComplete="off"
              placeholder="unchanged if empty"
            />
          </label>
          <Button onClick={save} disabled={busy !== null}>
            {busy === "save" ? "Checking login…" : "Save"}
          </Button>
        </div>
      )}

      {server.checkError && (
        <div className="card border-l-4 px-4 py-3 text-sm text-status-critical" style={{ borderColor: ROLE_COLOR.critical }}>
          The last check failed: {server.checkError}
          {snap && <span className="text-text-secondary"> — below is what the last successful check read.</span>}
        </div>
      )}

      {server.alerts.length > 0 && (
        <Section title="Alerts" hint={`${server.alerts.length} open`}>
          <ul className="flex flex-col gap-1.5">
            {server.alerts.map((a) => (
              <li key={a.id} className="flex flex-wrap items-center gap-2 rounded-2xl bg-surface-2 px-4 py-2.5">
                <StatusBadge role={a.severity === "critical" ? "critical" : "warning"} label={a.severity} />
                <span className="min-w-0 flex-1 text-sm text-text-primary">{a.message}</span>
                <span className="text-xs text-text-muted">
                  since <LocalTime value={a.startedAt} style="ago" />
                </span>
                {a.acknowledgedAt ? (
                  <span className="text-xs text-text-muted">acknowledged</span>
                ) : (
                  canWrite && (
                    <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => ack(a.id)}>
                      Acknowledge
                    </Button>
                  )
                )}
              </li>
            ))}
          </ul>
          <Link href="/servers/notifications" className="text-xs font-semibold text-accent hover:underline">
            Manage rules →
          </Link>
        </Section>
      )}

      {!snap ? (
        !server.checkError && <div className="card px-6 py-10 text-center text-sm text-text-muted">Not checked yet — press Refresh now.</div>
      ) : (
        <>
          <ServerMetrics snap={snap} />
          {snap.notes && snap.notes.length > 0 && (
            <div className="card flex flex-col gap-1 px-4 py-3 text-xs text-text-secondary">
              {snap.notes.map((n) => (
                <span key={n}>⚠ {n}</span>
              ))}
            </div>
          )}

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Section title="Logged in now" hint={`${snap.sessions?.length ?? 0} session${snap.sessions?.length === 1 ? "" : "s"}`}>
              {snap.sessions && snap.sessions.length > 0 ? (
                <ul className="flex flex-col gap-1 font-mono text-xs text-text-primary">
                  {snap.sessions.map((s, i) => (
                    <li key={i} className="flex flex-wrap gap-x-3 rounded-xl bg-surface-2 px-3 py-1.5">
                      <span className="font-bold">{s.user}</span>
                      <span className="text-text-secondary">{s.tty}</span>
                      <span className="text-text-secondary">{s.since}</span>
                      {s.from && <span className="ml-auto">{s.from}</span>}
                    </li>
                  ))}
                </ul>
              ) : (
                <Empty>Nobody is logged in.</Empty>
              )}
            </Section>

            <Section title="Successful logins" hint={snap.authSource ? `last 24 h · from ${snap.authSource}` : undefined}>
              {!snap.authSource ? (
                <Empty>The SSH log couldn&apos;t be read on this server.</Empty>
              ) : logins.length > 0 ? (
                <AuthTable events={logins} />
              ) : (
                <Empty>No SSH logins in the last 24 hours{panelLogins > 0 ? " besides the panel's own checks" : ""}.</Empty>
              )}
              {panelLogins > 0 && (
                <label className="flex items-center gap-2 text-xs text-text-muted">
                  <input type="checkbox" checked={showPanelLogins} onChange={(e) => setShowPanelLogins(e.target.checked)} />
                  Show {panelLogins} login{panelLogins === 1 ? "" : "s"} by this panel ({server.username} from {snap.panelAddress})
                </label>
              )}
            </Section>
          </div>

          <Section
            title="Failed logins"
            hint={snap.authSource ? `${snap.failed.lastHour} in the last hour · ${snap.failed.last24h} in 24 h` : undefined}
          >
            {!snap.authSource ? (
              <Empty>The SSH log couldn&apos;t be read on this server.</Empty>
            ) : snap.failed.last24h === 0 ? (
              <Empty>No failed SSH logins in the last 24 hours.</Empty>
            ) : (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <Counts title="Most tried users" items={snap.failed.topUsers} />
                  <Counts title="Most attempts from" items={snap.failed.topSources} />
                </div>
                {snap.failed.recent && <AuthTable events={snap.failed.recent} failed />}
              </>
            )}
          </Section>

          <Section title="Disks" hint={`${snap.disks?.length ?? 0} filesystems`}>
            <ul className="flex flex-col gap-2">
              {(snap.disks ?? []).map((d) => {
                const role = usageRole(d.usedPercent);
                return (
                  <li key={d.mount} className="flex flex-col gap-1">
                    <div className="flex flex-wrap items-baseline gap-x-3 text-sm">
                      <span className="font-mono font-bold text-text-primary">{d.mount}</span>
                      <span className="font-mono text-xs text-text-muted">{d.filesystem}</span>
                      <span className="ml-auto font-mono text-xs text-text-secondary">
                        {bytesLabel(d.usedBytes)} of {bytesLabel(d.sizeBytes)} · {bytesLabel(d.availBytes)} free
                        {d.inodesUsedPercent >= 0 && ` · inodes ${Math.round(d.inodesUsedPercent)}%`}
                      </span>
                      <span className="font-mono text-sm font-bold" style={{ color: ROLE_COLOR[role] }}>
                        {Math.round(d.usedPercent)}%
                      </span>
                    </div>
                    <div className="h-2 overflow-hidden rounded-full bg-surface-2" role="meter" aria-valuenow={d.usedPercent} aria-valuemin={0} aria-valuemax={100} aria-label={`${d.mount} used`}>
                      <div className="h-full rounded-full" style={{ width: `${Math.min(100, d.usedPercent)}%`, backgroundColor: ROLE_COLOR[role] }} />
                    </div>
                  </li>
                );
              })}
            </ul>
            <p className="text-xs text-text-muted">
              Memory {Math.round(snap.memory.usedPercent)}% used ({bytesLabel(snap.memory.availableBytes)} available of{" "}
              {bytesLabel(snap.memory.totalBytes)})
              {snap.memory.swapTotalBytes > 0 && ` · swap ${bytesLabel(snap.memory.swapUsedBytes)} of ${bytesLabel(snap.memory.swapTotalBytes)}`} · load{" "}
              {snap.load1.toFixed(2)} / {snap.load5.toFixed(2)} / {snap.load15.toFixed(2)}
            </p>
          </Section>

          <Section
            title="Kernel errors (dmesg)"
            hint={snap.kernelErrors.readable ? `${snap.kernelErrors.lastHour} in the last hour · ${snap.kernelErrors.total} since boot` : undefined}
          >
            {!snap.kernelErrors.readable ? (
              <Empty>The kernel log couldn&apos;t be read on this server.</Empty>
            ) : !snap.kernelErrors.recent || snap.kernelErrors.recent.length === 0 ? (
              <Empty>No kernel errors since boot.</Empty>
            ) : (
              <ul className="flex max-h-80 flex-col gap-1 overflow-auto font-mono text-xs">
                {snap.kernelErrors.recent.map((k, i) => (
                  <li key={i} className="flex gap-3 rounded-lg bg-surface-2 px-3 py-1">
                    <span className="shrink-0 text-text-muted">
                      <LocalTime value={k.time} style="ago" empty="" />
                    </span>
                    <span className="break-all text-text-primary">{k.message}</span>
                  </li>
                ))}
              </ul>
            )}
          </Section>

          <Section title="Connections" hint={snap.connections.readable ? `${snap.connections.total} TCP sockets` : undefined}>
            {!snap.connections.readable ? (
              <Empty>Connections couldn&apos;t be read: neither ss nor netstat is installed.</Empty>
            ) : (
              <>
                <div className="flex flex-wrap gap-2 font-mono text-xs">
                  {(
                    [
                      ["Established", snap.connections.established],
                      ["Listening", snap.connections.listen],
                      ["Time-wait", snap.connections.timeWait],
                      ["Close-wait", snap.connections.closeWait],
                      ["SYN received", snap.connections.synRecv],
                    ] as const
                  ).map(([label, n]) => (
                    <span key={label} className="rounded-xl bg-surface-2 px-3 py-1.5 text-text-primary">
                      <span className="font-sans text-text-muted">{label}</span> <b>{n}</b>
                    </span>
                  ))}
                </div>
                <Counts title="Most established connections with" items={snap.connections.topPeers} />
              </>
            )}
          </Section>
        </>
      )}
    </div>
  );
}
