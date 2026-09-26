import { DeployTabs } from "@/components/DeployTabs";

export default function DeployPrometheusKubernetesPage() {
  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/prometheus" active="kubernetes" />
      <div>
        <h1 className="text-xl font-semibold text-text-primary">Prometheus stack on Kubernetes</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Deploying the monitoring stack to a Kubernetes cluster, generated the same way the
          Docker tab does.
        </p>
      </div>
      <p className="rounded-lg border border-border bg-surface-1 px-4 py-10 text-center text-sm text-text-muted">
        Not built yet — coming soon.
      </p>
    </div>
  );
}
