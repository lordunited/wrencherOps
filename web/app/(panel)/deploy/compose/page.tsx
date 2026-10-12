// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import Link from "next/link";
import { BulkServiceActions } from "@/components/BulkServiceActions";
import { LinkButton } from "@/components/Button";
import { ComposeBrowser, type StackProject } from "@/components/ComposeBrowser";
import { ComposeTabs } from "@/components/ComposeTabs";
import { getApps, getServices } from "@/lib/api";

export default async function ComposeAppsPage(props: PageProps<"/deploy/compose">) {
  const { view } = await props.searchParams;
  const [apps, services] = await Promise.all([getApps(), getServices()]);
  const watched = services.filter((s) => s.type === "docker" && s.origin === "connected" && !s.appId);
  const appName = new Map(apps.map((a) => [a.id, a.name]));
  const appComposeFile = new Map(apps.map((a) => [a.id, a.composeFile || "docker-compose.yml"]));

  // Every compose stack the panel knows: each server of each app, and each
  // project watched as it is.
  const projects: StackProject[] = [
    ...services
      .filter((s) => s.appId)
      .map((s) => ({
        service: s,
        name: appName.get(s.appId!) ?? s.name,
        kind: "app" as const,
        composeFiles: [`${s.deployDir}/${appComposeFile.get(s.appId!) ?? "docker-compose.yml"}`],
        href: `/deploy/compose/${s.appId}/servers/${s.id}`,
      })),
    ...watched.map((s) => ({
      service: s,
      name: s.name,
      kind: "watched" as const,
      composeFiles: s.composeFiles ? s.composeFiles.split(",").filter(Boolean) : [`${s.deployDir}/docker-compose.yml`],
      href: `/deploy/compose/watch/${s.id}`,
    })),
  ];
  const unplaced = apps.filter((a) => a.servers.length === 0).map((a) => ({ id: a.id, name: a.name, deployDir: a.deployDir, updatedAt: a.updatedAt }));

  return (
    <div className="flex flex-col gap-5">
      <ComposeTabs active="apps" />
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">Custom compose</h1>
          <p className="mt-1 text-sm text-text-secondary">
            Your own docker-compose.yml, deployed to one or many servers and kept the same on all of them — and
            projects already on a server, imported or watched. Every stack is marked in yellow.
          </p>
        </div>
        <WriteOnly>
          <LinkButton href="/deploy/compose/new">+ New custom compose</LinkButton>
        </WriteOnly>
      </div>

      {projects.length === 0 && unplaced.length === 0 ? (
        <div className="card flex flex-col items-center gap-2 px-6 py-12 text-center">
          <p className="section-title">No stacks yet</p>
          <p className="max-w-sm text-sm text-text-muted">
            Create an app and pick the servers to run it on, or{" "}
            <Link href="/deploy/compose/detect" className="font-semibold text-accent hover:underline">
              detect what&apos;s already on a server
            </Link>
            .
          </p>
        </div>
      ) : (
        <BulkServiceActions items={projects.map((p) => ({ id: p.service.id, name: p.name, host: p.service.host }))}>
          <ComposeBrowser projects={projects} unplaced={unplaced} initialGroup={view === "apps" ? "app" : "server"} />
        </BulkServiceActions>
      )}
    </div>
  );
}
