import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(request: NextRequest) {
  return relayJSON(
    await proxyToBackend("/api/services/nginx", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: await request.text(),
    }),
  );
}
