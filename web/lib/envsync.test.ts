// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { ENV_SYNC_LABEL } from "@/lib/envsync";

describe(".env sync labels", () => {
  // The statuses internal/service/envsync.go's envStatus and checkEnv return.
  it("cover every status the backend sends", () => {
    expect(Object.keys(ENV_SYNC_LABEL).sort()).toEqual(["differs", "in-sync", "not-on-server", "only-on-server", "unreachable"]);
  });
  it("only call in-sync good", () => {
    for (const [status, l] of Object.entries(ENV_SYNC_LABEL)) {
      expect(l.role === "good", status).toBe(status === "in-sync");
    }
  });
});
