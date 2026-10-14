// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// Compares the app's .env with every server's.
export async function GET(_request: NextRequest, ctx: RouteContext<"/api/apps/[id]/env/sync">) {
  const { id } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/apps/${encodeURIComponent(id)}/env/sync`));
}
