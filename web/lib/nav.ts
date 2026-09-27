export interface NavItem {
  href: string;
  label: string;
}

export type NavIcon = "gitlab" | "deploy";

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
      { href: "/health", label: "Health" },
      { href: "/projects", label: "Projects" },
      { href: "/runners", label: "Runners" },
      { href: "/pipelines", label: "Pipelines" },
      { href: "/merge-requests", label: "Merge requests" },
      { href: "/users", label: "Users" },
    ],
  },
  {
    name: "Deploy",
    icon: "deploy",
    items: [
      { href: "/deploy/gitlab", label: "GitLab" },
      { href: "/deploy/nexus3", label: "Nexus3" },
      { href: "/deploy/prometheus", label: "Prometheus stack" },
      { href: "/deploy/nginx", label: "Nginx" },
    ],
  },
];
