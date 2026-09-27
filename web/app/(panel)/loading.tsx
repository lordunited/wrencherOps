export default function Loading() {
  return (
    <div className="flex flex-col gap-4" aria-busy="true" aria-label="Loading">
      <div className="h-8 w-48 animate-pulse rounded-full bg-surface-1 shadow-card" />
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <div key={i} className="h-24 animate-pulse rounded-3xl bg-surface-1 shadow-card" />
        ))}
      </div>
      <div className="h-64 animate-pulse rounded-3xl bg-surface-1 shadow-card" />
    </div>
  );
}
