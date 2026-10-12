// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import type { PanelTab } from "@/lib/api";
import type { AccessLevel, TabAccess } from "@/lib/nav";

type Level = "none" | AccessLevel;
const LEVELS: Level[] = ["none", "view", "write"];
const LEVEL_LABEL: Record<Level, string> = { none: "No access", view: "View", write: "Write" };

// grantable is whether a tab is set per user or group at all: admin tabs
// are for admins, the overview for everyone.
export const grantable = (t: PanelTab) => !t.adminOnly && !t.everyone;

// AccessMatrix is every tab of the panel, by category, each set to no
// access, view or write. cap is the most the role allows (a viewer:
// "view"); effective, when given, is what the user ends up with — through
// their groups too.
export function AccessMatrix({
  tabs,
  value,
  onChange,
  cap = "write",
  effective,
  disabled = false,
}: {
  tabs: PanelTab[];
  value: TabAccess;
  onChange: (next: TabAccess) => void;
  cap?: AccessLevel;
  effective?: TabAccess;
  disabled?: boolean;
}) {
  const categories: { name: string; tabs: PanelTab[] }[] = [];
  for (const t of tabs.filter(grantable)) {
    const c = categories.find((x) => x.name === t.category) ?? categories[categories.push({ name: t.category, tabs: [] }) - 1];
    c.tabs.push(t);
  }

  const allowed = (t: PanelTab, l: Level) => !disabled && (l !== "write" || (!t.viewOnly && cap === "write"));

  function set(keys: string[], l: Level) {
    const next = { ...value };
    for (const k of keys) {
      const t = tabs.find((x) => x.key === k);
      if (!t) continue;
      const level: Level = l === "write" && (t.viewOnly || cap !== "write") ? "view" : l;
      if (level === "none") delete next[k];
      else next[k] = level;
    }
    onChange(next);
  }

  return (
    <div className="flex flex-col gap-4">
      {categories.map((c) => (
        <section key={c.name} className="flex flex-col gap-1.5">
          <div className="flex flex-wrap items-center gap-2 px-1">
            <h3 className="text-[11px] font-bold uppercase tracking-[0.08em] text-text-muted">{c.name}</h3>
            {!disabled && (
              <span className="ml-auto flex gap-1 text-[11px]">
                <span className="text-text-muted">all:</span>
                {LEVELS.map((l) => (
                  <button
                    key={l}
                    type="button"
                    onClick={() => set(c.tabs.map((t) => t.key), l)}
                    className="rounded-full px-2 font-semibold text-accent hover:bg-surface-2"
                  >
                    {LEVEL_LABEL[l].toLowerCase()}
                  </button>
                ))}
              </span>
            )}
          </div>
          <ul className="flex flex-col divide-y divide-border overflow-hidden rounded-2xl bg-surface-2">
            {c.tabs.map((t) => {
              const current: Level = value[t.key] ?? "none";
              const eff = effective ? ((effective[t.key] ?? "none") as Level) : undefined;
              return (
                <li key={t.key} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2">
                  <span className="min-w-0 flex-1 text-sm font-medium text-text-primary">
                    {t.label}
                    {t.viewOnly && <span className="ml-2 text-[11px] font-normal text-text-muted">view only</span>}
                  </span>
                  {eff !== undefined && eff !== current && (
                    <span className="text-[11px] text-text-muted" title="What the user ends up with: the highest of their own access and their groups', capped by their role">
                      effective: <b className="text-text-secondary">{LEVEL_LABEL[eff].toLowerCase()}</b>
                    </span>
                  )}
                  <span role="radiogroup" aria-label={`${c.name} › ${t.label}`} className="inline-flex rounded-full bg-surface-1 p-0.5 shadow-card">
                    {LEVELS.map((l) => {
                      const on = current === l;
                      const ok = allowed(t, l);
                      return (
                        <button
                          key={l}
                          type="button"
                          role="radio"
                          aria-checked={on}
                          disabled={!ok && !on}
                          onClick={() => set([t.key], l)}
                          title={
                            !ok && l === "write"
                              ? t.viewOnly
                                ? "This page can only be viewed"
                                : "A viewer can't be given write access: change the role to writer first"
                              : undefined
                          }
                          className={`rounded-full px-2.5 py-1 text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-35 ${
                            on
                              ? l === "write"
                                ? "bg-accent text-on-accent"
                                : l === "view"
                                  ? "bg-accent-soft text-accent"
                                  : "bg-surface-2 text-text-primary"
                              : "text-text-secondary hover:text-text-primary"
                          }`}
                        >
                          {LEVEL_LABEL[l]}
                        </button>
                      );
                    })}
                  </span>
                </li>
              );
            })}
          </ul>
        </section>
      ))}
    </div>
  );
}
