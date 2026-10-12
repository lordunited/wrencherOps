// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatTile } from "@/components/StatTile";
import { PortChips } from "@/components/ServicePorts";
import { StatusBadge } from "@/components/StatusBadge";
import { ComposeVolumes } from "@/components/ComposeVolumes";
import { ProjectFilesEditor, type ProjectState } from "@/components/ProjectFilesEditor";
import type { ComposeApp, Service } from "@/lib/api";
import { serverLabel, type KnownServer } from "@/lib/servers";
import { serviceHealthRole, serviceStatusRole } from "@/lib/format";

type BulkAction = "deploy" | "start" | "stop" | "down" | "sync";

const BULK: Record<BulkAction, { label: string; busy: string; done: string; confirm?: string }> = {
  deploy: { label: "Deploy to all", busy: "Deploying…", done: "Deploy started" },
  start: { label: "Start all", busy: "Starting…", done: "Start requested" },
  sync: { label: "Sync all", busy: "Checking…", done: "Checked" },
  stop: { label: "Stop all", busy: "Stopping…", done: "Stop requested", confirm: "stops every server's containers" },
  down: { label: "Down all", busy: "Taking down…", done: "Down requested", confirm: "⚠ deletes every server's containers (volumes and data stay; Deploy creates them again)" },
};

const IN_PROGRESS = new Set(["deploying", "starting", "stopping", "removing"]);

