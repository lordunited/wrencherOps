// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(
  _request: NextRequest,
  ctx: RouteContext<"/api/services/[id]/logs/[logId]">,
) {
  const { id, logId } = await ctx.params;
  return relayJSON(
    await proxyToBackend(
      `/api/services/${encodeURIComponent(id)}/logs/${encodeURIComponent(logId)}`,
    ),
  );
}
