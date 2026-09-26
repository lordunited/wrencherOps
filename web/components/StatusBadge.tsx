import type { StatusRole } from "@/lib/format";

const ROLE_COLOR: Record<StatusRole, string> = {
  good: "var(--status-good)",
  warning: "var(--status-warning)",
  serious: "var(--status-serious)",
  critical: "var(--status-critical)",
  muted: "var(--text-muted)",
};

export function StatusBadge({ role, label }: { role: StatusRole; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-sm text-text-secondary">
      <span
        className="inline-block h-2 w-2 rounded-full"
        style={{ backgroundColor: ROLE_COLOR[role] }}
        aria-hidden
      />
      {label}
    </span>
  );
}
