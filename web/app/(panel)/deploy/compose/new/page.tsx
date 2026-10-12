// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NewComposeAppForm } from "@/components/NewComposeAppForm";
import { getServices } from "@/lib/api";
import { knownServers } from "@/lib/servers";

export default async function NewComposeAppPage(props: PageProps<"/deploy/compose/new">) {
  const { server } = await props.searchParams;
  const known = knownServers(await getServices());

  return <NewComposeAppForm known={known} preselect={Array.isArray(server) ? server[0] : server} />;
}
