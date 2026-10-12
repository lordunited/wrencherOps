// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { GroupEditor } from "@/components/AdminUsers";
import { getPanelGroup, getPanelTabs, getPanelUsers } from "@/lib/api";

export default async function AdminGroupPage({ params }: PageProps<"/admin/groups/[id]">) {
  const { id } = await params;
  const [group, tabs, users] = await Promise.all([getPanelGroup(id), getPanelTabs(), getPanelUsers()]);

  return (
    <div className="flex flex-col gap-5">
      <div>
        <Link href="/admin" className="text-sm text-text-secondary hover:text-text-primary">
          ← Users &amp; access
        </Link>
        <h1 className="page-title mt-1">{group.name}</h1>
      </div>
      <GroupEditor
        initial={group}
        tabs={tabs}
        users={users.filter((u) => !u.builtin).map((u) => ({ username: u.username, role: u.role }))}
      />
    </div>
  );
}
