import Link from "next/link";
import { LinkButton } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { Service } from "@/lib/api";
import { serviceStatusRole } from "@/lib/format";

export function ServicesTable({
  title,
  description,
  services,
  basePath,
  newHref,
  newLabel,
}: {
  title: string;
  description: string;
  services: Service[];
  basePath: string;
  newHref: string;
  newLabel: string;
}) {
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-text-primary">{title}</h1>
          <p className="mt-1 text-sm text-text-secondary">{description}</p>
        </div>
        <LinkButton href={newHref}>{newLabel}</LinkButton>
      </div>

      {services.length === 0 ? (
        <p className="rounded-lg border border-border bg-surface-1 px-4 py-10 text-center text-sm text-text-muted">
          None yet.
        </p>
      ) : (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {services.map((svc) => (
            <Link
              key={svc.id}
              href={`${basePath}/${svc.id}`}
              className="flex flex-col gap-2 rounded-lg bg-surface-1 p-4 transition-colors hover:bg-surface-hover"
            >
              <div className="flex items-start justify-between gap-2">
                <span className="truncate font-semibold text-text-primary">{svc.name}</span>
                <StatusBadge role={serviceStatusRole(svc.status)} label={svc.status} />
              </div>
              <span className="truncate text-sm text-text-secondary">
                {svc.host}:{svc.port}
              </span>
              <span className="text-xs text-text-muted">
                Updated {new Date(svc.updatedAt).toLocaleDateString()}
              </span>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
