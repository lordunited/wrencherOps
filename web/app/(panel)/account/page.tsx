// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { ChangePasswordForm, ROLE_INFO } from "@/components/AdminUsers";
import { getMe } from "@/lib/api";
import { NAV_CATEGORIES } from "@/lib/nav";

// Your account: who you are, what you can do, and your password.
export default async function AccountPage() {
  const me = await getMe();
  const rows = NAV_CATEGORIES.flatMap((c) => c.items.map((i) => ({ ...i, category: c.name, level: me.tabs[i.tab] })));

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="page-title">Your account</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Signed in as <b className="font-mono text-text-primary">{me.username}</b>, <b className="capitalize">{me.role}</b>:{" "}
          {ROLE_INFO[me.role]}
        </p>
      </div>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="text-base font-bold text-text-primary">Password</h2>
        {me.builtin ? (
          <p className="text-sm text-text-secondary">
            This is a built-in admin account: its password is set in the panel&apos;s <code className="font-mono">AUTH_USERS</code>{" "}
            setting (a bcrypt hash from <code className="font-mono">cmd/hashpassword</code>), not here.
          </p>
        ) : (
          <ChangePasswordForm />
        )}
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="text-base font-bold text-text-primary">What you can do</h2>
        <ul className="grid gap-1.5 sm:grid-cols-2">
          {rows.map((r) => (
            <li key={r.tab} className="flex items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-sm">
              {r.level ? (
                <Link href={r.href} className="min-w-0 flex-1 truncate text-text-primary hover:underline">
                  <span className="text-text-muted">{r.category} › </span>
                  {r.label}
                </Link>
              ) : (
                <span className="min-w-0 flex-1 truncate text-text-muted">
                  {r.category} › {r.label}
                </span>
              )}
              <span className={`text-xs font-semibold ${r.level === "write" ? "text-accent" : r.level ? "text-text-secondary" : "text-text-muted"}`}>
                {r.level ?? "no access"}
              </span>
            </li>
          ))}
        </ul>
        {me.role !== "admin" && <p className="text-xs text-text-muted">An admin can change this in Admin › Users &amp; access.</p>}
      </section>
    </div>
  );
}
