// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { filterItems, groupItems, healthCounts, isProblem, oneOf, sortItems, type ListItem } from "@/lib/listing";

const item = (id: string, name: string, host: string, health: string, updatedAt = "2026-01-01T00:00:00Z", app?: string): ListItem => ({
  id,
  name,
  host,
  health,
  updatedAt,
  app,
});

const items = [
  item("1", "web", "10.0.0.2", "healthy", "2026-03-01T00:00:00Z", "shop"),
  item("2", "db", "10.0.0.1", "unreachable", "2026-01-01T00:00:00Z", "shop"),
  item("3", "cache", "10.0.0.10", "degraded", "2026-02-01T00:00:00Z"),
  item("4", "api", "10.0.0.1", "starting", "2026-04-01T00:00:00Z", "billing"),
  item("5", "Web-2", "10.0.0.2", "healthy", "2026-01-15T00:00:00Z"),
];
const ids = (list: ListItem[]) => list.map((i) => i.id).join(",");

describe("sortItems", () => {
  it("puts what needs attention first", () => {
    expect(ids(sortItems(items, "attention"))).toBe("2,3,4,1,5");
  });
  it("sorts by name, case-insensitively", () => {
    expect(ids(sortItems(items, "name"))).toBe("4,3,2,1,5");
  });
  it("sorts by server, numerically", () => {
    expect(ids(sortItems(items, "server"))).toBe("4,2,1,5,3"); // 10.0.0.1, .2, then .10
  });
  it("sorts by most recently updated", () => {
    expect(ids(sortItems(items, "updated"))).toBe("4,1,3,5,2");
  });
  it("doesn't change the list it's given", () => {
    const copy = [...items];
    sortItems(items, "name");
    expect(items).toEqual(copy);
  });
});

describe("filterItems", () => {
  it("filters by health", () => {
    expect(ids(filterItems(items, "", "problems"))).toBe("2,3");
    expect(ids(filterItems(items, "", "healthy"))).toBe("1,5");
    expect(filterItems(items, "", "all")).toHaveLength(5);
  });
  it("matches every word of a search, in any field", () => {
    expect(ids(filterItems(items, "WEB", "all"))).toBe("1,5");
    expect(ids(filterItems(items, "shop 10.0.0.1", "all"))).toBe("2");
    expect(ids(filterItems(items, "billing", "all"))).toBe("4");
    expect(filterItems(items, "nothing-matches", "all")).toHaveLength(0);
  });
});

describe("groupItems", () => {
  it("groups by server", () => {
    const g = groupItems(sortItems(items, "name"), "server");
    expect(g.map((x) => x.label)).toEqual(["10.0.0.1", "10.0.0.2", "10.0.0.10"]);
    expect(ids(g[0].items)).toBe("4,2");
  });
  it("groups by app, with stacks outside any app last", () => {
    const g = groupItems(items, "app");
    expect(g.map((x) => x.label)).toEqual(["billing", "shop", "Not part of an app"]);
  });
  it("groups by health, worst first", () => {
    const g = groupItems(items, "health");
    expect(g.map((x) => x.label)).toEqual(["Needs attention", "Starting or not checked", "Healthy"]);
    expect(ids(g[0].items)).toBe("2,3");
  });
  it("leaves out empty groups", () => {
    expect(groupItems([items[0]], "health").map((x) => x.key)).toEqual(["healthy"]);
    expect(groupItems([], "none")).toEqual([]);
    expect(groupItems(items, "none")[0].items).toHaveLength(5);
  });
});

describe("helpers", () => {
  it("healthCounts", () => {
    expect(healthCounts(items)).toEqual({ all: 5, problems: 2, healthy: 2 });
  });
  it("isProblem", () => {
    expect(isProblem("paused")).toBe(true);
    expect(isProblem("starting")).toBe(false);
    expect(isProblem("something new")).toBe(false);
  });
  it("oneOf", () => {
    expect(oneOf("app", ["server", "app"] as const, "server")).toBe("app");
    expect(oneOf("evil", ["server", "app"] as const, "server")).toBe("server");
    expect(oneOf(null, ["server"] as const, "server")).toBe("server");
  });
});
