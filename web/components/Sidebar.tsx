"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Icon } from "@/components/icons";
import { NAV_CATEGORIES } from "@/lib/nav";

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="w-60 shrink-0 border-r border-border bg-sidebar-bg px-3 py-5">
      <Link href="/" className="flex items-center gap-2 px-2 pb-6">
        <span className="h-2 w-2 rounded-full bg-accent" aria-hidden />
        <span className="text-sm font-semibold tracking-wide text-text-primary">
          DevOps Panel
        </span>
      </Link>
      <nav className="flex flex-col gap-4">
        {NAV_CATEGORIES.map((category) => (
          <details key={category.name} open className="group">
            <summary className="flex cursor-pointer select-none items-center gap-2 rounded px-2 py-1.5 text-xs font-semibold uppercase tracking-wider text-text-muted hover:text-text-secondary">
              <Icon name={category.icon} className="text-text-muted" />
              {category.name}
            </summary>
            <div className="mt-1 flex flex-col gap-0.5">
              {category.items.map((item) => {
                const active = pathname === item.href;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={`relative rounded px-2 py-1.5 pl-5 text-sm font-medium transition-colors ${
                      active
                        ? "bg-surface-hover text-text-primary"
                        : "text-text-secondary hover:bg-surface-1 hover:text-text-primary"
                    }`}
                  >
                    {active && (
                      <span
                        className="absolute left-0 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-accent"
                        aria-hidden
                      />
                    )}
                    {item.label}
                  </Link>
                );
              })}
            </div>
          </details>
        ))}
      </nav>
    </aside>
  );
}
