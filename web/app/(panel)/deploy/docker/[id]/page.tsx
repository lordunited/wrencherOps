// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServiceDetail } from "@/components/ServiceDetail";
import { getService } from "@/lib/api";

export default async function DockerServiceDetailPage(props: PageProps<"/deploy/docker/[id]">) {
  const { id } = await props.params;
  const service = await getService(id);

  return <ServiceDetail initial={service} backHref="/deploy/docker" backLabel="Docker Compose" />;
}
