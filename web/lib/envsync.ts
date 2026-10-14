// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { EnvSync } from "@/lib/api";
import type { StatusRole } from "@/lib/format";

// ENV_SYNC_LABEL is how each .env comparison is shown.
export const ENV_SYNC_LABEL: Record<EnvSync["status"], { text: string; role: StatusRole; hint: string }> = {
  "in-sync": { text: "in sync", role: "good", hint: "The server's .env is the app's." },
  differs: {
    text: "differs",
    role: "warning",
    hint: "The server's .env isn't the app's: it was changed on the server, or the app's was changed and not deployed yet.",
  },
  "not-on-server": { text: "not on the server", role: "warning", hint: "The app has a .env the server doesn't: the next deploy writes it." },
  "only-on-server": { text: "only on the server", role: "warning", hint: "The server has a .env the app doesn't keep: take it into the app, or it stays as it is." },
  unreachable: { text: "couldn't check", role: "critical", hint: "" },
};
