// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { WriteOnly } from "@/components/Access";
import Link from "next/link";
import { BulkServiceActions, SelectBox } from "@/components/BulkServiceActions";
import { LinkButton } from "@/components/Button";
import { PortChips } from "@/components/ServicePorts";
import { StatusBadge } from "@/components/StatusBadge";
import { StackChip } from "@/components/ListToolbar";
import { getApps, getServices } from "@/lib/api";
import { sortItems, stackEdge } from "@/lib/listing";
import { serviceHealthRole, serviceStatusRole } from "@/lib/format";

// SigNoz apps (custom compose apps from the SigNoz template), each server
// with its status, health and a link to its UI; services deployed earlier
// with Foundry are listed after them and still work as before.
export default async function DeploySigNozListPage() {
  const [apps, services] = await Promise.all([getApps(), getServices()]);
  const signoz = apps.filter((a) => a.template === "signoz");
  const foundry = services.filter((s) => s.type === "signoz");

  return (
    <div className="flex flex-col gap-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">SigNoz</h1>
          <p className="mt-1 text-sm text-text-secondary">
            SigNoz observability (traces, metrics, logs), deployed with docker compose to one or many servers. Each is a
            custom compose app: its compose, config files and .env are all editable.
          </p>
        </div>
        <WriteOnly>
          <LinkButton href="/deploy/signoz/new">+ New SigNoz</LinkButton>
        </WriteOnly>
      </div>

      <BulkServiceActions
        items={[
          ...signoz.flatMap((a) => a.servers.map((s) => ({ id: s.id, name: `${a.name} on ${s.host}`, host: s.host }))),
          ...foundry.map((s) => ({ id: s.id, name: s.name, host: s.host })),
        ]}
      >
      {signoz.length === 0 && foundry.length === 0 && (
        <div className="card flex flex-col items-center gap-2 px-6 py-12 text-center">
          <p className="section-title">No SigNoz yet</p>
          <p className="max-w-sm text-sm text-text-muted">Create one and pick the servers to run it on.</p>
        </div>
      )}

      {signoz.length > 0 && (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {signoz.map((app) => (
            <div key={app.id} className="card flex flex-col gap-3 p-5" style={stackEdge(true)}>
              <div className="flex flex-wrap items-center gap-2">
                <Link href={`/deploy/compose/${app.id}`} className="text-base font-bold text-text-primary hover:underline">
                  {app.name}
                </Link>
                <StackChip />
                <span className="font-mono text-xs text-text-secondary">{app.deployDir}</span>
                <Link href={`/deploy/compose/${app.id}`} className="ml-auto text-xs font-semibold text-accent hover:underline">
                  Open app →
                </Link>
              </div>
              {app.servers.length === 0 ? (
                <p className="text-xs text-text-muted">No servers yet — open the app to add one.</p>
              ) : (
                <ul className="flex flex-col gap-1.5">
                  {sortItems(app.servers, "attention").map((s) => (
                    <li key={s.id} className="flex flex-wrap items-center gap-2 rounded-2xl bg-surface-2 px-3 py-2">
                      <SelectBox id={s.id} name={`${app.name} on ${s.host}`} />
                      <Link href={`/deploy/compose/${app.id}/servers/${s.id}`} className="font-mono text-sm font-semibold text-text-primary hover:underline">
                        {s.host}
                      </Link>
                      <StatusBadge role={serviceStatusRole(s.status)} label={s.status} />
                      <StatusBadge role={serviceHealthRole(s.health)} label={s.health} />
                      <PortChips ports={s.healthPorts} />
                      {s.status === "deployed" && (
                        <a
                          href={`http://${s.host}:8080`}
                          target="_blank"
                          rel="noreferrer"
                          className="ml-auto text-xs font-semibold text-accent hover:underline"
                        >
                          Open SigNoz UI ↗
                        </a>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          ))}
        </div>
      )}

      {foundry.length > 0 && (
        <section className="flex flex-col gap-3">
          <div>
            <h2 className="section-title">Deployed with Foundry</h2>
            <p className="mt-1 text-sm text-text-secondary">
              Created with SigNoz&apos;s Foundry installer before SigNoz moved to plain compose here. They keep working as
              they are; new ones are created with compose.
            </p>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {foundry.map((s) => (
              <div key={s.id} className="relative">
                <span className="absolute left-4 top-[23px] z-10 flex">
                  <SelectBox id={s.id} name={s.name} />
                </span>
                <Link href={`/deploy/signoz/${s.id}`} className="card flex h-full flex-col gap-3 p-5 pl-11 transition-transform hover:-translate-y-0.5">
                  <span className="truncate text-base font-bold text-text-primary">{s.name}</span>
                  <span className="truncate font-mono text-xs text-text-secondary">
                    {s.host} · {s.deployDir}
                  </span>
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge role={serviceStatusRole(s.status)} label={s.status} />
                    <StatusBadge role={serviceHealthRole(s.health)} label={s.health} />
                  </div>
                  <PortChips ports={s.healthPorts} />
                </Link>
              </div>
            ))}
          </div>
        </section>
      )}
      </BulkServiceActions>
    </div>
  );
}
