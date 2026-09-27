"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Brand } from "@/components/Brand";
import { Icon } from "@/components/icons";
import { NAV_CATEGORIES } from "@/lib/nav";

// Desktop navigation. On phones the MobileNav tab bar replaces it.
export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="sticky top-0 hidden h-dvh w-64 shrink-0 flex-col gap-6 overflow-y-auto bg-sidebar-bg px-4 py-6 shadow-[1px_0_0_var(--border)] md:flex">
      <div className="px-2">
        <Brand />
      </div>
      <nav className="flex flex-col gap-5">
        <Link
          href="/"
          className={`flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-semibold transition-colors ${
            pathname === "/"
              ? "bg-accent text-on-accent"
              : "text-text-secondary hover:bg-surface-2 hover:text-text-primary"
          }`}
        >
          <Icon name="home" />
          Overview
        </Link>
        {NAV_CATEGORIES.map((category) => (
          <div key={category.name} className="flex flex-col gap-1">
            <span className="flex items-center gap-2 px-3 pb-1 text-[11px] font-bold uppercase tracking-[0.08em] text-text-muted">
              <Icon name={category.icon} size={14} />
              {category.name}
            </span>
            {category.items.map((item) => {
              const active = pathname === item.href || pathname.startsWith(item.href + "/");
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={`rounded-xl px-3 py-2 text-sm font-medium transition-colors ${
                    active
                      ? "bg-accent font-semibold text-on-accent shadow-card"
                      : "text-text-secondary hover:bg-surface-2 hover:text-text-primary"
                  }`}
                >
                  {item.label}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>
    </aside>
  );
}
