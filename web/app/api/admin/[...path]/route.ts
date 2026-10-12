// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// Users, groups and access: every /api/admin/... call goes to the backend
// as it came, where only admins get through.
async function relay(request: NextRequest, ctx: RouteContext<"/api/admin/[...path]">) {
  const { path } = await ctx.params;
  const body = request.method === "GET" || request.method === "DELETE" ? undefined : await request.text();
  return relayJSON(
    await proxyToBackend(`/api/admin/${path.map(encodeURIComponent).join("/")}`, {
      method: request.method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body,
    }),
  );
}

export { relay as GET, relay as POST, relay as PUT, relay as DELETE };
