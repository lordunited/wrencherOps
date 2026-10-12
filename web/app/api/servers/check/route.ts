// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST() {
  return relayJSON(await proxyToBackend("/api/servers/check", { method: "POST" }));
}
