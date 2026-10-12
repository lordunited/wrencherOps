// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { AuthorCredit } from "@/components/AuthorCredit";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Brand } from "@/components/Brand";
import { Icon } from "@/components/icons";
import { useMe } from "@/components/Access";
import { NAV_CATEGORIES, activeHref, categoryVars, visibleCategories } from "@/lib/nav";

// Desktop navigation: only the tabs the user can see. On phones the
// MobileNav tab bar replaces it.
export function Sidebar() {
  const pathname = usePathname();
  const me = useMe();
  const categories = me ? visibleCategories(me.tabs) : NAV_CATEGORIES;

  return (
    <aside className="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col gap-5 overflow-y-auto bg-sidebar-bg px-4 pb-6 pt-0 shadow-[1px_0_0_var(--border)] md:flex">
      {/* The panel's warm stripe: red to yellow. */}
      <div className="-mx-4 h-1 shrink-0" style={{ background: "var(--brand-gradient)" }} aria-hidden />
      <div className="px-2 pt-1">
        <Brand />
      </div>
      <nav className="flex flex-col gap-4">
        <Link
          href="/"
          aria-current={pathname === "/" ? "page" : undefined}
          className={`flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-semibold transition-colors ${
            pathname === "/" ? "text-white shadow-card" : "text-text-secondary hover:bg-surface-2 hover:text-text-primary"
          }`}
          style={pathname === "/" ? { background: "var(--brand-hot)" } : undefined}
        >
          <span style={pathname === "/" ? undefined : { color: "var(--cat-gitlab)" }} className="flex">
            <Icon name="home" />
          </span>
          Overview
        </Link>
        {categories.map((category) => (
          <div key={category.name} className="flex flex-col gap-1" style={categoryVars(category.icon) as React.CSSProperties}>
            <span className="flex items-center gap-2 px-2 pb-0.5 text-[11px] font-bold uppercase tracking-[0.08em] text-[var(--cat-ink)]">
              <span className="grid h-6 w-6 place-items-center rounded-lg bg-[var(--cat-soft)] text-[var(--cat)]">
                <Icon name={category.icon} size={14} />
              </span>
              {category.name}
            </span>
            <div className="ml-[19px] flex flex-col gap-0.5 border-l-2 pl-2" style={{ borderColor: "color-mix(in srgb, var(--cat) 35%, transparent)" }}>
              {category.items.map((item) => {
                const active = item.href === activeHref(pathname);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={active ? "page" : undefined}
                    className={`rounded-xl px-3 py-1.5 text-sm transition-colors ${
                      active
                        ? "bg-[var(--cat-soft)] font-bold text-[var(--cat-ink)]"
                        : "font-medium text-text-secondary hover:bg-[var(--cat-soft)] hover:text-[var(--cat-ink)]"
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
      <AuthorCredit className="mt-auto border-t border-border px-3 pt-4" />
    </aside>
  );
}
