// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Browser-side helper; kept out of lib/api.ts, which uses server-only
// Next.js APIs and can't be imported into client components.

// The last saved Prometheus reading of a kind, or null when there is none.
export async function loadSnapshot<T>(
  serviceId: string,
  kind: "inspect" | "status" | "nginx" | "haproxy" | "envoy",
): Promise<{ data: T; fetchedAt: string } | null> {
  const area = kind === "nginx" || kind === "haproxy" || kind === "envoy" ? kind : "prometheus";
  try {
    const res = await fetch(`/api/services/${serviceId}/${area}/snapshot?kind=${kind}`, { cache: "no-store" });
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}
