// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import { BulkServiceActions } from "@/components/BulkServiceActions";
import { ServiceCards } from "@/components/ServiceCards";
import { LinkButton } from "@/components/Button";
import type { Service } from "@/lib/api";

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
        <WriteOnly>
          <div className="flex flex-wrap gap-2">
            {extraAction && (
              <LinkButton href={extraAction.href} variant="secondary">
                {extraAction.label}
              </LinkButton>
            )}
            <LinkButton href={newHref}>{newLabel}</LinkButton>
          </div>
        </WriteOnly>
      </div>

      {services.length === 0 ? (
        <div className="card flex flex-col items-center gap-2 px-6 py-12 text-center">
          <p className="section-title">No services yet</p>
          <p className="max-w-sm text-sm text-text-muted">
            Create one to install it on a server of your choice.
          </p>
        </div>
      ) : (
        <BulkServiceActions items={services.map((s) => ({ id: s.id, name: s.name, host: s.host }))}>
          <ServiceCards services={services} basePath={basePath} />
        </BulkServiceActions>
      )}
    </div>
  );
}
