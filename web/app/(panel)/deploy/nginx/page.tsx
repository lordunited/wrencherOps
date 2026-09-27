import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployNginxListPage() {
  const services = (await getServices()).filter((s) => s.type === "nginx");

  return (
    <ServicesTable
      title="Nginx"
      description="Nginx servers deployed by the panel, and existing ones connected to it. Manage their configuration: server blocks, upstreams and config files."
      services={services}
      basePath="/deploy/nginx"
      newHref="/deploy/nginx/new"
      newLabel="+ New Nginx"
      extraAction={{ href: "/deploy/nginx/connect", label: "Connect existing" }}
    />
  );
}
