import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(_request: NextRequest, ctx: RouteContext<"/api/deploy/jobs/[id]">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/deploy/jobs/${id}`));
}
