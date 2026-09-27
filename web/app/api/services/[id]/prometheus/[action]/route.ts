import { NextResponse, type NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

const ACTIONS = new Set(["inspect", "preview", "apply", "move-to-disk", "status"]);

export async function POST(
  request: NextRequest,
  ctx: RouteContext<"/api/services/[id]/prometheus/[action]">,
) {
  const { id, action } = await ctx.params;
  if (!ACTIONS.has(action)) {
    return NextResponse.json({ error: "unknown action" }, { status: 404 });
  }
  const body = await request.text();
  return relayJSON(
    await proxyToBackend(`/api/services/${encodeURIComponent(id)}/prometheus/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    }),
  );
}

// The last saved reading (?kind=inspect|status), shown while a fresh one loads.
export async function GET(
  request: NextRequest,
  ctx: RouteContext<"/api/services/[id]/prometheus/[action]">,
) {
  const { id, action } = await ctx.params;
  if (action !== "snapshot") {
    return NextResponse.json({ error: "unknown action" }, { status: 404 });
  }
  return relayJSON(
    await proxyToBackend(`/api/services/${encodeURIComponent(id)}/prometheus/snapshot${request.nextUrl.search}`),
  );
}
