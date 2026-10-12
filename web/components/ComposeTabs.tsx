// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";

const TABS = [
  { key: "apps", href: "/deploy/compose", label: "Apps" },
  { key: "detect", href: "/deploy/compose/detect", label: "Detect on a server" },
] as const;

export function ComposeTabs({ active }: { active: (typeof TABS)[number]["key"] }) {
  return (
    <nav className="inline-flex w-fit gap-1 rounded-full bg-surface-1 p-1 shadow-card" aria-label="Custom compose">
      {TABS.map((t) => (
        <Link
          key={t.key}
          href={t.href}
          aria-current={active === t.key ? "page" : undefined}
          className={`rounded-full px-4 py-1.5 text-sm font-semibold transition-colors ${
            active === t.key ? "bg-accent text-on-accent" : "text-text-secondary hover:text-text-primary"
          }`}
        >
          {t.label}
        </Link>
      ))}
    </nav>
  );
}
