// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

type Ctx = RouteContext<"/api/terraform/workspaces/[id]/variables">;

export async function GET(_request: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}/variables`));
}

export async function PUT(request: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}/variables`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: await request.text() }));
}
