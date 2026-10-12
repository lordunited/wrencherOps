// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { DeployTabs } from "@/components/DeployTabs";

export default function DeployNexus3KubernetesPage() {
  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/nexus3" active="kubernetes" />
      <div>
        <h1 className="page-title">Nexus3 on Kubernetes</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Deploying Nexus3 to a Kubernetes cluster, generated the same way the Docker tab does.
        </p>
      </div>
      <p className="card px-4 py-10 text-center text-sm text-text-muted">
        Not built yet — coming soon.
      </p>
    </div>
  );
}
