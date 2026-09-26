import { ServiceDetail } from "@/components/ServiceDetail";
import { getService } from "@/lib/api";

export default async function GitLabServiceDetailPage(props: PageProps<"/deploy/gitlab/[id]">) {
  const { id } = await props.params;
  const service = await getService(id);

  return <ServiceDetail initial={service} backHref="/deploy/gitlab" backLabel="GitLab" />;
}
