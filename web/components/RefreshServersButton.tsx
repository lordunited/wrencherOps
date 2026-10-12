// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import type { WatchedServer } from "@/lib/api";

// Checks every server now, rather than waiting for the next minute's check.
export function RefreshServersButton() {
  const router = useRouter();
  const [busy, setBusy] = useState(false);

  async function refresh() {
    setBusy(true);
    try {
      const res = await fetch("/api/servers/check", { method: "POST" });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const servers: WatchedServer[] = await res.json();
      const failed = servers.filter((s) => s.checkError).length;
      if (failed > 0) toast.warning(`Checked ${servers.length} servers; ${failed} couldn't be reached.`);
      else toast.success(`Checked ${servers.length} server${servers.length === 1 ? "" : "s"}.`);
      router.refresh();
    } finally {
      setBusy(false);
    }
  }

  return (
    <Button variant="secondary" onClick={refresh} disabled={busy}>
      {busy ? "Checking all…" : "↻ Refresh all"}
    </Button>
  );
}
