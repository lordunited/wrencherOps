// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(request: NextRequest) {
  const body = await request.text();
  return relayJSON(
    await proxyToBackend("/api/services/kubernetes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    }),
  );
}
