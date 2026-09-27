import Link from "next/link";

const TABS = [
  { key: "docker", label: "Docker" },
  { key: "kubernetes", label: "Kubernetes" },
] as const;

// A segmented control switching between a tool's Docker and Kubernetes pages.
export function DeployTabs({
  basePath,
  active,
}: {
  basePath: string;
  active: "docker" | "kubernetes";
}) {
  return (
    <div className="inline-flex w-fit gap-1 rounded-full bg-surface-1 p-1 shadow-card">
      {TABS.map((tab) => (
        <Link
          key={tab.key}
          href={tab.key === "docker" ? basePath : `${basePath}/kubernetes`}
          aria-current={active === tab.key ? "page" : undefined}
          className={`rounded-full px-4 py-1.5 text-sm font-semibold transition-colors ${
            active === tab.key
              ? "bg-accent text-on-accent"
              : "text-text-secondary hover:text-text-primary"
          }`}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  );
}
