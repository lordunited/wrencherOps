// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useState } from "react";
import Link from "next/link";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { ServerAlert, ServerAlertRule, ServerRuleKind } from "@/lib/api";
import { timeAgo } from "@/lib/format";

type Tab = "alerts" | "rules";

interface ServerRef {
  id: string;
  name: string;
}

// The Notifications section: alerts on every server, and the rules that
// raise them.
export function NotificationsManager({
  initialAlerts,
  initialRules,
  kinds,
  servers,
}: {
  initialAlerts: ServerAlert[];
  initialRules: ServerAlertRule[];
  kinds: ServerRuleKind[];
  servers: ServerRef[];
}) {
  const [tab, setTab] = useState<Tab>("alerts");
  const open = initialAlerts.filter((a) => !a.resolvedAt).length;
  return (
    <div className="flex flex-col gap-4">
      <nav className="inline-flex w-fit gap-1 rounded-full bg-surface-2 p-1 text-sm" aria-label="Notifications">
        {(
          [
            ["alerts", `Alerts${open ? ` (${open} open)` : ""}`],
            ["rules", "Rules"],
          ] as const
        ).map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setTab(key)}
            aria-current={tab === key ? "page" : undefined}
            className={`rounded-full px-3 py-1 font-semibold transition-colors ${
              tab === key ? "bg-surface-1 text-text-primary shadow-card" : "text-text-secondary hover:text-text-primary"
            }`}
          >
            {label}
          </button>
        ))}
      </nav>
      {tab === "alerts" ? <AlertsList initial={initialAlerts} /> : <RulesEditor initial={initialRules} kinds={kinds} servers={servers} />}
    </div>
  );
}

function AlertsList({ initial }: { initial: ServerAlert[] }) {
  const [alerts, setAlerts] = useState(initial);
  // Acknowledging is the Servers tab's or this one's to do.
  const writesServers = useCanWrite("servers");
  const writesNotifications = useCanWrite("servers.notifications");
  const canWrite = writesServers || writesNotifications;
  const [showResolved, setShowResolved] = useState(false);
  const shown = alerts.filter((a) => showResolved || !a.resolvedAt);

  async function ack(id: string) {
    const res = await fetch(`/api/servers/alerts/${id}/ack`, { method: "POST" });
    if (!res.ok) {
      toast.error(await res.text());
      return;
    }
    setAlerts((all) => all.map((a) => (a.id === id ? { ...a, acknowledgedAt: new Date().toISOString() } : a)));
  }

  async function ackAll(list: ServerAlert[]) {
    for (const a of list.filter((x) => !x.acknowledgedAt && !x.resolvedAt)) await ack(a.id);
  }

  const byServer = new Map<string, ServerAlert[]>();
  for (const a of shown) byServer.set(a.serverId, [...(byServer.get(a.serverId) ?? []), a]);

  return (
    <div className="flex flex-col gap-3">
      <label className="flex items-center gap-2 text-sm text-text-secondary">
        <input type="checkbox" checked={showResolved} onChange={(e) => setShowResolved(e.target.checked)} />
        Also show alerts resolved in the last 7 days
      </label>
      {byServer.size === 0 && <div className="card px-6 py-10 text-center text-sm text-text-muted">No open alerts. Every server is within its rules.</div>}
      {[...byServer.entries()].map(([serverId, list]) => {
        const unacked = list.filter((a) => !a.acknowledgedAt && !a.resolvedAt).length;
        return (
          <section key={serverId} className="card flex flex-col gap-2 p-4 sm:p-5">
            <div className="flex flex-wrap items-center gap-2">
              <Link href={`/servers/${serverId}`} className="text-base font-bold text-text-primary hover:underline">
                {list[0].serverName || serverId}
              </Link>
              <span className="text-xs text-text-muted">{list.filter((a) => !a.resolvedAt).length} open</span>
              {unacked > 1 && canWrite && (
                <Button variant="secondary" className="ml-auto px-3 py-1 text-xs" onClick={() => ackAll(list)}>
                  Acknowledge all {unacked}
                </Button>
              )}
            </div>
            <ul className="flex flex-col gap-1.5">
              {list.map((a) => (
                <li key={a.id} className={`flex flex-wrap items-center gap-2 rounded-2xl bg-surface-2 px-4 py-2.5 ${a.resolvedAt ? "opacity-60" : ""}`}>
                  <StatusBadge role={a.resolvedAt ? "muted" : a.severity === "critical" ? "critical" : "warning"} label={a.resolvedAt ? "resolved" : a.severity} />
                  <span className="min-w-0 flex-1 text-sm text-text-primary">{a.message}</span>
                  <span className="text-xs text-text-muted" title={new Date(a.startedAt).toLocaleString()}>
                    {a.resolvedAt ? `${timeAgo(a.startedAt)} → resolved ${timeAgo(a.resolvedAt)}` : `since ${timeAgo(a.startedAt)}`}
                  </span>
                  {a.ruleName && <span className="text-xs text-text-muted">· {a.ruleName}</span>}
                  {!a.resolvedAt &&
                    (a.acknowledgedAt ? (
                      <span className="text-xs text-text-muted">acknowledged</span>
                    ) : (
                      canWrite && (
                        <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => ack(a.id)}>
                          Acknowledge
                        </Button>
                      )
                    ))}
                </li>
              ))}
            </ul>
          </section>
        );
      })}
      <p className="text-xs text-text-muted">
        An alert opens when a rule&apos;s condition holds at a check, stays one alert while it keeps holding, and resolves on
        its own once it no longer does. Acknowledging only stops it popping up; it stays open until resolved.
      </p>
    </div>
  );
}

