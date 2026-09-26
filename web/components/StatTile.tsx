export function StatTile({
  label,
  value,
  href,
}: {
  label: string;
  value: string;
  href?: string;
}) {
  const content = (
    <>
      <span className="text-sm text-text-secondary">{label}</span>
      <span className="text-3xl font-semibold text-text-primary">{value}</span>
    </>
  );
  const className =
    "flex flex-col gap-1 rounded-lg bg-surface-1 px-5 py-4 transition-colors hover:bg-surface-hover";

  if (!href) {
    return <div className={className}>{content}</div>;
  }
  return (
    <a href={href} className={className}>
      {content}
    </a>
  );
}
