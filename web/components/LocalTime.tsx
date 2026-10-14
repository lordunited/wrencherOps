// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useSyncExternalStore } from "react";
import { formatUTC, localTime, type TimeStyle } from "@/lib/format";

const subscribe = () => () => {};

// useHydrated is false while the server renders and while the browser
// hydrates that HTML, then true. Anything that depends on the viewer's clock,
// locale or time zone renders only when it's true: the server's text and the
// browser's first render must be identical, or React throws the page's HTML
// away ("Hydration failed") and rebuilds it.
export function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false,
  );
}

// LocalTime shows a moment in the viewer's own locale and time zone, or as
// "5 min ago": a fixed UTC text first (on the server, and while hydrating),
// the viewer's after.
export function LocalTime({ value, style = "datetime", empty = "—" }: { value?: string | number | Date | null; style?: TimeStyle; empty?: string }) {
  const hydrated = useHydrated();
  if (value === undefined || value === null || value === "") return <>{empty}</>;
  const d = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(d.getTime())) return <>{empty}</>;
  return (
    <time dateTime={d.toISOString()} title={hydrated ? d.toLocaleString() : undefined}>
      {hydrated ? localTime(d, style) : formatUTC(d, style)}
    </time>
  );
}
