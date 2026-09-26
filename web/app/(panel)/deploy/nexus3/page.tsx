import { DeployTabs } from "@/components/DeployTabs";
import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployNexus3ListPage() {
  const services = (await getServices()).filter((s) => s.type === "nexus3");

  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/nexus3" active="docker" />
      <ServicesTable
        title="Nexus3"
        description="Sonatype Nexus Repository instances deployed or configured to be deployed via Docker."
        services={services}
        basePath="/deploy/nexus3"
        newHref="/deploy/nexus3/new"
        newLabel="+ New Nexus3 service"
      />
    </div>
  );
}
