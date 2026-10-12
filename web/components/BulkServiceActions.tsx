// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { createContext, useContext, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";

interface Item {
  id: string;
  name: string;
  host: string;
}

interface Selection {
  selected: Set<string>;
  toggle: (id: string) => void;
  busy: boolean;
}

const SelectionContext = createContext<Selection | null>(null);

// runAll runs fn on every id, a few at a time, and returns the failures.
async function runAll(items: Item[], fn: (item: Item) => Promise<string | null>) {
  const failures: string[] = [];
  const queue = [...items];
  await Promise.all(
    Array.from({ length: Math.min(4, queue.length) }, async () => {
      for (let item = queue.shift(); item; item = queue.shift()) {
        const err = await fn(item);
        if (err) failures.push(`${item.name}: ${err}`);
      }
    }),
  );
  return failures;
}

// BulkServiceActions puts a selection toolbar above a list of services:
// tick any of them (with SelectBox on each card) to Sync them all at once,
// or remove them from the panel — which only deletes the panel's records;
// their containers, volumes and files on the servers are left as they are.
export function BulkServiceActions({ items, children }: { items: Item[]; children: React.ReactNode }) {
  const router = useRouter();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const canWrite = useCanWrite(); // Sync only reads; removing needs write
  // Ticks for services that left the list (removed elsewhere) don't count.
  const chosen = items.filter((i) => selected.has(i.id));
  const allTicked = items.length > 0 && chosen.length === items.length;

  function toggle(id: string) {
    setSelected((s) => {
      const next = new Set(s);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function sync() {
    setBusy(true);
    try {
      const failures = await runAll(chosen, async (item) => {
        const res = await fetch(`/api/services/${item.id}/sync`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({}),
        });
        return res.ok ? null : (await res.text()).trim();
      });
      if (failures.length === 0) toast.success(`Synced ${chosen.length} service${chosen.length === 1 ? "" : "s"}.`);
      else toast.error(`Synced ${chosen.length - failures.length} of ${chosen.length}. ${failures.join(" · ")}`, { duration: 12000 });
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    try {
      const failures = await runAll(chosen, async (item) => {
        const res = await fetch(`/api/services/${item.id}`, { method: "DELETE" });
        return res.ok ? null : (await res.text()).trim() || `HTTP ${res.status}`;
      });
      if (failures.length === 0) toast.success(`Removed ${chosen.length} from the panel. Nothing on the servers changed.`);
      else toast.error(`Removed ${chosen.length - failures.length} of ${chosen.length}. ${failures.join(" · ")}`, { duration: 12000 });
      setSelected(new Set());
      setConfirmRemove(false);
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <SelectionContext.Provider value={{ selected, toggle, busy }}>
      {items.length > 0 && (
        <div className="card sticky top-16 z-20 flex flex-col gap-3 px-4 py-3">
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                className="h-4 w-4"
                checked={allTicked}
                disabled={busy}
                onChange={() => setSelected(allTicked ? new Set() : new Set(items.map((i) => i.id)))}
              />
              Select all ({items.length})
            </label>
            <span className="text-xs text-text-muted">{chosen.length} selected</span>
            <div className="ml-auto flex flex-wrap gap-2">
              <Button variant="secondary" className="px-3 py-1.5 text-xs" onClick={sync} disabled={chosen.length === 0 || busy}>
                {busy && !confirmRemove ? "Syncing…" : `Sync${chosen.length ? ` ${chosen.length}` : ""}`}
              </Button>
              {canWrite && (
                <Button variant="danger" className="px-3 py-1.5 text-xs" onClick={() => setConfirmRemove(true)} disabled={chosen.length === 0 || busy}>
                  Remove{chosen.length ? ` ${chosen.length}` : ""} from panel
                </Button>
              )}
            </div>
          </div>
          {confirmRemove && chosen.length > 0 && (
            <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 p-3 text-sm">
              <p className="font-semibold text-text-primary">
                Remove {chosen.length} service{chosen.length === 1 ? "" : "s"} from the panel?
              </p>
              <p className="text-text-secondary">
                This only deletes the panel&apos;s record{chosen.length === 1 ? "" : "s"}.{" "}
                <b>No container is deleted</b>: containers, volumes, config and files on the servers stay exactly as they are
                and keep running. You can add them back later (Detect, or Connect existing).
              </p>
              <ul className="flex flex-wrap gap-1.5">
                {chosen.map((i) => (
                  <li key={i.id} className="rounded-full bg-surface-1 px-2.5 py-0.5 font-mono text-xs text-text-primary">
                    {i.name} <span className="text-text-muted">@ {i.host}</span>
                  </li>
                ))}
              </ul>
              <div className="flex gap-2">
                <Button variant="danger" className="px-3 py-1.5 text-xs" onClick={remove} disabled={busy}>
                  {busy ? "Removing…" : `Yes, remove ${chosen.length} from the panel`}
                </Button>
                <Button variant="secondary" className="px-3 py-1.5 text-xs" onClick={() => setConfirmRemove(false)} disabled={busy}>
                  Cancel
                </Button>
              </div>
            </div>
          )}
        </div>
      )}
      {children}
    </SelectionContext.Provider>
  );
}

// SelectBox is a service's tick box for BulkServiceActions. It sits beside
// (not inside) the card's link, so ticking it doesn't open the service.
export function SelectBox({ id, name }: { id: string; name: string }) {
  const ctx = useContext(SelectionContext);
  if (!ctx) return null;
  const checked = ctx.selected.has(id);
  return (
    <input
      type="checkbox"
      className="h-4 w-4 shrink-0 cursor-pointer"
      aria-label={`Select ${name}`}
      checked={checked}
      disabled={ctx.busy}
      onChange={() => ctx.toggle(id)}
    />
  );
}
