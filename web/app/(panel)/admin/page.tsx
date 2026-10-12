// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { NewGroupForm, NewUserForm } from "@/components/AdminUsers";
import { StatusBadge } from "@/components/StatusBadge";
import { getPanelGroups, getPanelTabs, getPanelUsers } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

const ROLE_BADGE: Record<string, StatusRole> = { admin: "serious", writer: "warning", viewer: "muted" };

// Users & access: who can sign in to the panel, the groups they're in, and
// how much of each tab they get.
export default async function AdminPage() {
  const [users, groups, tabs] = await Promise.all([getPanelUsers(), getPanelGroups(), getPanelTabs()]);
  const groupName = new Map(groups.map((g) => [g.id, g.name]));
  const grantable = tabs.filter((t) => !t.adminOnly && !t.everyone).length;

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="page-title">Users &amp; access</h1>
        <p className="mt-1 max-w-2xl text-sm text-text-secondary">
          Who can sign in, and what they can do. An <b>admin</b> can do everything; a <b>writer</b> gets exactly the access
          given, per tab; a <b>viewer</b> can only look. Access is set per tab — on the user, or on a group they&apos;re in — and
          checked by the backend on every request.
        </p>
      </div>

      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-bold text-text-primary">Users</h2>
          <NewUserForm groups={groups} />
        </div>
        <ul className="card flex flex-col divide-y divide-border overflow-hidden">
          {users.map((u) => (
            <li key={u.username}>
              <Link href={`/admin/users/${encodeURIComponent(u.username)}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-surface-hover">
                <span className="grid h-8 w-8 place-items-center rounded-full bg-accent-soft text-xs font-bold uppercase text-accent" aria-hidden>
                  {u.username.slice(0, 1)}
                </span>
                <span className="font-mono text-sm font-semibold text-text-primary">{u.username}</span>
                <StatusBadge role={ROLE_BADGE[u.role] ?? "muted"} label={u.role} />
                {u.builtin && <span className="text-xs text-text-muted">built-in</span>}
                {u.disabled && <StatusBadge role="critical" label="disabled" />}
                <span className="ml-auto flex flex-wrap gap-1">
                  {u.groups.map((g) => (
                    <span key={g} className="rounded-full bg-surface-2 px-2 py-0.5 text-[11px] text-text-secondary">
                      {groupName.get(g) ?? g}
                    </span>
                  ))}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-lg font-bold text-text-primary">Groups</h2>
          <NewGroupForm />
        </div>
        {groups.length === 0 ? (
          <p className="card px-4 py-6 text-sm text-text-secondary">
            No groups yet. A group gives the same access to everyone in it — a team, say.
          </p>
        ) : (
          <ul className="card flex flex-col divide-y divide-border overflow-hidden">
            {groups.map((g) => {
              const given = Object.keys(g.grants).length;
              return (
                <li key={g.id}>
                  <Link href={`/admin/groups/${encodeURIComponent(g.id)}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-surface-hover">
                    <span className="text-sm font-semibold text-text-primary">{g.name}</span>
                    {g.description && <span className="text-xs text-text-muted">{g.description}</span>}
                    <span className="ml-auto text-xs text-text-secondary">
                      {g.members.length} member{g.members.length === 1 ? "" : "s"} · access to {given} of {grantable} tabs
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </div>
  );
}
