import type { NavIcon } from "@/lib/nav";

const COMMON = {
  width: 16,
  height: 16,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.75,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
};

// Generic glyphs, not brand marks: a branch icon stands in for "GitLab" as
// a category (source control), not the GitLab logo itself.
const PATHS: Record<NavIcon, React.ReactNode> = {
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
};

export function Icon({ name, className }: { name: NavIcon; className?: string }) {
  return (
    <svg {...COMMON} className={className} aria-hidden>
      {PATHS[name]}
    </svg>
  );
}
