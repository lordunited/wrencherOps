import type { NavIcon } from "@/lib/nav";

const COMMON = {
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.75,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

export type IconName = NavIcon | "home" | "pulse" | "folder" | "menu" | "close" | "logout";

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
