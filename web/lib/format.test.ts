// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  accessLevelLabel,
  bytesLabel,
  compactNumber,
  formatDuration,
  logStatusRole,
  pipelineStatusRole,
  serviceHealthRole,
  serviceStatusRole,
  timeAgo,
  uptimeLabel,
  usageRole,
} from "@/lib/format";

describe("roles", () => {
  it("color statuses, and anything unknown is muted", () => {
    expect(pipelineStatusRole("failed")).toBe("critical");
    expect(pipelineStatusRole("success")).toBe("good");
    expect(pipelineStatusRole("something new")).toBe("muted");
    expect(serviceStatusRole("deployed")).toBe("good");
    expect(serviceStatusRole("failed")).toBe("critical");
    expect(serviceHealthRole("paused")).toBe("warning");
    expect(serviceHealthRole("unreachable")).toBe("critical");
    expect(serviceHealthRole("not deployed")).toBe("muted");
    expect(logStatusRole("running")).toBe("warning");
  });
  it("label GitLab access levels", () => {
    expect(accessLevelLabel(40)).toBe("Maintainer");
    expect(accessLevelLabel(99)).toBe("Level 99");
  });
});

describe("usageRole", () => {
  it("warns from 85% and is critical from 95%", () => {
    expect(usageRole(50)).toBe("good");
    expect(usageRole(85)).toBe("warning");
    expect(usageRole(94.9)).toBe("warning");
    expect(usageRole(95)).toBe("critical");
    expect(usageRole(70, 60, 80)).toBe("warning");
  });
});

describe("sizes and durations", () => {
  it("bytesLabel uses binary units", () => {
    expect(bytesLabel(0)).toBe("0 B");
    expect(bytesLabel(1023)).toBe("1023 B");
    expect(bytesLabel(1024)).toBe("1 KiB");
    expect(bytesLabel(1536)).toBe("1.5 KiB");
    expect(bytesLabel(500 * 1024 * 1024)).toBe("500 MiB");
    expect(bytesLabel(NaN)).toBe("0 B");
  });
  it("formatDuration", () => {
    expect(formatDuration("2026-01-01T00:00:00Z")).toBe("");
    expect(formatDuration("2026-01-01T00:00:00Z", "2026-01-01T00:00:04Z")).toBe("4s");
    expect(formatDuration("2026-01-01T00:00:00Z", "2026-01-01T00:02:13Z")).toBe("2m 13s");
    expect(formatDuration("2026-01-01T00:00:00Z", "2026-01-01T03:05:00Z")).toBe("3h 5m");
  });
  it("uptimeLabel", () => {
    expect(uptimeLabel(59)).toBe("0m");
    expect(uptimeLabel(3 * 3600 + 120)).toBe("3h 2m");
    expect(uptimeLabel(2 * 86400 + 5 * 3600)).toBe("2d 5h");
  });
  it("compactNumber", () => {
    expect(compactNumber(1200)).toBe("1.2K");
    expect(compactNumber(7)).toBe("7");
  });
});

describe("timeAgo", () => {
  afterEach(() => vi.useRealTimers());
  it("says how long ago", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-06-01T12:00:00Z"));
    expect(timeAgo(undefined)).toBe("never");
    expect(timeAgo("2026-06-01T11:59:30Z")).toBe("30s ago");
    expect(timeAgo("2026-06-01T11:50:00Z")).toBe("10 min ago");
    expect(timeAgo("2026-06-01T09:00:00Z")).toBe("3 h ago");
    expect(timeAgo("2026-05-29T12:00:00Z")).toBe("3 d ago");
    expect(timeAgo("2026-06-01T12:05:00Z")).toBe("0s ago"); // a clock ahead isn't negative
  });
});
