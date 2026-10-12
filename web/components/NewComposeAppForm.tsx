// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { ProjectFilesEditor, type ProjectState } from "@/components/ProjectFilesEditor";
import type { ComposeApp, ComposeTemplate } from "@/lib/api";
import { serverLabel, type KnownServer } from "@/lib/servers";

const LABEL = "flex flex-col gap-1 text-sm text-text-secondary";

const PLACEHOLDER = `services:
  app:
    image: nginx:latest
    restart: always
    ports:
      - '8080:80'
`;

interface ServerRow {
  key: number;
  host: string;
  port: string;
  username: string;
  password: string;
}

function slugify(s: string) {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "") || "app";
}

// Creates a custom compose app: one docker-compose.yml and the servers it
// runs on — written here, or started from a template (SigNoz) that fills in
// the compose, .env and files, all still editable. Nothing is deployed until
// Deploy on the app's page.
export function NewComposeAppForm({
  known = [],
  preselect,
  template,
}: {
  known?: KnownServer[];
  preselect?: string;
  template?: ComposeTemplate;
}) {
  const signoz = template?.template === "signoz";
  const router = useRouter();
  // Known servers picked for this app: added by reusing their saved login.
  const [picked, setPicked] = useState<KnownServer[]>(() => known.filter((k) => k.serviceId === preselect));
  const [name, setName] = useState(template?.name ?? "");
  const [deployDir, setDeployDir] = useState(template?.deployDir ?? "");
  const [project, setProject] = useState<ProjectState>(
    template ? { manifest: template.manifest, env: template.env, files: template.files } : { manifest: PLACEHOLDER, env: "", files: [] },
  );
  const [sameLogin, setSameLogin] = useState(true);
  const [shared, setShared] = useState({ port: "22", username: "root", password: "" });
  const [rows, setRows] = useState<ServerRow[]>(() =>
    known.some((k) => k.serviceId === preselect) ? [] : [{ key: 0, host: "", port: "22", username: "root", password: "" }],
  );
  const [nextKey, setNextKey] = useState(1);
  const [submitting, setSubmitting] = useState(false);

  function update(key: number, patch: Partial<ServerRow>) {
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const servers = [
      ...picked.map((k) => ({ copyLoginFrom: k.serviceId })),
      ...rows
        .filter((r) => r.host.trim())
        .map((r) => {
          const login = sameLogin ? shared : r;
          return { host: r.host.trim(), port: Number(login.port) || 22, username: login.username.trim(), password: login.password };
        }),
    ];
    if (servers.length === 0) {
      toast.error("Add at least one server.");
      return;
    }
    setSubmitting(true);
    try {
      const res = await fetch("/api/apps", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          deployDir: deployDir.trim(),
          manifest: project.manifest,
          env: project.env,
          files: project.files,
          servers,
          composeFile: template?.composeFile,
          template: template?.template,
        }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const app: ComposeApp = await res.json();
      router.push(`/deploy/compose/${app.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link href={signoz ? "/deploy/signoz" : "/deploy/compose"} className="text-sm text-text-secondary hover:text-text-primary">
        ← {signoz ? "SigNoz" : "Custom compose"}
      </Link>
      {signoz ? (
        <div>
          <h1 className="page-title">New SigNoz</h1>
          <p className="mt-1 text-sm text-text-secondary">
            SigNoz deployed with plain docker compose: SigNoz, ClickHouse, ClickHouse Keeper, Postgres and the
            OpenTelemetry ingester, as SigNoz&apos;s own installer lays them out, with the images pinned in .env. Every
            file is below and editable; nothing else is installed on the server. The UI is on port 8080, and
            telemetry goes to OTLP on 4317 (gRPC) and 4318 (HTTP). Give each server at least 4 GB of memory.
          </p>
        </div>
      ) : (
        <div>
          <h1 className="page-title">New custom compose</h1>
          <p className="mt-1 text-sm text-text-secondary">
            Your own docker-compose.yml — with its .env and any files it uses — deployed exactly as written to every
            server you list. Upload a project folder or write it here; edit it once and redeploy everywhere. Each server
            keeps its own status, health and history.
          </p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="flex flex-col gap-8">
        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">App</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={LABEL}>
              Name
              <input value={name} onChange={(e) => setName(e.target.value)} className="field" required />
            </label>
            <label className={LABEL}>
              Deploy folder (the same on every server)
              <input
                value={deployDir}
                onChange={(e) => setDeployDir(e.target.value)}
                placeholder={`/opt/${slugify(name)}`}
                className="field font-mono"
              />
            </label>
          </div>
        </fieldset>

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">Servers</legend>
          {known.length > 0 && (
            <div className="flex flex-col gap-2">
              <p className="text-xs text-text-muted">Servers the panel already knows — added with their saved login, no password needed:</p>
              <div className="flex flex-wrap gap-1.5">
                {known
                  .filter((k) => !picked.some((p) => p.serviceId === k.serviceId))
                  .map((k) => (
                    <Button
                      key={k.serviceId}
                      type="button"
                      variant="secondary"
                      className="px-3 py-1 font-mono text-xs"
                      onClick={() => setPicked((ps) => [...ps, k])}
                    >
                      + {serverLabel(k)}
                    </Button>
                  ))}
              </div>
            </div>
          )}
          {picked.length > 0 && (
            <ul className="flex flex-col gap-2">
              {picked.map((k) => (
                <li key={k.serviceId} className="flex flex-wrap items-center gap-2 rounded-2xl bg-surface-2 px-3 py-2.5">
                  <span className="font-mono text-sm font-semibold text-text-primary">{serverLabel(k)}</span>
                  <span className={`text-xs ${k.hasSavedPassword ? "text-status-good" : "text-text-muted"}`}>
                    {k.hasSavedPassword ? "saved login" : "no saved password — you'll enter it when deploying"}
                  </span>
                  <Button
                    type="button"
                    variant="secondary"
                    className="ml-auto px-3 py-1 text-xs"
                    onClick={() => setPicked((ps) => ps.filter((p) => p.serviceId !== k.serviceId))}
                  >
                    Remove
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <label className="flex items-center gap-2 text-sm text-text-primary">
            <input type="checkbox" checked={sameLogin} onChange={(e) => setSameLogin(e.target.checked)} />
            Same SSH login for every new server below
          </label>
          {sameLogin && (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <label className={LABEL}>
                SSH user
                <input value={shared.username} onChange={(e) => setShared({ ...shared, username: e.target.value })} className="field" required />
              </label>
              <label className={LABEL}>
                SSH password (optional)
                <input
                  type="password"
                  value={shared.password}
                  onChange={(e) => setShared({ ...shared, password: e.target.value })}
                  className="field"
                  autoComplete="off"
                />
              </label>
              <label className={LABEL}>
                SSH port
                <input value={shared.port} onChange={(e) => setShared({ ...shared, port: e.target.value })} inputMode="numeric" className="field" />
              </label>
            </div>
          )}
          <ul className="flex flex-col gap-2">
            {rows.map((r, i) => (
              <li key={r.key} className="flex flex-col gap-2 rounded-2xl bg-surface-2 p-3 sm:flex-row sm:items-end">
                <label className={`${LABEL} flex-1`}>
                  Server {i + 1} host
                  <input value={r.host} onChange={(e) => update(r.key, { host: e.target.value })} placeholder="203.0.113.10" className="field font-mono" />
                </label>
                {!sameLogin && (
                  <>
                    <label className={`${LABEL} sm:w-28`}>
                      Port
                      <input value={r.port} onChange={(e) => update(r.key, { port: e.target.value })} inputMode="numeric" className="field" />
                    </label>
                    <label className={`${LABEL} sm:w-36`}>
                      SSH user
                      <input value={r.username} onChange={(e) => update(r.key, { username: e.target.value })} className="field" />
                    </label>
                    <label className={`${LABEL} sm:w-44`}>
                      Password (optional)
                      <input
                        type="password"
                        value={r.password}
                        onChange={(e) => update(r.key, { password: e.target.value })}
                        className="field"
                        autoComplete="off"
                      />
                    </label>
                  </>
                )}
                {rows.length + picked.length > 1 && (
                  <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => setRows((rs) => rs.filter((x) => x.key !== r.key))}>
                    Remove
                  </Button>
                )}
              </li>
            ))}
          </ul>
          <Button
            type="button"
            variant="secondary"
            className="w-fit"
            onClick={() => {
              setRows((rs) => [...rs, { key: nextKey, host: "", port: "22", username: "root", password: "" }]);
              setNextKey((k) => k + 1);
            }}
          >
            + Add server
          </Button>
          <p className="text-xs text-text-muted">
            A password given here is saved encrypted, so Deploy to all won&apos;t ask for it. Leave it empty to enter it
            when you deploy instead. The SSH user needs Docker access, directly or via sudo — when sudo asks for a password, the SSH password is used.
          </p>
        </fieldset>

        <fieldset className="card-fieldset flex flex-col gap-3" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">Project</legend>
          <ProjectFilesEditor value={project} onChange={setProject} disabled={submitting} composeFile={template?.composeFile} />
        </fieldset>

        <Button type="submit" disabled={submitting} className="w-fit">
          {submitting ? "Creating…" : signoz ? "Create SigNoz" : "Create app"}
        </Button>
      </form>
    </div>
  );
}
