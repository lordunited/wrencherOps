import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(
  request: NextRequest,
  ctx: RouteContext<"/api/services/[id]/down">,
) {
  const { id } = await ctx.params;
  const body = await request.text();
  return relayJSON(
    await proxyToBackend(`/api/services/${id}/down`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    }),
  );
}
