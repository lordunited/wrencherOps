// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { NAV_CATEGORIES, VIEW_ONLY_TABS, activeHref, categoryVars, pageLevel, tabForPath, visibleCategories } from "@/lib/nav";

describe("activeHref", () => {
  it("picks the longest nav link a path is at or under", () => {
    expect(activeHref("/servers")).toBe("/servers");
    expect(activeHref("/servers/abc")).toBe("/servers");
    expect(activeHref("/servers/notifications")).toBe("/servers/notifications");
    expect(activeHref("/ansible/runs/42")).toBe("/ansible/runs");
    expect(activeHref("/deploy/compose/detect")).toBe("/deploy/compose");
    expect(activeHref("/admin/users/sara")).toBe("/admin");
  });
  it("is null outside the nav", () => {
    expect(activeHref("/")).toBeNull();
    expect(activeHref("/serversx")).toBeNull();
  });
});

describe("tabForPath", () => {
  it("maps pages to their tab", () => {
    expect(tabForPath("/")).toBe("overview");
    expect(tabForPath("/projects/12")).toBe("gitlab.projects");
    expect(tabForPath("/terraform/runs/x")).toBe("terraform.runs");
    expect(tabForPath("/deploy/docker/new")).toBe("deploy.other");
    expect(tabForPath("/deploy/kubernetes")).toBe("deploy.other");
    expect(tabForPath("/account")).toBeNull();
  });
});

describe("nav items", () => {
  it("each have a unique tab and href", () => {
    const items = NAV_CATEGORIES.flatMap((c) => c.items);
    expect(new Set(items.map((i) => i.tab)).size).toBe(items.length);
    expect(new Set(items.map((i) => i.href)).size).toBe(items.length);
  });
  it("mark GitLab's pages and histories view-only", () => {
    expect(VIEW_ONLY_TABS.has("gitlab.health")).toBe(true);
    expect(VIEW_ONLY_TABS.has("terraform.runs")).toBe(true);
    expect(VIEW_ONLY_TABS.has("servers")).toBe(false);
  });
});

describe("visibleCategories", () => {
  it("keeps only the tabs a user can see, and drops empty categories", () => {
    const cats = visibleCategories({ servers: "view", "deploy.nginx": "write" });
    expect(cats.map((c) => c.name)).toEqual(["Servers", "Deploy"]);
    expect(cats[0].items.map((i) => i.href)).toEqual(["/servers"]);
    expect(cats[1].items.map((i) => i.href)).toEqual(["/deploy/nginx"]);
  });
  it("shows nothing for no access", () => {
    expect(visibleCategories({})).toEqual([]);
  });
  it("shows everything to an admin", () => {
    const all = Object.fromEntries(NAV_CATEGORIES.flatMap((c) => c.items.map((i) => [i.tab, "write" as const])));
    expect(visibleCategories(all)).toHaveLength(NAV_CATEGORIES.length);
  });
});

describe("pageLevel", () => {
  const tabs = { overview: "view", servers: "view", "deploy.nginx": "write" } as const;
  it("is the page's tab's level", () => {
    expect(pageLevel(tabs, "/servers/abc")).toBe("view");
    expect(pageLevel(tabs, "/deploy/nginx/xyz")).toBe("write");
    expect(pageLevel(tabs, "/terraform")).toBeUndefined();
    expect(pageLevel(tabs, "/admin")).toBeUndefined();
  });
  it("lets anyone use a page outside the tabs", () => {
    expect(pageLevel(tabs, "/account")).toBe("write");
    expect(pageLevel(null, "/terraform")).toBe("write");
  });
  it("needs write for a page that only adds something", () => {
    expect(pageLevel({ "deploy.prometheus": "view" }, "/deploy/prometheus/new")).toBeUndefined();
    expect(pageLevel({ "deploy.prometheus": "view" }, "/deploy/prometheus/connect")).toBeUndefined();
    expect(pageLevel({ "deploy.compose": "view" }, "/deploy/compose/detect")).toBeUndefined();
    expect(pageLevel({ "deploy.prometheus": "write" }, "/deploy/prometheus/new")).toBe("write");
  });
});

describe("category colors", () => {
  it("every category has its own color", () => {
    const colors = NAV_CATEGORIES.map((c) => categoryVars(c.icon)["--cat"]);
    expect(new Set(colors).size).toBe(NAV_CATEGORIES.length);
    expect(categoryVars("gitlab")).toEqual({ "--cat": "var(--cat-gitlab)", "--cat-soft": "var(--cat-gitlab-soft)", "--cat-ink": "var(--cat-gitlab-ink)" });
  });
});
