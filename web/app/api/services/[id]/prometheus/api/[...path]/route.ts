import type { NextRequest } from "next/server";
import { proxyToBackend } from "@/lib/backend";

// Read-only Prometheus API calls for the PromQL editor (autocomplete and
// query checks). The backend decides which calls are allowed; Prometheus's
// status and JSON body are passed through as-is so errors keep their message.
export async function GET(
  request: NextRequest,
  ctx: RouteContext<"/api/services/[id]/prometheus/api/[...path]">,
) {
  const { id, path } = await ctx.params;
  const rest = path.map(encodeURIComponent).join("/");
  const res = await proxyToBackend(
    `/api/services/${encodeURIComponent(id)}/prometheus/api/${rest}${request.nextUrl.search}`,
  );
  return new Response(await res.text(), {
    status: res.status,
    headers: { "Content-Type": res.headers.get("Content-Type") ?? "application/json" },
  });
}
