// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Sorting, grouping and filtering the panel's lists of services and stacks.

export type SortKey = "attention" | "name" | "server" | "updated";
export type GroupKey = "server" | "app" | "health" | "none";
export type HealthFilter = "all" | "problems" | "healthy";

export const SORT_LABEL: Record<SortKey, string> = {
  attention: "Needs attention first",
  name: "Name",
  server: "Server",
  updated: "Recently updated",
};

export const GROUP_LABEL: Record<GroupKey, string> = {
  server: "Server",
  app: "App",
  health: "Health",
  none: "None",
};

// ListItem is what a list needs of a service or a stack to sort it.
export interface ListItem {
  id: string;
  name: string;
  host: string;
  health: string;
  updatedAt: string;
  // app is the custom compose app it belongs to, if any.
  app?: string;
  // text is everything a search matches against, besides name and host.
  text?: string;
}

// Health from worst to best: what needs a look comes first.
const HEALTH_RANK: Record<string, number> = {
  unreachable: 0,
  down: 1,
  degraded: 2,
  stopped: 3,
  paused: 4,
  starting: 5,
  unknown: 6,
  "not deployed": 7,
  healthy: 8,
};

export function healthRank(health: string): number {
  return HEALTH_RANK[health] ?? 6;
}

// isProblem: down, unreachable, degraded, stopped or paused.
export function isProblem(health: string): boolean {
  return healthRank(health) <= 4;
}

const byName = (a: ListItem, b: ListItem) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
const byHost = (a: ListItem, b: ListItem) => a.host.trim().localeCompare(b.host.trim(), undefined, { numeric: true });

export function sortItems<T extends ListItem>(items: T[], key: SortKey): T[] {
  const out = [...items];
  switch (key) {
    case "attention":
      return out.sort((a, b) => healthRank(a.health) - healthRank(b.health) || byName(a, b));
    case "name":
      return out.sort((a, b) => byName(a, b) || byHost(a, b));
    case "server":
      return out.sort((a, b) => byHost(a, b) || byName(a, b));
    case "updated":
      return out.sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt) || byName(a, b));
  }
}

export function filterItems<T extends ListItem>(items: T[], query: string, health: HealthFilter): T[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return items.filter((it) => {
    if (health === "problems" && !isProblem(it.health)) return false;
    if (health === "healthy" && it.health !== "healthy") return false;
    if (words.length === 0) return true;
    const hay = `${it.name} ${it.host} ${it.app ?? ""} ${it.health} ${it.text ?? ""}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

export interface Group<T> {
  key: string;
  label: string;
  items: T[];
}

const HEALTH_GROUPS = [
  { key: "attention", label: "Needs attention", has: (h: string) => isProblem(h) },
  { key: "pending", label: "Starting or not checked", has: (h: string) => !isProblem(h) && h !== "healthy" },
  { key: "healthy", label: "Healthy", has: (h: string) => h === "healthy" },
];

// groupItems splits items (already sorted) into groups, in a stable order:
// servers by address, apps by name with stacks outside any app last, and
// health from worst to best. Empty groups are left out.
export function groupItems<T extends ListItem>(items: T[], key: GroupKey): Group<T>[] {
  if (key === "none") return items.length ? [{ key: "all", label: "", items }] : [];
  if (key === "health") {
    return HEALTH_GROUPS.map((g) => ({ key: g.key, label: g.label, items: items.filter((it) => g.has(it.health)) })).filter((g) => g.items.length > 0);
  }
  const groups = new Map<string, T[]>();
  for (const it of items) {
    const k = key === "server" ? it.host.trim() : (it.app ?? "");
    groups.set(k, [...(groups.get(k) ?? []), it]);
  }
  return [...groups.entries()]
    .sort(([a], [b]) => (a === "" ? 1 : b === "" ? -1 : a.localeCompare(b, undefined, { numeric: true, sensitivity: "base" })))
    .map(([k, list]) => ({ key: k || "(none)", label: k || (key === "app" ? "Not part of an app" : "No server"), items: list }));
}

// healthCounts is how many items are in each health filter.
export function healthCounts(items: ListItem[]): Record<HealthFilter, number> {
  return {
    all: items.length,
    problems: items.filter((i) => isProblem(i.health)).length,
    healthy: items.filter((i) => i.health === "healthy").length,
  };
}

// Narrowing a URL parameter to one of the allowed values, or a fallback.
export function oneOf<T extends string>(value: string | null | undefined, allowed: readonly T[], fallback: T): T {
  return value && (allowed as readonly string[]).includes(value) ? (value as T) : fallback;
}

// stackEdge is the yellow edge a Compose stack's card carries, on top of
// its shadow (cards) or alone (rows).
export function stackEdge(card: boolean): { boxShadow: string } {
  return { boxShadow: card ? "inset 4px 0 0 var(--stack), var(--card-shadow)" : "inset 4px 0 0 var(--stack)" };
}
