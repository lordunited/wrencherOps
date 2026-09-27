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
        description="Monitoring stacks deployed by the panel, and existing Prometheus servers connected to it."
        services={services}
        basePath="/deploy/prometheus"
        newHref="/deploy/prometheus/new"
        newLabel="+ New Prometheus stack"
        extraAction={{ href: "/deploy/prometheus/connect", label: "Connect existing" }}
      />
    </div>
  );
}
