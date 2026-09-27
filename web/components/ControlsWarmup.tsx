"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import type { Service } from "@/lib/api";

const UNLOCK_AFTER_MS = 5 * 60 * 1000;
const POLL_MS = 15_000;

// Shown on a connected stack until its Start/Stop/Down unlock. The panel
// checks the stack every 30 seconds on its own; this shows how far along
// the 5 minutes of continuous health it is, and picks up the unlock.
export function ControlsWarmup({ service, onUpdate }: { service: Service; onUpdate: (s: Service) => void }) {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, []);

  useEffect(() => {
    const poll = setInterval(async () => {
      const res = await fetch(`/api/services/${service.id}`, { cache: "no-store" });
      if (!res.ok) return;
      const fresh: Service = await res.json();
      if (fresh.controlsUnlockedAt) {
        toast.success("The stack stayed healthy for 5 minutes. Start, Stop and Down are now available.");
      }
      onUpdate(fresh);
    }, POLL_MS);
    return () => clearInterval(poll);
  }, [service.id, onUpdate]);

  const since = service.healthySince ? new Date(service.healthySince).getTime() : null;
  const elapsed = since ? Math.min(UNLOCK_AFTER_MS, Math.max(0, now - since)) : 0;
  const left = UNLOCK_AFTER_MS - elapsed;
  const pct = (elapsed / UNLOCK_AFTER_MS) * 100;
  const mmss = (ms: number) => {
    const s = Math.ceil(ms / 1000);
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
  };

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5" aria-live="polite">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="section-title">Start, Stop and Down unlock after 5 healthy minutes</h2>
        <span className="font-mono text-sm font-bold tabular-nums text-text-primary">
          {since ? `${mmss(elapsed)} / 5:00` : "waiting"}
        </span>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-surface-2" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)}>
        <div
          className={`h-full rounded-full transition-[width] duration-1000 ${since ? "bg-status-good" : "bg-transparent"}`}
          style={{ width: `${pct}%` }}
        />
      </div>
      <p className="text-sm text-text-secondary">
        {since ? (
          left > 0 ? (
            <>
              Every part of the stack has been healthy for {mmss(elapsed)}. The controls unlock in about {mmss(left)} if
              nothing fails in the meantime.
            </>
          ) : (
            <>Five healthy minutes reached. Unlocking on the next check…</>
          )
        ) : service.lastSyncedAt ? (
          <>
            <span className="font-semibold text-status-critical">Not healthy right now:</span> {service.healthDetail || service.health}. The
            count starts again as soon as every container is running and Prometheus is ready.
          </>
        ) : (
          <>The panel checks the stack every 30 seconds. The first check is on its way.</>
        )}
      </p>
      <p className="text-xs text-text-muted">
        The panel watches this stack by itself, so you can leave this page. Any failed check restarts the count. Once
        unlocked, the controls stay available.
      </p>
    </section>
  );
}
