// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { useMe } from "@/components/Access";
import { Icon } from "@/components/icons";
import type { ServerAlert } from "@/lib/api";

const SEEN_KEY = "server-alerts-seen";
const POLL_SECONDS = 30;

function loadSeen(): Set<string> {
  try {
    return new Set(JSON.parse(localStorage.getItem(SEEN_KEY) ?? "[]") as string[]);
  } catch {
    return new Set();
  }
}

function saveSeen(ids: Set<string>) {
  try {
    // Only ids still worth remembering: the last 500.
    localStorage.setItem(SEEN_KEY, JSON.stringify([...ids].slice(-500)));
  } catch {
    // Private mode or storage blocked: alerts may pop up again, nothing worse.
  }
}

// ServerAlertsNotifier keeps an eye on server alerts from every page: a bell
// in the header with the number open, and a pop-up per server when new ones
// fire. A pop-up shows once per alert in this browser; acknowledging it
// marks the alerts acknowledged for everyone.
export function ServerAlertsNotifier() {
  const router = useRouter();
  const [open, setOpen] = useState<ServerAlert[]>([]);
  // Where the bell and a toast's View go: pages the user can open.
  const tabs = useMe()?.tabs ?? {};
  const seesServers = Boolean(tabs["servers"]);
  const listHref = tabs["servers.notifications"] ? "/servers/notifications" : "/servers";
  const seen = useRef<Set<string> | null>(null);

  const poll = useCallback(async () => {
    let alerts: ServerAlert[];
    try {
      const res = await fetch("/api/servers/alerts", { cache: "no-store" });
      if (!res.ok) return;
      alerts = await res.json();
    } catch {
      return;
    }
    setOpen(alerts);
    if (!seen.current) seen.current = loadSeen();
    const fresh = alerts.filter((a) => !a.acknowledgedAt && !seen.current!.has(a.id));
    if (fresh.length === 0) return;

    const byServer = new Map<string, ServerAlert[]>();
    for (const a of fresh) byServer.set(a.serverId, [...(byServer.get(a.serverId) ?? []), a]);
    for (const [serverId, list] of byServer) {
      const critical = list.some((a) => a.severity === "critical");
      const show = critical ? toast.error : toast.warning;
      show(`${list[0].serverName || "A server"}: ${list.length} new alert${list.length === 1 ? "" : "s"}`, {
        id: `server-alerts-${serverId}`,
        description: (
          <ul className="mt-1 flex flex-col gap-0.5">
            {list.slice(0, 4).map((a) => (
              <li key={a.id}>• {a.message}</li>
            ))}
            {list.length > 4 && <li>+{list.length - 4} more</li>}
          </ul>
        ),
        duration: critical ? Infinity : 20000,
        action: { label: "View", onClick: () => router.push(seesServers ? `/servers/${serverId}` : listHref) },
        cancel: {
          label: "Acknowledge",
          onClick: () => {
            for (const a of list) fetch(`/api/servers/alerts/${a.id}/ack`, { method: "POST" });
          },
        },
      });
      for (const a of list) seen.current!.add(a.id);
    }
    saveSeen(seen.current);
  }, [router, seesServers, listHref]);

  useEffect(() => {
    const first = setTimeout(poll, 0);
    const t = setInterval(() => {
      if (document.visibilityState === "visible") poll();
    }, POLL_SECONDS * 1000);
    return () => {
      clearTimeout(first);
      clearInterval(t);
    };
  }, [poll]);

  const critical = open.some((a) => a.severity === "critical");
  return (
    <Link
      href={listHref}
      className="relative grid h-9 w-9 place-items-center rounded-full bg-surface-1 text-text-secondary shadow-card transition-colors hover:text-text-primary"
      aria-label={open.length ? `${open.length} open server alerts` : "Server alerts"}
      title={open.length ? `${open.length} open server alert${open.length === 1 ? "" : "s"}` : "No open server alerts"}
    >
      <Icon name="bell" size={18} />
      {open.length > 0 && (
        <span
          className="absolute -right-1 -top-1 min-w-[18px] rounded-full px-1 text-center text-[11px] font-bold leading-[18px] text-white"
          style={{ backgroundColor: critical ? "var(--status-critical)" : "var(--status-warning)" }}
        >
          {open.length > 99 ? "99+" : open.length}
        </span>
      )}
    </Link>
  );
}
