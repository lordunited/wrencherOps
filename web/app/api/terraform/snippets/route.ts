// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

// Adds a concept's example (or a provider) to the editor's files; the
// editor shows the result to save.
export async function POST(request: NextRequest) {
  return relayJSON(
    await proxyToBackend("/api/terraform/snippets", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}
