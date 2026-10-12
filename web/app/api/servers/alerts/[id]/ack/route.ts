// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(_request: NextRequest, ctx: RouteContext<"/api/servers/alerts/[id]/ack">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/servers/alerts/${encodeURIComponent(id)}/ack`, { method: "POST" }));
}
