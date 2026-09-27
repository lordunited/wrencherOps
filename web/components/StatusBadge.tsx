import type { StatusRole } from "@/lib/format";

const ROLE_COLOR: Record<StatusRole, string> = {
  good: "var(--status-good)",
  warning: "var(--status-warning)",
  serious: "var(--status-serious)",
  critical: "var(--status-critical)",
  muted: "var(--text-muted)",
};

// A status pill: colored dot plus label, on a soft tinted ground.
export function StatusBadge({ role, label }: { role: StatusRole; label: string }) {
  const color = ROLE_COLOR[role];
  return (
    <span
      className="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-semibold text-text-primary"
      style={{ backgroundColor: `color-mix(in srgb, ${color} 14%, transparent)` }}
    >
      <span className="inline-block h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} aria-hidden />
      {label.charAt(0).toUpperCase() + label.slice(1)}
    </span>
  );
}
