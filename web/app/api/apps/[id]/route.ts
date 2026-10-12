// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(_request: NextRequest, ctx: RouteContext<"/api/apps/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/apps/${encodeURIComponent(id)}`));
}

export async function PUT(request: NextRequest, ctx: RouteContext<"/api/apps/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(
    await proxyToBackend(`/api/apps/${encodeURIComponent(id)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}

export async function DELETE(_request: NextRequest, ctx: RouteContext<"/api/apps/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/apps/${encodeURIComponent(id)}`, { method: "DELETE" }));
}
