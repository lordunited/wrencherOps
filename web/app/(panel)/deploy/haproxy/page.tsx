// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { DeployTabs } from "@/components/DeployTabs";
import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployHAProxyListPage() {
  const services = (await getServices()).filter((s) => s.type === "haproxy");

  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/haproxy" active="docker" />
      <ServicesTable
        title="HAProxy"
        description="HAProxy load balancers deployed by the panel, and existing ones connected to it."
        services={services}
        basePath="/deploy/haproxy"
        newHref="/deploy/haproxy/new"
        newLabel="+ New HAProxy"
        extraAction={{ href: "/deploy/haproxy/connect", label: "Connect existing" }}
      />
    </div>
  );
}
