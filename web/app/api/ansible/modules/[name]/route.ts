// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(_request: NextRequest, ctx: RouteContext<"/api/ansible/modules/[name]">) {
  const { name } = await ctx.params;
  return relayJSON(await proxyToBackend(`/api/ansible/modules/${encodeURIComponent(name)}`));
}
