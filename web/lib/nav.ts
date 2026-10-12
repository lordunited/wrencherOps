// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// tab names the tab access is granted on; the backend's access.Tabs has
// the same keys (a Go test keeps them in step).
export interface NavItem {
  href: string;
  label: string;
  tab: string;
}

export type NavIcon = "gitlab" | "deploy" | "server" | "ansible" | "terraform" | "admin";

export interface NavCategory {
  name: string;
  icon: NavIcon;
  items: NavItem[];
}

export const NAV_CATEGORIES: NavCategory[] = [
  {
    name: "GitLab",
    icon: "gitlab",
    items: [
      { href: "/health", label: "Health", tab: "gitlab.health" },
      { href: "/projects", label: "Projects", tab: "gitlab.projects" },
      { href: "/runners", label: "Runners", tab: "gitlab.runners" },
      { href: "/pipelines", label: "Pipelines", tab: "gitlab.pipelines" },
      { href: "/merge-requests", label: "Merge requests", tab: "gitlab.merge-requests" },
      { href: "/users", label: "Users", tab: "gitlab.users" },
    ],
  },
  {
    name: "Servers",
    icon: "server",
    items: [
      { href: "/servers", label: "Servers", tab: "servers" },
      { href: "/servers/notifications", label: "Notifications", tab: "servers.notifications" },
    ],
  },
  {
    name: "Ansible",
    icon: "ansible",
    items: [
      { href: "/ansible", label: "Playbooks", tab: "ansible.playbooks" },
      { href: "/ansible/runs", label: "Run history", tab: "ansible.runs" },
      { href: "/ansible/modules", label: "Modules", tab: "ansible.modules" },
    ],
  },
  {
    name: "Terraform",
    icon: "terraform",
    items: [
      { href: "/terraform", label: "Workspaces", tab: "terraform.workspaces" },
      { href: "/terraform/runs", label: "Run history", tab: "terraform.runs" },
    ],
  },
  {
    name: "Deploy",
    icon: "deploy",
    items: [
      { href: "/deploy/gitlab", label: "GitLab", tab: "deploy.gitlab" },
      { href: "/deploy/nexus3", label: "Nexus3", tab: "deploy.nexus3" },
      { href: "/deploy/prometheus", label: "Prometheus stack", tab: "deploy.prometheus" },
      { href: "/deploy/nginx", label: "Nginx", tab: "deploy.nginx" },
      { href: "/deploy/haproxy", label: "HAProxy", tab: "deploy.haproxy" },
      { href: "/deploy/envoy", label: "Envoy", tab: "deploy.envoy" },
      { href: "/deploy/signoz", label: "SigNoz", tab: "deploy.signoz" },
      { href: "/deploy/compose", label: "Custom compose", tab: "deploy.compose" },
    ],
  },
  {
    name: "Admin",
    icon: "admin",
    items: [{ href: "/admin", label: "Users & access", tab: "admin.users" }],
  },
];

// VIEW_ONLY_TABS have nothing to change, for anyone (access.Tabs' ViewOnly;
// a Go test keeps them in step).
export const VIEW_ONLY_TABS = new Set([
  "overview",
  "gitlab.health",
  "gitlab.projects",
  "gitlab.runners",
  "gitlab.pipelines",
  "gitlab.merge-requests",
  "gitlab.users",
  "ansible.runs",
  "ansible.modules",
  "terraform.runs",
]);

// Levels a user has in a tab; a tab missing from a user's map is "none".
export type AccessLevel = "view" | "write";
export type TabAccess = Record<string, AccessLevel>;

// visibleCategories is the nav for a user: only the tabs they can view,
// and only categories with a tab left.
export function visibleCategories(tabs: TabAccess): NavCategory[] {
  return NAV_CATEGORIES.map((c) => ({ ...c, items: c.items.filter((i) => tabs[i.tab]) })).filter((c) => c.items.length > 0);
}

// tabForPath is the tab a page belongs to: the nav item it's at or under.
// Pages outside the nav (the raw Docker and Kubernetes ones) are
// deploy.other; the overview is everyone's.
export function tabForPath(pathname: string): string | null {
  if (pathname === "/") return "overview";
  if (/^\/deploy\/(docker|kubernetes)(\/|$)/.test(pathname)) return "deploy.other";
  const href = activeHref(pathname);
  for (const c of NAV_CATEGORIES) for (const i of c.items) if (i.href === href) return i.tab;
  return null;
}

// activeHref is the nav link for pathname: the longest one it's at or under,
// so /servers/notifications marks "Notifications", not also "Servers".
export function activeHref(pathname: string): string | null {
  let best: string | null = null;
  for (const c of NAV_CATEGORIES) {
    for (const item of c.items) {
      if ((pathname === item.href || pathname.startsWith(item.href + "/")) && (!best || item.href.length > best.length)) {
        best = item.href;
      }
    }
  }
  return best;
}

// CREATE_PAGE is a page only for adding something: it needs write.
const CREATE_PAGE = /\/(new|connect|detect)$/;

// pageLevel is a user's level on a page: its tab's level ("view", "write",
// or undefined for no access). A page outside every tab (their own
// account) is theirs; a page for adding something needs write.
export function pageLevel(tabs: TabAccess | null, pathname: string): AccessLevel | undefined {
  if (!tabs) return "write";
  const tab = tabForPath(pathname);
  if (tab === null) return "write";
  const level = tabs[tab];
  if (level === "view" && CREATE_PAGE.test(pathname)) return undefined;
  return level;
}

// CATEGORY_COLOR is each sidebar category's color (globals.css --cat-*).
export const CATEGORY_COLOR: Record<NavIcon, string> = {
  gitlab: "gitlab",
  server: "servers",
  ansible: "ansible",
  terraform: "terraform",
  deploy: "deploy",
  admin: "admin",
};

// categoryVars sets --cat, --cat-soft and --cat-ink to a category's
// colors, for the elements under it to use.
export function categoryVars(icon: NavIcon): Record<string, string> {
  const c = CATEGORY_COLOR[icon];
  return { "--cat": `var(--cat-${c})`, "--cat-soft": `var(--cat-${c}-soft)`, "--cat-ink": `var(--cat-${c}-ink)` };
}
