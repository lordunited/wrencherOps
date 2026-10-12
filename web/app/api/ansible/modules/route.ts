// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET() {
  return relayJSON(await proxyToBackend("/api/ansible/modules"));
}
