"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { Icon, type IconName } from "@/components/icons";
import { NAV_CATEGORIES } from "@/lib/nav";

const TABS: { href: string; label: string; icon: IconName; match: (p: string) => boolean }[] = [
  { href: "/", label: "Overview", icon: "home", match: (p) => p === "/" },
  { href: "/health", label: "Health", icon: "pulse", match: (p) => p.startsWith("/health") },
  { href: "/projects", label: "Projects", icon: "folder", match: (p) => p.startsWith("/projects") },
  { href: "/deploy/gitlab", label: "Deploy", icon: "deploy", match: (p) => p.startsWith("/deploy") },
];

// Phone navigation: a floating tab bar within thumb reach, plus a "More"
// sheet listing every page. Hidden from md up, where the Sidebar takes over.
export function MobileNav() {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);

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
            {NAV_CATEGORIES.map((category) => (
              <div key={category.name} className="mb-3 last:mb-0">
                <span className="flex items-center gap-2 px-2 pb-1 text-[11px] font-bold uppercase tracking-[0.08em] text-text-muted">
                  <Icon name={category.icon} size={14} />
                  {category.name}
                </span>
                <div className="grid grid-cols-2 gap-1.5">
                  {category.items.map((item) => {
                    const active = pathname === item.href || pathname.startsWith(item.href + "/");
                    return (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={`rounded-xl px-3 py-2.5 text-sm font-semibold ${
                          active ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary"
                        }`}
                      >
                        {item.label}
                      </Link>
                    );
                  })}
                </div>
              </div>
            ))}
          </nav>
        </div>
      )}

      <nav
        aria-label="Main"
        className="fixed inset-x-3 bottom-[calc(0.75rem+env(safe-area-inset-bottom,0px))] z-50 flex items-center justify-around rounded-[22px] bg-tabbar-bg px-1.5 py-1.5 shadow-card md:hidden"
      >
        {TABS.map((tab) => {
          const active = !open && tab.match(pathname);
          return (
            <Link
              key={tab.href}
              href={tab.href}
              aria-current={active ? "page" : undefined}
              className={`flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-2xl py-2 text-[11px] font-semibold ${
                active ? "bg-white/10 text-white" : "text-tabbar-ink"
              }`}
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
