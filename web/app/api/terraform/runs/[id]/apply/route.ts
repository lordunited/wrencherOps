// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(request: NextRequest, ctx: RouteContext<"/api/terraform/runs/[id]/apply">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/terraform/runs/${encodeURIComponent(id)}/apply`, { method: "POST", headers: { "Content-Type": "application/json" }, body: await request.text() }));
}
