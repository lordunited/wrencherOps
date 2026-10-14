// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// Writes the app's .env to one server.
export async function POST(request: NextRequest, ctx: RouteContext<"/api/apps/[id]/env/push">) {
  const { id } = await ctx.params;
  return relayJSON(
    await proxyToBackend(`/api/apps/${encodeURIComponent(id)}/env/push`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}
