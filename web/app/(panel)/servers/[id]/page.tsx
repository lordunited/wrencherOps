// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { ServerDetail } from "@/components/ServerDetail";
import { getWatchedServer } from "@/lib/api";

export default async function ServerPage(props: PageProps<"/servers/[id]">) {
  const { id } = await props.params;
  const server = await getWatchedServer(id);
  return <ServerDetail key={server.id} initial={server} />;
}
