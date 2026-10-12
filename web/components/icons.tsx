// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { NavIcon } from "@/lib/nav";

const COMMON = {
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.75,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

export type IconName = NavIcon | "home" | "pulse" | "folder" | "menu" | "close" | "logout" | "bell" | "stack" | "search";

// Generic glyphs, not brand marks: a branch icon stands in for "GitLab" as
// a category (source control), not the GitLab logo itself.
const PATHS: Record<IconName, React.ReactNode> = {
  gitlab: (
    <>
      <circle cx="6" cy="6" r="2.5" />
      <circle cx="6" cy="18" r="2.5" />
      <circle cx="18" cy="9" r="2.5" />
      <path d="M6 8.5v7" />
      <path d="M6 12c0-3 3-3 6-3s6 0 6-3" />
    </>
  ),
  deploy: (
    <>
      <rect x="4" y="14" width="16" height="6" rx="1.5" />
      <path d="M8 14V9a4 4 0 0 1 8 0v5" />
      <path d="M12 3v3" />
    </>
  ),
  server: (
    <>
      <rect x="4" y="4" width="16" height="7" rx="1.5" />
      <rect x="4" y="13" width="16" height="7" rx="1.5" />
      <path d="M8 7.5h.01" />
      <path d="M8 16.5h.01" />
    </>
  ),
  terraform: (
    <>
      <path d="M4 4.5l6 3.5v7l-6-3.5z" />
      <path d="M11.5 8l6-3.5v7l-6 3.5z" />
      <path d="M11.5 16.5l6-3.5v7l-6-3.5z" />
    </>
  ),
  ansible: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M10 8.5v7l5.5-3.5z" />
    </>
  ),
  admin: (
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M3 19.5c0-3.3 2.7-5.5 6-5.5s6 2.2 6 5.5" />
      <path d="M17.5 8.5l1.5 1.5 3-3" />
    </>
  ),
  stack: (
    <>
      <path d="M12 3l9 5-9 5-9-5z" />
      <path d="M3 13l9 5 9-5" />
      <path d="M3 17.5l9 5 9-5" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="6.5" />
      <path d="M16 16l4.5 4.5" />
    </>
  ),
  bell: (
    <>
      <path d="M6 16V11a6 6 0 0 1 12 0v5l1.5 2h-15z" />
      <path d="M10 20a2 2 0 0 0 4 0" />
    </>
  ),
  home: (
    <>
      <path d="M4 11l8-6 8 6" />
      <path d="M6 10v9h12v-9" />
    </>
  ),
  pulse: <path d="M3 12h4l2-5 4 10 2-5h6" />,
  folder: <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />,
  menu: (
    <>
      <path d="M4 7h16" />
      <path d="M4 12h16" />
      <path d="M4 17h16" />
    </>
  ),
  close: (
    <>
      <path d="M6 6l12 12" />
      <path d="M18 6L6 18" />
    </>
  ),
  logout: (
    <>
      <path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3" />
      <path d="M10 16l-4-4 4-4" />
      <path d="M6 12h10" />
    </>
  ),
};

export function Icon({
  name,
  className,
  size = 16,
}: {
  name: IconName;
  className?: string;
  size?: number;
}) {
  return (
    <svg {...COMMON} width={size} height={size} className={className} aria-hidden>
      {PATHS[name]}
    </svg>
  );
}
