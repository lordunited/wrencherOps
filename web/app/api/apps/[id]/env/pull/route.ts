// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// Makes one server's .env the app's.
export async function POST(request: NextRequest, ctx: RouteContext<"/api/apps/[id]/env/pull">) {
  const { id } = await ctx.params;
  return relayJSON(
    await proxyToBackend(`/api/apps/${encodeURIComponent(id)}/env/pull`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}
