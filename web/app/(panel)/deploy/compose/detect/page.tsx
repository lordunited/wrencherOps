// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ComposeTabs } from "@/components/ComposeTabs";
import { DetectProjects } from "@/components/DetectProjects";
import { getServices } from "@/lib/api";
import { knownServers } from "@/lib/servers";

export default async function DetectComposePage(props: PageProps<"/deploy/compose/detect">) {
  const { host, port, user } = await props.searchParams;
  const one = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v);
  const known = knownServers(await getServices());

  return (
    <div className="flex flex-col gap-6">
      <ComposeTabs active="detect" />
      <DetectProjects initial={{ host: one(host), port: one(port), user: one(user) }} known={known} />
    </div>
  );
}
