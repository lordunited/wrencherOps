import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(_request: NextRequest, ctx: RouteContext<"/api/services/[id]/logs">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/services/${encodeURIComponent(id)}/logs`));
}
