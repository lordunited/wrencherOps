// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";

const TABS = [
  { key: "servers", href: "/servers", label: "Servers" },
  { key: "notifications", href: "/servers/notifications", label: "Notifications" },
] as const;

export function ServersTabs({ active, alertCount }: { active: (typeof TABS)[number]["key"]; alertCount?: number }) {
  return (
    <nav className="inline-flex w-fit gap-1 rounded-full bg-surface-1 p-1 shadow-card" aria-label="Servers">
      {TABS.map((t) => (
        <Link
          key={t.key}
          href={t.href}
          aria-current={active === t.key ? "page" : undefined}
          className={`flex items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-semibold transition-colors ${
            active === t.key ? "bg-accent text-on-accent" : "text-text-secondary hover:text-text-primary"
          }`}
        >
          {t.label}
          {t.key === "notifications" && !!alertCount && (
            <span className="rounded-full bg-[var(--status-critical)] px-1.5 text-[11px] font-bold leading-4 text-white">{alertCount}</span>
          )}
        </Link>
      ))}
    </nav>
  );
}
