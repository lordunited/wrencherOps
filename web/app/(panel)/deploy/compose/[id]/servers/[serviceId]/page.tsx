// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServiceDetail } from "@/components/ServiceDetail";
import { getApp, getService } from "@/lib/api";

export default async function ComposeAppServerPage(props: PageProps<"/deploy/compose/[id]/servers/[serviceId]">) {
  const { id, serviceId } = await props.params;
  const [app, service] = await Promise.all([getApp(id), getService(serviceId)]);

  return <ServiceDetail initial={service} backHref={`/deploy/compose/${app.id}`} backLabel={app.name} />;
}
