// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// A figure in a card. "hero" fills it with the accent color, for the one
// number on a page that matters most.
export function StatTile({
  label,
  value,
  href,
  hint,
  hero = false,
}: {
  label: string;
  value: React.ReactNode;
  href?: string;
  hint?: React.ReactNode;
  hero?: boolean;
}) {
  const content = (
    <>
      <span
        className={`text-[11px] font-bold uppercase tracking-[0.07em] ${hero ? "text-on-accent/75" : "text-text-muted"}`}
      >
        {label}
      </span>
      <span className={`text-2xl font-extrabold tabular-nums tracking-tight ${hero ? "text-on-accent" : "text-text-primary"}`}>
        {value}
      </span>
      {hint && <span className={`text-xs ${hero ? "text-on-accent/75" : "text-text-muted"}`}>{hint}</span>}
    </>
  );
  const className = `flex min-w-0 flex-col gap-1 rounded-3xl px-5 py-4 shadow-card transition-transform ${
    hero ? "bg-accent" : "bg-surface-1"
  } ${href ? "hover:-translate-y-0.5" : ""}`;

  if (!href) {
    return <div className={className}>{content}</div>;
  }
  return (
    <a href={href} className={className}>
      {content}
    </a>
  );
}
