// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

type Ctx = RouteContext<"/api/terraform/workspaces/[id]">;

export async function GET(_request: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}`));
}

export async function PUT(request: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: await request.text() }));
}

export async function DELETE(request: NextRequest, ctx: Ctx) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}${request.nextUrl.search}`, { method: "DELETE" }));
}