interface Draft {
  kind: string;
  name: string;
  threshold: string;
  severity: "warning" | "critical";
  enabled: boolean;
  serverId: string;
}

function RulesEditor({ initial, kinds, servers }: { initial: ServerAlertRule[]; kinds: ServerRuleKind[]; servers: ServerRef[] }) {
  const [rules, setRules] = useState(initial);
  const canWrite = useCanWrite();
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [adding, setAdding] = useState(false);
  const kindOf = (k: string) => kinds.find((x) => x.kind === k);
  const serverName = (id: string) => (id ? (servers.find((s) => s.id === id)?.name ?? "a removed server") : "Every server");

  function startEdit(r: ServerAlertRule) {
    setAdding(false);
    setEditing(r.id);
    setDraft({ kind: r.kind, name: r.name, threshold: String(r.threshold), severity: r.severity, enabled: r.enabled, serverId: r.serverId });
  }

  function startAdd() {
    setEditing(null);
    setAdding(true);
    setDraft({ kind: kinds[0]?.kind ?? "disk", name: "", threshold: "90", severity: "warning", enabled: true, serverId: "" });
  }

  async function send(method: "POST" | "PUT", url: string, body: object) {
    const res = await fetch(url, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!res.ok) {
      toast.error(await res.text(), { duration: 8000 });
      return null;
    }
    return (await res.json()) as ServerAlertRule;
  }

  async function save() {
    if (!draft) return;
    const body = { ...draft, threshold: Number(draft.threshold) };
    if (adding) {
      const r = await send("POST", "/api/servers/rules", body);
      if (r) {
        setRules((rs) => [...rs, r]);
        setAdding(false);
        toast.success("Rule added. It applies from the next check.");
      }
    } else if (editing) {
      const r = await send("PUT", `/api/servers/rules/${editing}`, body);
      if (r) {
        setRules((rs) => rs.map((x) => (x.id === r.id ? r : x)));
        setEditing(null);
        toast.success("Saved. It applies from the next check.");
      }
    }
  }

  async function toggle(r: ServerAlertRule) {
    const updated = await send("PUT", `/api/servers/rules/${r.id}`, { ...r, enabled: !r.enabled });
    if (updated) setRules((rs) => rs.map((x) => (x.id === updated.id ? updated : x)));
  }

  async function remove(r: ServerAlertRule) {
    const res = await fetch(`/api/servers/rules/${r.id}`, { method: "DELETE" });
    if (!res.ok) {
      toast.error(await res.text());
      return;
    }
    setRules((rs) => rs.filter((x) => x.id !== r.id));
  }

  const form = draft && (
    <div className="flex flex-col gap-3 rounded-2xl bg-surface-2 p-3">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-6">
        <label className="flex flex-col gap-1 text-xs text-text-secondary sm:col-span-2">
          What it watches
          <select className="field" value={draft.kind} disabled={!adding} onChange={(e) => setDraft({ ...draft, kind: e.target.value })}>
            {kinds.map((k) => (
              <option key={k.kind} value={k.kind}>
                {k.label}
              </option>
            ))}
          </select>
        </label>
        {draft.kind !== "unreachable" && (
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            Alert at ≥ ({kindOf(draft.kind)?.unit})
            <input className="field font-mono" inputMode="decimal" value={draft.threshold} onChange={(e) => setDraft({ ...draft, threshold: e.target.value })} />
          </label>
        )}
        <label className="flex flex-col gap-1 text-xs text-text-secondary">
          Severity
          <select className="field" value={draft.severity} onChange={(e) => setDraft({ ...draft, severity: e.target.value as Draft["severity"] })}>
            <option value="warning">Warning</option>
            <option value="critical">Critical</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-text-secondary sm:col-span-2">
          Applies to
          <select className="field" value={draft.serverId} onChange={(e) => setDraft({ ...draft, serverId: e.target.value })}>
            <option value="">Every server</option>
            {servers.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-text-secondary sm:col-span-4">
          Name
          <input className="field" value={draft.name} placeholder={kindOf(draft.kind)?.label} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </label>
      </div>
      <p className="text-xs text-text-muted">{kindOf(draft.kind)?.description}</p>
      <div className="flex gap-2">
        <Button className="px-3 py-1.5 text-xs" onClick={save}>
          {adding ? "Add rule" : "Save"}
        </Button>
        <Button
          variant="secondary"
          className="px-3 py-1.5 text-xs"
          onClick={() => {
            setAdding(false);
            setEditing(null);
          }}
        >
          Cancel
        </Button>
      </div>
    </div>
  );

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-text-secondary">Every server is checked against these each minute.</p>
        {canWrite && <Button onClick={startAdd}>+ Add rule</Button>}
      </div>
      {adding && form}
      <ul className="flex flex-col gap-1.5">
        {rules.map((r) => {
          const k = kindOf(r.kind);
          return (
            <li key={r.id} className={`card flex flex-col gap-2 px-4 py-3 ${r.enabled ? "" : "opacity-60"}`}>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
                <label className="flex items-center gap-2" title={r.enabled ? "On" : "Off"}>
                  <input type="checkbox" checked={r.enabled} onChange={() => toggle(r)} disabled={!canWrite} aria-label={`${r.name} on`} />
                </label>
                <span className="text-sm font-bold text-text-primary">{r.name}</span>
                <StatusBadge role={r.severity === "critical" ? "critical" : "warning"} label={r.severity} />
                <span className="font-mono text-xs text-text-secondary">
                  {r.kind === "unreachable" ? "can't log in to check it" : `${k?.label ?? r.kind} ≥ ${r.threshold} ${k?.unit ?? ""}`}
                </span>
                <span className="text-xs text-text-muted">· {serverName(r.serverId)}</span>
                {r.builtin && <span className="text-[11px] text-text-muted">built-in</span>}
                <span className={`ml-auto gap-2 ${canWrite ? "flex" : "hidden"}`}>
                  <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => startEdit(r)}>
                    Edit
                  </Button>
                  {!r.builtin && (
                    <Button variant="danger" className="px-3 py-1 text-xs" onClick={() => remove(r)}>
                      Delete
                    </Button>
                  )}
                </span>
              </div>
              {editing === r.id && form}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