// A custom compose app: one compose file, run on every one of its servers.
// Bulk actions call each server's own endpoint, so each keeps its own job,
// status, health and history — open a server for its logs and controls.
export function ComposeAppDetail({ initial, known = [] }: { initial: ComposeApp; known?: KnownServer[] }) {
  const router = useRouter();
  const [app, setApp] = useState(initial);
  const canWrite = useCanWrite();
  const [password, setPassword] = useState("");
  const [running, setRunning] = useState<BulkAction | null>(null);
  const [confirming, setConfirming] = useState<BulkAction | null>(null);

  const [name, setName] = useState(initial.name);
  const [deployDir, setDeployDir] = useState(initial.deployDir);
  const [project, setProject] = useState<ProjectState>({ manifest: initial.manifest, env: initial.env ?? "", files: initial.files ?? [] });
  const [saving, setSaving] = useState(false);

  const [newHost, setNewHost] = useState("");
  const [newPort, setNewPort] = useState("22");
  const [newUser, setNewUser] = useState(initial.servers[0]?.username ?? "root");
  const [newPassword, setNewPassword] = useState("");
  const [adding, setAdding] = useState(false);

  const refresh = useCallback(async () => {
    const res = await fetch(`/api/apps/${initial.id}`, { cache: "no-store" });
    if (res.status === 404) {
      router.push("/deploy/compose");
      return;
    }
    if (res.ok) setApp(await res.json());
  }, [initial.id, router]);

  // While any server is mid-action, keep its status current.
  const busyServers = app.servers.some((s) => IN_PROGRESS.has(s.status));
  useEffect(() => {
    if (!busyServers) return;
    const t = setInterval(refresh, 2000);
    return () => clearInterval(t);
  }, [busyServers, refresh]);

  const needsPassword = app.servers.some((s) => !s.hasSavedPassword);
  const projectDirty =
    project.manifest !== app.manifest || project.env !== (app.env ?? "") || JSON.stringify(project.files) !== JSON.stringify(app.files ?? []);
  const dirty = name !== app.name || deployDir !== app.deployDir || projectDirty;

  async function runAll(action: BulkAction) {
    setConfirming(null);
    if (needsPassword && !password) {
      toast.error("Enter the SSH password for the servers that have none saved.");
      return;
    }
    setRunning(action);
    try {
      const results = await Promise.all(
        app.servers.map(async (s) => {
          const res = await fetch(`/api/services/${s.id}/${action}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ password: s.hasSavedPassword ? "" : password }),
          });
          return { host: s.host, ok: res.ok, error: res.ok ? "" : (await res.text()).trim() };
        }),
      );
      const failed = results.filter((r) => !r.ok);
      const ok = results.length - failed.length;
      if (ok > 0) toast.success(`${BULK[action].done} on ${ok}/${results.length} server${results.length === 1 ? "" : "s"}.`);
      for (const f of failed) toast.error(`${f.host}: ${f.error}`, { duration: 10000 });
      await refresh();
    } finally {
      setRunning(null);
    }
  }

  async function save() {
    setSaving(true);
    try {
      const res = await fetch(`/api/apps/${app.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, deployDir, manifest: project.manifest, env: project.env, files: project.files }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const updated: ComposeApp = await res.json();
      setApp(updated);
      setName(updated.name);
      setDeployDir(updated.deployDir);
      setProject({ manifest: updated.manifest, env: updated.env ?? "", files: updated.files ?? [] });
      toast.success("Saved to every server. Deploy to all to roll it out.");
    } finally {
      setSaving(false);
    }
  }

  async function addKnown(k: KnownServer) {
    setAdding(true);
    try {
      const res = await fetch(`/api/apps/${app.id}/servers`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ copyLoginFrom: k.serviceId }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      toast.success(`Added ${serverLabel(k)}. Deploy to roll the app out there.`);
      await refresh();
    } finally {
      setAdding(false);
    }
  }

  async function addServer() {
    setAdding(true);
    try {
      const res = await fetch(`/api/apps/${app.id}/servers`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host: newHost.trim(), port: Number(newPort) || 22, username: newUser.trim(), password: newPassword }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      toast.success(`Added ${newHost.trim()}. Deploy to roll the app out there.`);
      setNewHost("");
      setNewPassword("");
      await refresh();
    } finally {
      setAdding(false);
    }
  }

  const [confirmDelete, setConfirmDelete] = useState(false);

  async function deleteApp() {
    const res = await fetch(`/api/apps/${app.id}`, { method: "DELETE" });
    if (!res.ok) {
      toast.error(await res.text(), { duration: 8000 });
      return;
    }
    router.push("/deploy/compose");
  }

  const healthy = app.servers.filter((s) => s.health === "healthy").length;
  const deployed = app.servers.filter((s) => s.status === "deployed").length;
  const disabled = running !== null || app.servers.length === 0;

  return (
    <div className="flex flex-col gap-6">
      <Link href={app.template === "signoz" ? "/deploy/signoz" : "/deploy/compose"} className="text-sm text-text-secondary hover:text-text-primary">
        ← {app.template === "signoz" ? "SigNoz" : "Custom compose"}
      </Link>
      <div>
        <h1 className="page-title">{app.name}</h1>
        <p className="mt-1 font-mono text-sm text-text-secondary">{app.deployDir}</p>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <StatTile hero label="Servers" value={String(app.servers.length)} hint={`${deployed} deployed`} />
        <StatTile label="Healthy" value={`${healthy}/${app.servers.length}`} hint="as of each server's last sync" />
        <StatTile label="Updated" value={new Date(app.updatedAt).toLocaleDateString()} hint="compose last saved" />
      </div>

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <h2 className="section-title">Every server</h2>
        {needsPassword && (
          <label className="flex max-w-sm flex-col gap-1 text-sm text-text-secondary">
            SSH password for servers without a saved one
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="field" autoComplete="off" />
          </label>
        )}
        <div className={`flex-wrap gap-2 ${canWrite ? "flex" : "hidden"}`}>
          {(Object.keys(BULK) as BulkAction[]).map((a) => (
            <Button
              key={a}
              variant={a === "deploy" ? "primary" : a === "down" ? "danger" : a === "stop" ? "warning" : "secondary"}
              onClick={() => (BULK[a].confirm ? setConfirming(a) : runAll(a))}
              disabled={disabled}
            >
              {running === a ? BULK[a].busy : BULK[a].label}
            </Button>
          ))}
        </div>
        {confirming && (
          <div className="flex flex-wrap items-center gap-3 rounded-2xl bg-surface-2 px-4 py-3 text-sm">
            <span className="text-text-primary">
              {BULK[confirming].label} {BULK[confirming].confirm} — on {app.servers.length} server{app.servers.length === 1 ? "" : "s"}.
            </span>
            <Button variant={confirming === "down" ? "danger" : "warning"} className="px-3 py-1 text-xs" onClick={() => runAll(confirming)}>
              Confirm
            </Button>
            <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
          </div>
        )}
        {dirty && (
          <p className="text-xs text-status-warning">The project below has unsaved changes — Deploy uses the last saved version.</p>
        )}

        {app.servers.length === 0 ? (
          <p className="text-sm text-text-muted">No servers yet. Add one below.</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {app.servers.map((s: Service) => (
              <li key={s.id} className="flex flex-col gap-1.5 rounded-2xl bg-surface-2 px-4 py-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Link href={`/deploy/compose/${app.id}/servers/${s.id}`} className="font-mono text-sm font-bold text-text-primary hover:underline">
                    {s.host}
                    {s.port !== 22 ? `:${s.port}` : ""}
                  </Link>
                  <StatusBadge role={serviceStatusRole(s.status)} label={s.status} />
                  <StatusBadge role={serviceHealthRole(s.health)} label={s.health} />
                  <PortChips ports={s.healthPorts} />
                  {(s.healthStats?.length ?? 0) > 0 && (
                    <span className="font-mono text-xs text-text-muted">
                      {s.healthStats!.length} container{s.healthStats!.length === 1 ? "" : "s"} · CPU {s.healthStats!.map((c) => c.cpu).join(" + ")}
                    </span>
                  )}
                  {app.template === "signoz" && s.status === "deployed" && (
                    <a
                      href={`http://${s.host}:8080`}
                      target="_blank"
                      rel="noreferrer"
                      className="ml-auto text-xs font-semibold text-accent hover:underline"
                    >
                      SigNoz UI ↗
                    </a>
                  )}
                  <Link
                    href={`/deploy/compose/${app.id}/servers/${s.id}`}
                    className={`${app.template === "signoz" && s.status === "deployed" ? "" : "ml-auto "}text-xs font-semibold text-accent hover:underline`}
                  >
                    Open →
                  </Link>
                </div>
                {s.healthDetail && <p className="break-all text-xs text-text-secondary">{s.healthDetail}</p>}
                <p className="text-[11px] text-text-muted">
                  {s.username}@{s.host}
                  {s.lastSyncedAt ? ` · last checked ${new Date(s.lastSyncedAt).toLocaleString()}` : " · not checked yet"}
                  {!s.hasSavedPassword && " · no saved password"}
                </p>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className={`card flex-col gap-3 p-4 sm:p-5 ${canWrite ? "flex" : "hidden"}`}>
        <h2 className="section-title">Add a server</h2>
        {(() => {
          const available = known.filter((k) => !app.servers.some((s) => s.host === k.host && s.port === k.port));
          return (
            available.length > 0 && (
              <div className="flex flex-col gap-2">
                <p className="text-xs text-text-muted">Servers the panel already knows — added with their saved login:</p>
                <div className="flex flex-wrap gap-1.5">
                  {available.map((k) => (
                    <Button key={k.serviceId} variant="secondary" className="px-3 py-1 font-mono text-xs" onClick={() => addKnown(k)} disabled={adding}>
                      + {serverLabel(k)}
                    </Button>
                  ))}
                </div>
                <p className="text-xs text-text-muted">Or a new one:</p>
              </div>
            )
          );
        })()}
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
          <label className="flex flex-1 flex-col gap-1 text-sm text-text-secondary">
            Host
            <input value={newHost} onChange={(e) => setNewHost(e.target.value)} placeholder="203.0.113.11" className="field font-mono" />
          </label>
          <label className="flex flex-col gap-1 text-sm text-text-secondary sm:w-24">
            Port
            <input value={newPort} onChange={(e) => setNewPort(e.target.value)} inputMode="numeric" className="field" />
          </label>
          <label className="flex flex-col gap-1 text-sm text-text-secondary sm:w-36">
            SSH user
            <input value={newUser} onChange={(e) => setNewUser(e.target.value)} className="field" />
          </label>
          <label className="flex flex-col gap-1 text-sm text-text-secondary sm:w-44">
            Password (optional)
            <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="field" autoComplete="off" />
          </label>
          <Button onClick={addServer} disabled={adding || !newHost.trim() || !newUser.trim()}>
            {adding ? "Adding…" : "Add"}
          </Button>
        </div>
        <p className="text-xs text-text-muted">
          It gets this app&apos;s compose and folder. To remove a server, open it and use Delete — that takes its stack
          down and removes its folder on that server.
        </p>
      </section>

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="section-title">Project</h2>
          <Button variant="secondary" onClick={save} disabled={saving || !dirty} className={canWrite ? "" : "hidden"}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <label className="flex flex-col gap-1 text-sm text-text-secondary">
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className="field" />
          </label>
          <label className="flex flex-col gap-1 text-sm text-text-secondary">
            Deploy folder (the same on every server)
            <input value={deployDir} onChange={(e) => setDeployDir(e.target.value)} className="field font-mono" />
          </label>
        </div>
        <ProjectFilesEditor value={project} onChange={setProject} disabled={saving} composeFile={app.composeFile} />
        <p className="text-xs text-text-muted">
          Saving updates every server&apos;s copy; nothing is redeployed until Deploy to all. Changing the folder only
          affects the next deploy — a stack already running in the old folder stays there until you take it down.
        </p>
      </section>

      <ComposeVolumes analysis={app.analysis} stale={projectDirty || deployDir !== app.deployDir} />

      {app.servers.length === 0 && canWrite && (
        <section className="card flex flex-wrap items-center justify-between gap-3 p-4 sm:p-5">
          <p className="text-sm text-text-secondary">This app has no servers left.</p>
          {confirmDelete ? (
            <div className="flex w-full flex-col gap-2 rounded-2xl bg-surface-2 p-3 text-sm">
              <p className="text-text-primary">
                Delete <b>{app.name}</b> from the panel? Its compose and files saved in the panel are deleted.{" "}
                <b>No container is deleted</b>: it has no servers, so nothing on any server is touched.
              </p>
              <div className="flex gap-2">
                <Button variant="danger" className="px-3 py-1.5 text-xs" onClick={deleteApp}>
                  Yes, delete the app
                </Button>
                <Button variant="secondary" className="px-3 py-1.5 text-xs" onClick={() => setConfirmDelete(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          ) : (
            <Button variant="danger" onClick={() => setConfirmDelete(true)}>
              Delete app
            </Button>
          )}
        </section>
      )}
    </div>
  );
}
