import Link from "next/link";
import { LinkButton } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { Service } from "@/lib/api";
import { serviceHealthRole, serviceStatusRole } from "@/lib/format";

export function ServicesTable({
  title,
  description,
  services,
  basePath,
  newHref,
  newLabel,
  extraAction,
}: {
  title: string;
  description: string;
  services: Service[];
  basePath: string;
  newHref: string;
  newLabel: string;
  extraAction?: { href: string; label: string };
}) {
  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">{title}</h1>
          <p className="mt-1 text-sm text-text-secondary">{description}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {extraAction && (
            <LinkButton href={extraAction.href} variant="secondary">
              {extraAction.label}
            </LinkButton>
          )}
          <LinkButton href={newHref}>{newLabel}</LinkButton>
        </div>
      </div>

      {services.length === 0 ? (
        <div className="card flex flex-col items-center gap-2 px-6 py-12 text-center">
          <p className="section-title">No services yet</p>
          <p className="max-w-sm text-sm text-text-muted">
            Create one to install it on a server of your choice.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {services.map((svc) => (
            <Link
              key={svc.id}
              href={`${basePath}/${svc.id}`}
              className="card flex flex-col gap-3 p-5 transition-transform hover:-translate-y-0.5"
            >
              <div className="flex items-start justify-between gap-2">
                <span className="min-w-0 truncate text-base font-bold text-text-primary">
                  {svc.name}
                  {svc.origin === "connected" && (
                    <span className="ml-2 align-middle text-[11px] font-semibold text-text-muted">connected</span>
                  )}
                </span>
                <StatusBadge role={serviceStatusRole(svc.status)} label={svc.status} />
              </div>
              <span className="truncate font-mono text-xs text-text-secondary">
                {svc.username}@{svc.host}:{svc.port}
              </span>
              <div className="flex items-center justify-between gap-2">
                <StatusBadge role={serviceHealthRole(svc.health)} label={svc.health} />
                <span className="text-xs text-text-muted">
                  Updated {new Date(svc.updatedAt).toLocaleDateString()}
                </span>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
