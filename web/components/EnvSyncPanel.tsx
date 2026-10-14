// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { LineDiff } from "@/components/LineDiff";
import { StatusBadge } from "@/components/StatusBadge";
import type { ComposeApp, EnvSync } from "@/lib/api";
import { ENV_SYNC_LABEL } from "@/lib/envsync";
import { LocalTime } from "@/components/LocalTime";

// EnvSyncPanel compares an app's .env with its servers' (all of them, or
// one), so a file changed on a server, or an app change not deployed yet,
// shows instead of drifting unseen. Each difference can be settled either
// way: take the server's into the app, or write the app's to the server.
export function EnvSyncPanel({
  appId,
  appEnv,
  serviceId,
  pullBlocked,
  onPulled,
}: {
  appId: string;
  appEnv: string;
  serviceId?: string;
  // pullBlocked says why the app can't take a server's .env now (unsaved
  // edits), or is "".
  pullBlocked?: string;
  onPulled: (app: ComposeApp) => void;
}) {
  const [syncs, setSyncs] = useState<EnvSync[] | null>(null);
  const [checking, setChecking] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [log, setLog] = useState<{ serviceId: string; text: string } | null>(null);

  const check = useCallback(async () => {
    setChecking(true);
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(appId)}/env/sync`, { cache: "no-store" });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const all: EnvSync[] = await res.json();
      setSyncs(serviceId ? all.filter((s) => s.serviceId === serviceId) : all);
    } finally {
      setChecking(false);
    }
  }, [appId, serviceId]);

  useEffect(() => {
    const t = setTimeout(check, 0);
    return () => clearTimeout(t);
  }, [check]);

  async function act(kind: "pull" | "push", s: EnvSync) {
    setBusy(`${kind}:${s.serviceId}`);
    setLog(null);
    try {
      const res = await fetch(`/api/apps/${encodeURIComponent(appId)}/env/${kind}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ serviceId: s.serviceId, serverSha: s.serverSha ?? "" }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      if (kind === "pull") {
        const app: ComposeApp = await res.json();
        toast.success(`The app's .env is now ${s.host}'s. Deploy to all to give it to the other servers.`);
        onPulled(app);
      } else {
        const out: { log: string } = await res.json();
        setLog({ serviceId: s.serviceId, text: out.log });
        toast.success(`Wrote the app's .env to ${s.host}. Deploy to apply it to the running containers.`);
      }
      await check();
    } finally {
      setBusy(null);
    }
  }

  const outOfSync = syncs?.filter((s) => s.status !== "in-sync").length ?? 0;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-text-primary">.env on the server{serviceId ? "" : "s"}</span>
        {syncs && (outOfSync === 0 ? <StatusBadge role="good" label="in sync" /> : <StatusBadge role="warning" label={`${outOfSync} out of sync`} />)}
        <Button variant="secondary" className="ml-auto px-3 py-1 text-xs" onClick={check} disabled={checking || busy !== null}>
          {checking ? "Checking…" : "Check again"}
        </Button>
      </div>
      {!syncs && checking && <p className="text-xs text-text-muted">Reading each server&apos;s .env…</p>}
      {syncs && syncs.length === 0 && <p className="text-xs text-text-muted">No servers yet.</p>}
      <ul className="flex flex-col gap-2">
        {syncs?.map((s) => {
          const label = ENV_SYNC_LABEL[s.status];
          const comparable = s.status === "differs" || s.status === "only-on-server";
          return (
            <li key={s.serviceId} className="flex flex-col gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono font-semibold text-text-primary">{s.host}</span>
                <StatusBadge role={label.role} label={label.text} />
                <span className="font-mono text-text-muted">{s.path}</span>
                <span className="text-text-muted">
                  checked <LocalTime value={s.checkedAt} style="ago" />
                </span>
              </div>
              <p className="text-text-secondary">{s.error || label.hint}</p>
              {s.status !== "in-sync" && s.status !== "unreachable" && (
                <div className="flex flex-wrap gap-2">
                  {comparable && (
                    <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setOpen(open === s.serviceId ? null : s.serviceId)} aria-expanded={open === s.serviceId}>
                      {open === s.serviceId ? "Hide differences" : "Show differences"}
                    </Button>
                  )}
                  {comparable && (
                    <Button
                      variant="warning"
                      className="px-3 py-1 text-xs"
                      disabled={busy !== null || Boolean(pullBlocked)}
                      title={pullBlocked || "Replace the app's .env with this server's"}
                      onClick={() => act("pull", s)}
                    >
                      {busy === `pull:${s.serviceId}` ? "Taking…" : "Use the server's .env"}
                    </Button>
                  )}
                  {appEnv.trim() !== "" && (
                    <Button className="px-3 py-1 text-xs" disabled={busy !== null} onClick={() => act("push", s)} title="Write the app's saved .env to this server, the old one backed up">
                      {busy === `push:${s.serviceId}` ? "Writing…" : "Write the app's .env to this server"}
                    </Button>
                  )}
                </div>
              )}
              {pullBlocked && comparable && <p className="text-status-warning">{pullBlocked}</p>}
              {open === s.serviceId && comparable && (
                <div className="flex flex-col gap-1">
                  <p className="text-text-muted">
                    <span className="font-semibold">−</span> the app&apos;s saved .env · <span className="font-semibold">+</span> {s.host}&apos;s
                  </p>
                  <LineDiff before={appEnv} after={s.serverEnv ?? ""} />
                </div>
              )}
              {log?.serviceId === s.serviceId && <pre className="log-panel max-h-40 overflow-auto p-3 text-[11px]">{log.text}</pre>}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
