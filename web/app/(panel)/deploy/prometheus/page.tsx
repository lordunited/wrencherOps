import { DeployTabs } from "@/components/DeployTabs";
import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployPrometheusListPage() {
  const services = (await getServices()).filter((s) => s.type === "prometheus");

  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/prometheus" active="docker" />
      <ServicesTable
        title="Prometheus stack"
        description="Prometheus, Grafana, Alertmanager, and node_exporter, deployed or configured to be deployed via Docker."
        services={services}
        basePath="/deploy/prometheus"
        newHref="/deploy/prometheus/new"
        newLabel="+ New Prometheus stack"
      />
    </div>
  );
}
