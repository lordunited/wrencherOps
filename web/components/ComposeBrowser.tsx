// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import Link from "next/link";
import { WriteOnly } from "@/components/Access";
import { SelectBox } from "@/components/BulkServiceActions";
import { GroupHeading, ListToolbar, StackChip, useListControls } from "@/components/ListToolbar";
import { PortChips } from "@/components/ServicePorts";
import { ContainerSummary } from "@/components/ServiceContainers";
import { StatusBadge } from "@/components/StatusBadge";
import type { Service } from "@/lib/api";
import { serviceHealthRole, serviceStatusRole } from "@/lib/format";
import { filterItems, groupItems, healthCounts, sortItems, stackEdge, type GroupKey } from "@/lib/listing";

// StackProject is one compose project on one server: a server of an app,
// or a project watched as it is.
export interface StackProject {
  service: Service;
  name: string;
  kind: "app" | "watched";
  composeFiles: string[];
  href: string;
}

export interface UnplacedApp {
  id: string;
  name: string;
  deployDir: string;
  updatedAt: string;
}

const GROUPS: GroupKey[] = ["server", "app", "health", "none"];

// ComposeBrowser is every Compose stack the panel knows — searchable,
// filtered by health, grouped by server, app or health, and sorted. Each
// carries the yellow stack mark.
export function ComposeBrowser({ projects, unplaced, initialGroup }: { projects: StackProject[]; unplaced: UnplacedApp[]; initialGroup: GroupKey }) {
  const controls = useListControls({ group: initialGroup }, GROUPS);
  const items = projects.map((p) => ({
    ...p,
    id: p.service.id,
    host: p.service.host,
    health: p.service.health,
    updatedAt: p.service.updatedAt,
    app: p.kind === "app" ? p.name : undefined,
    text: `${p.composeFiles.join(" ")} ${p.service.deployDir} ${p.service.composeProject ?? ""} ${p.kind === "app" ? "app" : "watched watch"} ${p.service.status}`,
  }));
  const shown = sortItems(filterItems(items, controls.query, controls.show), controls.sort);
  const groups = groupItems(shown, controls.group);
  const q = controls.query.toLowerCase();
  const unplacedShown = controls.show === "all" ? unplaced.filter((a) => `${a.name} ${a.deployDir}`.toLowerCase().includes(q)) : [];
  const byServer = controls.group === "server";

  return (
    <div className="flex flex-col gap-4">
      <ListToolbar controls={controls} groups={GROUPS} counts={healthCounts(items)} shown={shown.length} placeholder="Search stack, server, compose file…" />
      {shown.length === 0 && unplacedShown.length === 0 && (
        <p className="card px-4 py-6 text-center text-sm text-text-muted">{projects.length === 0 ? "No stacks yet." : "Nothing matches."}</p>
      )}

      {groups.map((g) => {
        const first = g.items[0].service;
        const healthy = g.items.filter((p) => p.service.health === "healthy").length;
        // A new app on this server reuses the saved login of whichever of
        // its stacks has one.
        const loginFrom = g.items.find((p) => p.service.hasSavedPassword)?.service ?? first;
        const appId = controls.group === "app" ? g.items.find((p) => p.service.appId)?.service.appId : undefined;
        return (
          <section key={g.key} className="card flex flex-col gap-3 p-4 sm:p-5">
            <GroupHeading label={g.label} count={g.items.length} mono={byServer}>
              <span className="text-xs text-text-muted">
                {healthy}/{g.items.length} healthy{byServer ? ` · as ${first.username}` : ""}
              </span>
              {byServer && (
                <WriteOnly>
                  <span className="ml-auto flex items-center gap-3">
                    <Link href={`/deploy/compose/new?server=${loginFrom.id}`} className="text-xs font-semibold text-accent hover:underline">
                      + New compose here
                    </Link>
                    <Link
                      href={`/deploy/compose/detect?host=${encodeURIComponent(g.label)}&port=${first.port}&user=${encodeURIComponent(first.username)}`}
                      className="text-xs font-semibold text-accent hover:underline"
                    >
                      Scan again →
                    </Link>
                  </span>
                </WriteOnly>
              )}
              {appId && (
                <Link href={`/deploy/compose/${appId}`} className="ml-auto text-xs font-semibold text-accent hover:underline">
                  Open app →
                </Link>
              )}
            </GroupHeading>
            <ul className="flex flex-col gap-1.5">
              {g.items.map((p) => (
                <li key={p.service.id} className="relative">
                  <span className="absolute left-5 top-[15px] z-10 flex">
                    <SelectBox id={p.service.id} name={p.name} />
                  </span>
                  <Link
                    href={p.href}
                    className="flex flex-col gap-1 rounded-2xl bg-surface-2 py-3 pl-12 pr-4 transition-colors hover:bg-surface-hover"
                    style={stackEdge(false)}
                  >
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-bold text-text-primary">{p.name}</span>
                      <StackChip project={p.service.composeProject || undefined} />
                      <StatusBadge role="muted" label={p.kind === "app" ? "app" : "watch only"} />
                      {p.kind === "app" && <StatusBadge role={serviceStatusRole(p.service.status)} label={p.service.status} />}
                      <StatusBadge role={serviceHealthRole(p.service.health)} label={p.service.health} />
                      {!byServer && <span className="font-mono text-xs text-text-secondary">{p.service.host.trim()}</span>}
                      <span className="ml-auto">
                        <PortChips ports={p.service.healthPorts} />
                      </span>
                    </div>
                    <ContainerSummary containers={p.service.healthContainers} />
                    {p.composeFiles.map((f) => (
                      <span key={f} className="break-all font-mono text-xs text-text-secondary">
                        {f}
                      </span>
                    ))}
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        );
      })}

      {unplacedShown.length > 0 && (
        <section className="flex flex-col gap-2.5">
          <GroupHeading label="No server yet" count={unplacedShown.length}>
            <span className="text-xs text-text-muted">Open an app to add the servers to run it on.</span>
          </GroupHeading>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {unplacedShown.map((app) => (
              <Link
                key={app.id}
                href={`/deploy/compose/${app.id}`}
                className="card flex flex-col gap-2 p-5 transition-transform hover:-translate-y-0.5"
                style={stackEdge(true)}
              >
                <span className="flex items-center gap-2">
                  <span className="truncate text-base font-bold text-text-primary">{app.name}</span>
                  <StackChip />
                </span>
                <span className="truncate font-mono text-xs text-text-secondary">{app.deployDir}</span>
                <span className="text-xs text-text-muted">Updated {new Date(app.updatedAt).toLocaleDateString()}</span>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
