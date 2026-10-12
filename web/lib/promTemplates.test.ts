// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { ALERT_TEMPLATES, JOB_PRESETS, parseTargets } from "@/lib/promTemplates";

describe("parseTargets", () => {
  it("reads addresses with notes, skipping blanks and comments", () => {
    expect(
      parseTargets(`
# the web tier
10.0.0.1:9100 # web-1
- "10.0.0.2:9100"
10.0.0.3:9100 db primary # main
`),
    ).toEqual([
      { address: "10.0.0.1:9100", comment: "web-1" },
      { address: "10.0.0.2:9100", comment: "" },
      { address: "10.0.0.3:9100", comment: "db primary main" },
    ]);
  });
});

describe("templates", () => {
  it("are complete and uniquely named", () => {
    expect(ALERT_TEMPLATES.length).toBeGreaterThan(0);
    expect(new Set(ALERT_TEMPLATES.map((t) => t.id)).size).toBe(ALERT_TEMPLATES.length);
    expect(new Set(ALERT_TEMPLATES.map((t) => t.alert)).size).toBe(ALERT_TEMPLATES.length);
    for (const t of ALERT_TEMPLATES) {
      expect(t.expr && t.summary && t.description, t.id).toBeTruthy();
      expect(t.for, t.id).toMatch(/^\d+[smh]$/);
    }
    expect(JOB_PRESETS.length).toBeGreaterThan(0);
  });
});
