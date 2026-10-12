// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { UserEditor } from "@/components/AdminUsers";
import { getPanelGroups, getPanelTabs, getPanelUser } from "@/lib/api";

export default async function AdminUserPage({ params }: PageProps<"/admin/users/[username]">) {
  const { username } = await params;
  const [user, tabs, groups] = await Promise.all([getPanelUser(decodeURIComponent(username)), getPanelTabs(), getPanelGroups()]);

  return (
    <div className="flex flex-col gap-5">
      <div>
        <Link href="/admin" className="text-sm text-text-secondary hover:text-text-primary">
          ← Users &amp; access
        </Link>
        <h1 className="page-title mt-1 font-mono">{user.username}</h1>
      </div>
      <UserEditor key={user.updatedAt} initial={user} tabs={tabs} groups={groups} />
    </div>
  );
}
