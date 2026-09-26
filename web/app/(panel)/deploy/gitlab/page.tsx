import { DeployTabs } from "@/components/DeployTabs";
import { ServicesTable } from "@/components/ServicesTable";
import { getServices } from "@/lib/api";

export default async function DeployGitLabListPage() {
  const services = (await getServices()).filter((s) => s.type === "gitlab");

  return (
    <div className="flex flex-col gap-6">
      <DeployTabs basePath="/deploy/gitlab" active="docker" />
      <ServicesTable
        title="GitLab"
        description="GitLab instances deployed or configured to be deployed via Docker."
        services={services}
        basePath="/deploy/gitlab"
        newHref="/deploy/gitlab/new"
        newLabel="+ New GitLab service"
      />
    </div>
  );
}
