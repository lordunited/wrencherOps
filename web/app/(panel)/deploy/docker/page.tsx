// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployDockerListPage() {
  const services = (await getServices()).filter((s) => s.type === "docker" && !s.appId);

  return (
    <ServicesTable
      title="Docker Compose"
      description="Services deployed from your own docker-compose.yml — no generation, applied as-is."
      services={services}
      basePath="/deploy/docker"
      newHref="/deploy/docker/new"
      newLabel="+ New Docker service"
    />
  );
}
