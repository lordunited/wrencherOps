// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import Link from "next/link";
import { SelectBox } from "@/components/BulkServiceActions";
import { GroupHeading, ListToolbar, StackChip, useListControls } from "@/components/ListToolbar";
import { PortChips } from "@/components/ServicePorts";
import { ContainerSummary } from "@/components/ServiceContainers";
import { StatusBadge } from "@/components/StatusBadge";
import type { Service } from "@/lib/api";
import { serviceHealthRole, serviceStatusRole } from "@/lib/format";
import { filterItems, groupItems, healthCounts, sortItems, stackEdge, type GroupKey } from "@/lib/listing";
import { LocalTime } from "@/components/LocalTime";

const GROUPS: GroupKey[] = ["server", "health", "none"];

// ServiceCards is a deploy page's services: searchable, filtered by
// health, grouped and sorted. A service that's a Compose stack carries
// the yellow stack mark.
export function ServiceCards({ services, basePath }: { services: Service[]; basePath: string }) {
  const controls = useListControls({ group: "server" }, GROUPS);
  const items = services.map((s) => ({ ...s, text: `${s.deployDir} ${s.composeProject ?? ""} ${s.container ?? ""} ${s.status}` }));
  const shown = sortItems(filterItems(items, controls.query, controls.show), controls.sort);
  const groups = groupItems(shown, controls.group);

  return (
    <div className="flex flex-col gap-4">
      <ListToolbar controls={controls} groups={GROUPS} counts={healthCounts(items)} shown={shown.length} />
      {shown.length === 0 && <p className="card px-4 py-6 text-center text-sm text-text-muted">Nothing matches.</p>}
      {groups.map((g) => (
        <section key={g.key} className="flex flex-col gap-2.5">
          <GroupHeading label={g.label} count={g.items.length} mono={controls.group === "server"} />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {g.items.map((svc) => {
              const stack = Boolean(svc.composeProject);
              return (
                <div key={svc.id} className="relative">
                  <span className="absolute left-4 top-[22px] z-10 flex">
                    <SelectBox id={svc.id} name={svc.name} />
                  </span>
                  <Link
                    href={`${basePath}/${svc.id}`}
                    className="card flex h-full flex-col gap-3 p-5 pl-11 transition-transform hover:-translate-y-0.5"
                    style={stack ? stackEdge(true) : undefined}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <span className="min-w-0 truncate text-base font-bold text-text-primary">{svc.name}</span>
                      <StatusBadge role={serviceStatusRole(svc.status)} label={svc.status} />
                    </div>
                    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                      {stack && <StackChip project={svc.composeProject} />}
                      {svc.origin === "connected" && <span className="text-[11px] font-semibold text-text-muted">connected</span>}
                      <span className="truncate font-mono text-xs text-text-secondary">
                        {svc.username}@{svc.host}:{svc.port}
                      </span>
                    </div>
                    <ContainerSummary containers={svc.healthContainers} />
                    <PortChips ports={svc.healthPorts} />
                    <div className="mt-auto flex items-center justify-between gap-2">
                      <StatusBadge role={serviceHealthRole(svc.health)} label={svc.health} />
                      <span className="text-xs text-text-muted">
                        Updated <LocalTime value={svc.updatedAt} style="date" />
                      </span>
                    </div>
                  </Link>
                </div>
              );
            })}
          </div>
        </section>
      ))}
    </div>
  );
}
