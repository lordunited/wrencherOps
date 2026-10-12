// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployEnvoyListPage() {
  const services = (await getServices()).filter((s) => s.type === "envoy");

  return (
    <ServicesTable
      title="Envoy"
      description="Envoy proxies deployed by the panel, and existing ones connected to it: their routes, clusters and live endpoint health, with config changes validated before they're applied."
      services={services}
      basePath="/deploy/envoy"
      newHref="/deploy/envoy/new"
      newLabel="+ New Envoy"
      extraAction={{ href: "/deploy/envoy/connect", label: "Connect existing" }}
    />
  );
}
