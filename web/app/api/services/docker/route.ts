import type { NextRequest } from "next/server";
import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function POST(request: NextRequest) {
  const body = await request.text();
  return relayJSON(
    await proxyToBackend("/api/services/docker", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    }),
  );
}
