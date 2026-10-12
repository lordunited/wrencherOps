// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ComposeAppDetail } from "@/components/ComposeAppDetail";
import { getApp, getServices } from "@/lib/api";
import { knownServers } from "@/lib/servers";

export default async function ComposeAppPage(props: PageProps<"/deploy/compose/[id]">) {
  const { id } = await props.params;
  const [app, services] = await Promise.all([getApp(id), getServices()]);

  return <ComposeAppDetail key={app.updatedAt} initial={app} known={knownServers(services)} />;
}
