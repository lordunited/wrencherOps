import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(_request: NextRequest, ctx: RouteContext<"/api/services/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/services/${id}`));
}

export async function PUT(request: NextRequest, ctx: RouteContext<"/api/services/[id]">) {
  const { id } = await ctx.params;
  const body = await request.text();
  return relayJSON(
    await proxyToBackend(`/api/services/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body,
    }),
  );
}

export async function DELETE(_request: NextRequest, ctx: RouteContext<"/api/services/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/services/${id}`, { method: "DELETE" }));
}
