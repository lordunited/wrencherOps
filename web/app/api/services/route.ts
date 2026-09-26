import { proxyToBackend, relayJSON } from "@/lib/backend";

export async function GET() {
  return relayJSON(await proxyToBackend("/api/services"));
}
