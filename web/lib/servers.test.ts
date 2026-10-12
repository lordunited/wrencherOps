// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import type { Service } from "@/lib/api";
import { knownServers, serverLabel } from "@/lib/servers";

const svc = (id: string, host: string, username: string, hasSavedPassword: boolean, port = 22) =>
  ({ id, host, username, port, hasSavedPassword }) as Service;

describe("knownServers", () => {
  it("lists each login once, preferring one with a saved password", () => {
    const got = knownServers([
      svc("a", "10.0.0.2", "root", false),
      svc("b", " 10.0.0.2 ", "root", true),
      svc("c", "10.0.0.1", "deploy", false),
      svc("d", "10.0.0.2", "root", false, 2222),
    ]);
    expect(got.map((k) => `${k.serviceId}:${k.host}:${k.port}`)).toEqual(["c:10.0.0.1:22", "b:10.0.0.2:22", "d:10.0.0.2:2222"]);
    expect(got[1].hasSavedPassword).toBe(true);
  });
});

describe("serverLabel", () => {
  it("shows the port only when it isn't 22", () => {
    expect(serverLabel({ host: "h", port: 22, username: "u" })).toBe("u@h");
    expect(serverLabel({ host: "h", port: 2222, username: "u" })).toBe("u@h:2222");
  });
});
