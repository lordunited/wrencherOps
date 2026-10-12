// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Icon } from "@/components/icons";
import { GROUP_LABEL, SORT_LABEL, oneOf, type GroupKey, type HealthFilter, type SortKey } from "@/lib/listing";

const SORTS = Object.keys(SORT_LABEL) as SortKey[];
const SHOWS: HealthFilter[] = ["all", "problems", "healthy"];

export interface ListControls {
  group: GroupKey;
  sort: SortKey;
  show: HealthFilter;
  query: string;
  setGroup: (g: GroupKey) => void;
  setSort: (s: SortKey) => void;
  setShow: (s: HealthFilter) => void;
  setQuery: (q: string) => void;
}

// useListControls keeps a list's grouping, sort and health filter in the
// URL (so a view can be shared and survives a reload); the search box is
// local.
export function useListControls(defaults: { group: GroupKey; sort?: SortKey }, groups: GroupKey[]): ListControls {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const [query, setQuery] = useState("");
  const defaultSort = defaults.sort ?? "attention";

  function set(key: string, value: string, fallback: string) {
    const next = new URLSearchParams(params.toString());
    if (value === fallback) next.delete(key);
    else next.set(key, value);
    if (key === "group") next.delete("view"); // the older By server / By app switch
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }

  return {
    group: oneOf(params.get("group"), groups, defaults.group),
    sort: oneOf(params.get("sort"), SORTS, defaultSort),
    show: oneOf(params.get("show"), SHOWS, "all"),
    query,
    setGroup: (g) => set("group", g, defaults.group),
    setSort: (s) => set("sort", s, defaultSort),
    setShow: (s) => set("show", s, "all"),
    setQuery,
  };
}

function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: React.ReactNode }[];
  onChange: (v: T) => void;
}) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex flex-wrap rounded-full bg-surface-2 p-0.5">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={value === o.value}
          onClick={() => onChange(o.value)}
          className={`inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
            value === o.value ? "bg-surface-1 text-text-primary shadow-card" : "text-text-secondary hover:text-text-primary"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

// ListToolbar is a list's search, health filter, grouping and sort.
export function ListToolbar({
  controls,
  groups,
  counts,
  shown,
  placeholder = "Search name, server, path…",
}: {
  controls: ListControls;
  groups: GroupKey[];
  counts: Record<HealthFilter, number>;
  shown: number;
  placeholder?: string;
}) {
  const { group, sort, show, query } = controls;
  return (
    <div className="card flex flex-col gap-2.5 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <label className="relative min-w-[12rem] flex-1">
          <span className="sr-only">Search</span>
          <Icon name="search" size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
          <input
            type="search"
            value={query}
            onChange={(e) => controls.setQuery(e.target.value)}
            placeholder={placeholder}
            className="field w-full py-1.5 pl-9 text-sm"
          />
        </label>
        <Segmented<HealthFilter>
          label="Show"
          value={show}
          onChange={controls.setShow}
          options={[
            { value: "all", label: <>All <span className="tabular-nums text-text-muted">{counts.all}</span></> },
            {
              value: "problems",
              label: (
                <>
                  Needs attention{" "}
                  <span className={`tabular-nums ${counts.problems > 0 ? "text-status-critical" : "text-text-muted"}`}>{counts.problems}</span>
                </>
              ),
            },
            { value: "healthy", label: <>Healthy <span className="tabular-nums text-text-muted">{counts.healthy}</span></> },
          ]}
        />
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-text-secondary">
        <span className="flex flex-wrap items-center gap-2">
          Group by
          <Segmented<GroupKey> label="Group by" value={group} onChange={controls.setGroup} options={groups.map((g) => ({ value: g, label: GROUP_LABEL[g] }))} />
        </span>
        <label className="flex items-center gap-2">
          Sort
          <select value={sort} onChange={(e) => controls.setSort(e.target.value as SortKey)} className="field py-1 text-xs">
            {SORTS.map((s) => (
              <option key={s} value={s}>
                {SORT_LABEL[s]}
              </option>
            ))}
          </select>
        </label>
        <span className="ml-auto tabular-nums text-text-muted">
          {shown === counts.all ? `${counts.all} shown` : `${shown} of ${counts.all} shown`}
        </span>
      </div>
    </div>
  );
}

// StackChip marks a Compose stack: yellow, with its project's name.
export function StackChip({ project }: { project?: string }) {
  return (
    <span
      className="inline-flex max-w-full items-center gap-1 rounded-full bg-stack-soft px-2 py-0.5 text-[11px] font-semibold text-stack-ink"
      title={project ? `Compose stack ${project}` : "Compose stack"}
    >
      <Icon name="stack" size={12} />
      <span className="truncate">{project ? project : "stack"}</span>
    </span>
  );
}

// GroupHeading is a group's title in a list.
export function GroupHeading({ label, count, mono = false, children }: { label: string; count: number; mono?: boolean; children?: React.ReactNode }) {
  if (!label) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-1">
      <h2 className={`text-base font-bold text-text-primary ${mono ? "font-mono" : ""}`}>{label}</h2>
      <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[11px] font-semibold tabular-nums text-text-secondary">{count}</span>
      {children}
    </div>
  );
}
