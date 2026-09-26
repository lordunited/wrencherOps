import Link from "next/link";

const TABS = [
  { key: "docker", label: "Docker" },
  { key: "kubernetes", label: "Kubernetes" },
] as const;

export function DeployTabs({
  basePath,
  active,
}: {
  basePath: string;
  active: "docker" | "kubernetes";
}) {
  return (
    <div className="flex gap-6 border-b-2 border-border-strong">
      {TABS.map((tab) => (
        <Link
          key={tab.key}
          href={tab.key === "docker" ? basePath : `${basePath}/kubernetes`}
          className={`-mb-0.5 border-b-2 px-1 pb-2 text-sm font-medium transition-colors ${
            active === tab.key
              ? "border-accent text-text-primary"
              : "border-transparent text-text-secondary hover:text-text-primary"
          }`}
        >
          {tab.label}
        </Link>
      ))}
    </div>
  );
}
