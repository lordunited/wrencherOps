// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET(request: NextRequest) {
  const all = request.nextUrl.searchParams.get("all") === "1";
  return relayJSON(await proxyToBackend(`/api/servers/alerts${all ? "?all=1" : ""}`));
}
