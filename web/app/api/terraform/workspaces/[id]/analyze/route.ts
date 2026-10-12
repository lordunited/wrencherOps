// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// The workspace's code checked against the standard: the saved code, or
// the files in the body (the editor's, unsaved).
export async function POST(request: NextRequest, ctx: RouteContext<"/api/terraform/workspaces/[id]/analyze">) {
  const { id } = await ctx.params;
  return relayJSON(
    await proxyToBackend(`/api/terraform/workspaces/${encodeURIComponent(id)}/analyze`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}
