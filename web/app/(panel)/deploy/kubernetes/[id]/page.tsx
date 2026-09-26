import { ServiceDetail } from "@/components/ServiceDetail";
import { getService } from "@/lib/api";

export default async function KubernetesServiceDetailPage(
  props: PageProps<"/deploy/kubernetes/[id]">,
) {
  const { id } = await props.params;
  const service = await getService(id);

  return <ServiceDetail initial={service} backHref="/deploy/kubernetes" backLabel="Kubernetes" />;
}
