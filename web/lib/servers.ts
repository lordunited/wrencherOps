// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { Service } from "@/lib/api";

// A server the panel already knows, identified by one of its services so a
// new app can reuse that service's saved login (copied server-side; the
// browser never sees the password).
export interface KnownServer {
  serviceId: string;
  host: string;
  port: number;
  username: string;
  hasSavedPassword: boolean;
}

// knownServers lists each distinct host/port/user once, preferring a
// service whose password is saved, so reusing it needs no password typed.
export function knownServers(services: Service[]): KnownServer[] {
  const byKey = new Map<string, KnownServer>();
  for (const s of services) {
    const host = s.host.trim();
    const username = s.username.trim();
    const key = `${host}:${s.port}:${username}`;
    const existing = byKey.get(key);
    if (!existing || (!existing.hasSavedPassword && s.hasSavedPassword)) {
      byKey.set(key, { serviceId: s.id, host, port: s.port, username, hasSavedPassword: s.hasSavedPassword });
    }
  }
  return [...byKey.values()].sort((a, b) => a.host.localeCompare(b.host) || a.username.localeCompare(b.username));
}

export function serverLabel(k: Pick<KnownServer, "host" | "port" | "username">) {
  return `${k.username}@${k.host}${k.port !== 22 ? `:${k.port}` : ""}`;
}
