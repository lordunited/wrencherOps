// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { AuthorCredit } from "@/components/AuthorCredit";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Icon, type IconName } from "@/components/icons";
import { useMe } from "@/components/Access";
import { NAV_CATEGORIES, activeHref, categoryVars, visibleCategories } from "@/lib/nav";

type QuickTab = { href: string; label: string; icon: IconName; match: (p: string) => boolean };

// quickTabs are the tab bar's shortcuts the user can see: Deploy goes to
// the first deploy page they have.
function quickTabs(categories: typeof NAV_CATEGORIES): QuickTab[] {
  const hrefs = new Set(categories.flatMap((c) => c.items.map((i) => i.href)));
  const deploy = categories.find((c) => c.name === "Deploy")?.items[0]?.href;
  const tabs: QuickTab[] = [{ href: "/", label: "Overview", icon: "home", match: (p) => p === "/" }];
  if (hrefs.has("/health")) tabs.push({ href: "/health", label: "Health", icon: "pulse", match: (p) => p.startsWith("/health") });
  if (hrefs.has("/projects")) tabs.push({ href: "/projects", label: "Projects", icon: "folder", match: (p) => p.startsWith("/projects") });
  if (deploy) tabs.push({ href: deploy, label: "Deploy", icon: "deploy", match: (p) => p.startsWith("/deploy") });
  return tabs;
}

// Phone navigation: a floating tab bar within thumb reach, plus a "More"
// sheet listing every page. Hidden from md up, where the Sidebar takes over.
export function MobileNav() {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const me = useMe();
  const categories = me ? visibleCategories(me.tabs) : NAV_CATEGORIES;
  const tabs = quickTabs(categories);

  // Close the sheet after navigating.
  useEffect(() => {
    const t = setTimeout(() => setOpen(false), 0);
    return () => clearTimeout(t);
  }, [pathname]);

  return (
    <>
      {open && (
        <div className="fixed inset-0 z-40 bg-black/40 md:hidden" onClick={() => setOpen(false)}>
          <nav
            className="card absolute inset-x-3 bottom-[calc(5.5rem+env(safe-area-inset-bottom,0px))] max-h-[70dvh] overflow-y-auto p-4"
            onClick={(e) => e.stopPropagation()}
            aria-label="All pages"
          >
            {categories.map((category) => (
              <div key={category.name} className="mb-3 last:mb-0" style={categoryVars(category.icon) as React.CSSProperties}>
                <span className="flex items-center gap-2 px-2 pb-1 text-[11px] font-bold uppercase tracking-[0.08em] text-[var(--cat-ink)]">
                  <span className="grid h-6 w-6 place-items-center rounded-lg bg-[var(--cat-soft)] text-[var(--cat)]">
                    <Icon name={category.icon} size={14} />
                  </span>
                  {category.name}
                </span>
                <div className="grid grid-cols-2 gap-1.5">
                  {category.items.map((item) => {
                    const active = item.href === activeHref(pathname);
                    return (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={`rounded-xl px-3 py-2.5 text-sm font-semibold ${
                          active ? "bg-[var(--cat-soft)] text-[var(--cat-ink)]" : "bg-surface-2 text-text-primary"
                        }`}
                        style={active ? { boxShadow: "inset 3px 0 0 var(--cat)" } : undefined}
                      >
                        {item.label}
                      </Link>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
          <AuthorCredit className="mt-4 border-t border-border pt-3" />
        </div>
      )}

      <nav
        aria-label="Main"
        className="fixed inset-x-3 bottom-[calc(0.75rem+env(safe-area-inset-bottom,0px))] z-50 flex items-center justify-around rounded-[22px] bg-tabbar-bg px-1.5 py-1.5 shadow-card md:hidden"
      >
        {tabs.map((tab) => {
          const active = !open && tab.match(pathname);
          return (
            <Link
              key={tab.href}
              href={tab.href}
              aria-current={active ? "page" : undefined}
              className={`flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-2xl py-2 text-[11px] font-semibold ${
                active ? "text-white" : "text-tabbar-ink"
              }`}
              style={active ? { background: "var(--brand-hot)" } : undefined}
            >
              <Icon name={tab.icon} size={20} />
              {tab.label}
            </Link>
          );
        })}
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className={`flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-2xl py-2 text-[11px] font-semibold ${
            open ? "bg-white/10 text-white" : "text-tabbar-ink"
          }`}
        >
          <Icon name={open ? "close" : "menu"} size={20} />
          More
        </button>
      </nav>
    </>
  );
}
