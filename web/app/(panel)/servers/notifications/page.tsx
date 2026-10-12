// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NotificationsManager } from "@/components/NotificationsManager";
import { ServersTabs } from "@/components/ServersTabs";
import { getServerAlerts, getServerRules, getWatchedServers } from "@/lib/api";

export default async function ServerNotificationsPage() {
  const [alerts, { rules, kinds }, servers] = await Promise.all([getServerAlerts(true), getServerRules(), getWatchedServers()]);
  return (
    <div className="flex flex-col gap-5">
      <ServersTabs active="notifications" alertCount={alerts.filter((a) => !a.resolvedAt).length} />
      <div>
        <h1 className="page-title">Notifications</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Alerts on your servers, and the rules that raise them: failed logins, disks near full, too many connections,
          kernel errors, load, memory, and servers that can&apos;t be reached. New alerts pop up on every page.
        </p>
      </div>
      <NotificationsManager
        initialAlerts={alerts}
        initialRules={rules}
        kinds={kinds}
        servers={servers.map((s) => ({ id: s.id, name: s.name }))}
      />
    </div>
  );
}
