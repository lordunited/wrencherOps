"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { ServiceLogEntry } from "@/lib/api";
import { formatDuration, logStatusRole } from "@/lib/format";

const ACTION_LABEL: Record<ServiceLogEntry["action"], string> = {
  deploy: "Deploy",
  start: "Start",
  stop: "Stop",
  down: "Down",
  destroy: "Delete",
  sync: "Sync",
  rules: "Rules",
  "move-config": "Move config",
  connect: "Connect",
  unlock: "Unlock",
  nginx: "Nginx config",
};

// How often to refresh while something in the history is still running.
const RUNNING_POLL_MS = 2500;

// Saved history of everything the panel has run against one service, kept
// in the database so it survives reloads and backend restarts. Click an
// entry to read its full output or download it.
export function ServiceHistory({
  serviceId,
  host,
  refreshKey,
}: {
  serviceId: string;
  host: string;
  // Changes whenever the parent starts or finishes an action, so the list
  // picks up new entries right away instead of waiting for a reload.
  refreshKey: string;
}) {
  const [entries, setEntries] = useState<ServiceLogEntry[] | null>(null);
  const [loadError, setLoadError] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);
  const [openEntry, setOpenEntry] = useState<ServiceLogEntry | null>(null);

  const loadList = useCallback(async () => {
    const res = await fetch(`/api/services/${serviceId}/logs`, { cache: "no-store" });
    if (!res.ok) {
      setLoadError(true);
      return;
    }
    setLoadError(false);
    setEntries(await res.json());
  }, [serviceId]);

  const loadEntry = useCallback(
    async (id: string) => {
      const res = await fetch(`/api/services/${serviceId}/logs/${id}`, { cache: "no-store" });
      if (res.ok) setOpenEntry(await res.json());
    },
    [serviceId],
  );

  useEffect(() => {
    // Deferred so the fetch's state updates never run synchronously inside the effect.
    const t = setTimeout(loadList, 0);
    return () => clearTimeout(t);
  }, [loadList, refreshKey]);

  const anyRunning = entries?.some((e) => e.status === "running") ?? false;
  useEffect(() => {
    if (!anyRunning) return;
    const t = setInterval(() => {
      loadList();
      if (openId) loadEntry(openId);
    }, RUNNING_POLL_MS);
    return () => clearInterval(t);
  }, [anyRunning, openId, loadList, loadEntry]);

  function toggle(id: string) {
    if (openId === id) {
      setOpenId(null);
      setOpenEntry(null);
      return;
    }
    setOpenId(id);
    setOpenEntry(null);
    loadEntry(id);
  }

  function download(entry: ServiceLogEntry) {
    const header =
      `Service: ${entry.serviceName}\n` +
      `Host: ${entry.host}\n` +
      `Action: ${ACTION_LABEL[entry.action] ?? entry.action}\n` +
      `Status: ${entry.status}\n` +
      `Started: ${new Date(entry.startedAt).toLocaleString()}\n` +
      (entry.finishedAt ? `Finished: ${new Date(entry.finishedAt).toLocaleString()}\n` : "") +
      "\n";
    const blob = new Blob([header + (entry.log ?? "")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    const stamp = entry.startedAt.replace(/[:.]/g, "-");
    a.href = url;
    a.download = `${entry.host}-${entry.action}-${stamp}.log`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-baseline justify-between gap-2 px-1">
        <h2 className="section-title">Activity history</h2>
        <p className="text-xs text-text-muted">Everything run on {host}, newest first</p>
      </div>

      {loadError && (
        <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-status-critical">
          Could not load the history. Reload the page to try again.
        </p>
      )}
      {entries === null && !loadError && <p className="px-1 text-sm text-text-muted">Loading…</p>}
      {entries?.length === 0 && (
        <p className="rounded-2xl bg-surface-2 px-4 py-6 text-center text-sm text-text-muted">
          No activity yet. Deploy, Start, Stop, Down, Delete and Sync will all be recorded here.
        </p>
      )}

      {entries && entries.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {entries.map((entry) => {
            const open = openId === entry.id;
            return (
              <li key={entry.id} className="overflow-hidden rounded-2xl bg-surface-2">
                <button
                  onClick={() => toggle(entry.id)}
                  aria-expanded={open}
                  className="grid w-full grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1 px-4 py-3 text-left text-sm transition-colors hover:bg-surface-hover sm:grid-cols-[11rem_6.5rem_1fr_auto]"
                >
                  <span className="col-span-2 font-mono text-xs tabular-nums text-text-muted sm:col-span-1">
                    {new Date(entry.startedAt).toLocaleString()}
                  </span>
                  <span className="font-bold text-text-primary">
                    {ACTION_LABEL[entry.action] ?? entry.action}
                  </span>
                  <span className="flex items-center justify-end gap-3 sm:contents">
                    <span className="sm:justify-self-start">
                      <StatusBadge role={logStatusRole(entry.status)} label={entry.status} />
                    </span>
                    <span className="font-mono text-xs tabular-nums text-text-muted sm:justify-self-end">
                      {formatDuration(entry.startedAt, entry.finishedAt)}
                      <span aria-hidden className="ml-2 inline-block w-3 text-center">
                        {open ? "▾" : "▸"}
                      </span>
                    </span>
                  </span>
                </button>

                {open && (
                  <div className="flex flex-col gap-2 px-2 pb-2">
                    <pre className="log-panel max-h-96 overflow-y-auto whitespace-pre-wrap p-4">
                      {openEntry?.id === entry.id
                        ? openEntry.log || "No output was recorded."
                        : "Loading…"}
                    </pre>
                    {openEntry?.id === entry.id && (
                      <Button variant="secondary" className="w-fit" onClick={() => download(openEntry)}>
                        Download log
                      </Button>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
