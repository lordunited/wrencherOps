// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { createContext, useContext } from "react";
import { usePathname } from "next/navigation";
import { LinkButton } from "@/components/Button";
import type { Me } from "@/lib/api";
import { VIEW_ONLY_TABS, pageLevel, tabForPath } from "@/lib/nav";

const MeContext = createContext<Me | null>(null);

// AccessProvider hands the signed-in user, and what they can do, to the
// client components below it. The backend enforces it on every call; this
// only keeps the pages from offering what would be refused.
export function AccessProvider({ me, children }: { me: Me; children: React.ReactNode }) {
  return <MeContext.Provider value={me}>{children}</MeContext.Provider>;
}

export function useMe(): Me | null {
  return useContext(MeContext);
}

// useCanWrite says whether the user can change things on this page (or in
// tab, when given).
export function useCanWrite(tab?: string): boolean {
  const me = useMe();
  const pathname = usePathname();
  if (!me) return true;
  return (tab ? me.tabs[tab] : pageLevel(me.tabs, pathname)) === "write";
}

// WriteOnly shows its children only to users who can change things on the
// page: "+ New", Delete and the like.
export function WriteOnly({ children, tab }: { children: React.ReactNode; tab?: string }) {
  return useCanWrite(tab) ? <>{children}</> : null;
}

export function NoAccess() {
  return (
    <div className="card flex flex-col items-start gap-3 p-6 sm:p-8">
      <span className="grid h-11 w-11 place-items-center rounded-2xl bg-accent-soft text-lg font-extrabold text-accent" aria-hidden>
        ⊘
      </span>
      <h1 className="page-title">You don&apos;t have access to this page</h1>
      <p className="max-w-md text-sm text-text-secondary">
        Your account can&apos;t see this part of the panel. An admin can give you access in Admin › Users &amp; access.
      </p>
      <LinkButton href="/">Back to overview</LinkButton>
    </div>
  );
}

// AccessGate stands in front of every page: no access shows NoAccess
// (before the page's own calls are even refused), view only says so.
export function AccessGate({ children }: { children: React.ReactNode }) {
  const me = useMe();
  const pathname = usePathname();
  const level = pageLevel(me?.tabs ?? null, pathname);
  if (!level) return <NoAccess />;
  const tab = tabForPath(pathname);
  return (
    <>
      {level === "view" && tab && !VIEW_ONLY_TABS.has(tab) && (
        <p className="mb-4 flex items-center gap-2 rounded-2xl bg-surface-2 px-4 py-2 text-xs text-text-secondary" role="status">
          <span className="font-bold text-text-primary">View only.</span>
          You can look around here, not change anything; changes are refused.
        </p>
      )}
      {children}
    </>
  );
}
