// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployKubernetesListPage() {
  const services = (await getServices()).filter((s) => s.type === "kubernetes");

  return (
    <ServicesTable
      title="Kubernetes"
      description="Services deployed from your own manifest via kubectl apply, using whatever cluster access is already configured on the target."
      services={services}
      basePath="/deploy/kubernetes"
      newHref="/deploy/kubernetes/new"
      newLabel="+ New Kubernetes service"
    />
  );
}
