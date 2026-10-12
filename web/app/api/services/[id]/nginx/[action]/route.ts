// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { NextResponse, type NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

const ACTIONS = new Set(["inspect", "preview", "apply"]);

export async function POST(request: NextRequest, ctx: RouteContext<"/api/services/[id]/nginx/[action]">) {
  const { id, action } = await ctx.params;
  if (!ACTIONS.has(action)) {
    return NextResponse.json({ error: "unknown action" }, { status: 404 });
  }
  return relayJSON(
    await proxyToBackend(`/api/services/${encodeURIComponent(id)}/nginx/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}

// The last saved reading, shown while a fresh one loads.
export async function GET(request: NextRequest, ctx: RouteContext<"/api/services/[id]/nginx/[action]">) {
  const { id, action } = await ctx.params;
  if (action !== "snapshot") {
    return NextResponse.json({ error: "unknown action" }, { status: 404 });
  }
  return relayJSON(await proxyToBackend(`/api/services/${encodeURIComponent(id)}/nginx/snapshot${request.nextUrl.search}`));
}
