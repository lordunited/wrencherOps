// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { useCanWrite } from "@/components/Access";
import { Button } from "@/components/Button";
import { EnvFilesEditor } from "@/components/EnvFilesEditor";
import { EnvSyncPanel } from "@/components/EnvSyncPanel";
import type { ComposeApp, DetectedProject, Service } from "@/lib/api";

const trimSlash = (p: string) => p.replace(/\/+$/, "");

// ComposeEnvFiles is a compose project's environment on its service page,
// shown as soon as the page opens. A server of an app shows the app's .env
// and whether this server's still matches it: the panel keeps the .env, a
// deploy writes it, and a difference can be settled either way. A watched
// project's env files live on its server: read and edited there, with the
// server's saved login.
export function ComposeEnvFiles({ service }: { service: Service }) {
  const canWrite = useCanWrite();
  const inApp = Boolean(service.appId);
  const shown = service.type === "docker" && (inApp || service.origin === "connected");
  const [app, setApp] = useState<ComposeApp | null>(null);
  const [project, setProject] = useState<DetectedProject | null>(null);
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setProblem("");
    try {
      if (inApp) {
        const res = await fetch(`/api/apps/${encodeURIComponent(service.appId ?? "")}`, { cache: "no-store" });
        if (!res.ok) setProblem(await res.text());
        else setApp(await res.json());
        return;
      }
      const res = await fetch("/api/apps/detect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host: service.host, port: service.port, username: service.username, password: "" }),
      });
      if (!res.ok) {
        setProblem(await res.text());
        return;
      }
      const projects: DetectedProject[] = await res.json();
      const p = projects.find((x) => trimSlash(x.workingDir) === trimSlash(service.deployDir));
      if (!p) setProblem(`No compose project was found in ${service.deployDir} on ${service.host} any more.`);
      else if ((p.envFiles ?? []).length === 0) setProblem("This project's compose reads no .env and loads no env_file.");
      setProject(p ?? null);
    } finally {
      setLoading(false);
    }
  }, [inApp, service.appId, service.host, service.port, service.username, service.deployDir]);

  useEffect(() => {
    if (!shown || !canWrite) return;
    const t = setTimeout(load, 0);
    return () => clearTimeout(t);
  }, [shown, canWrite, load]);

  if (!shown) return null;

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="section-title">Environment files</h2>
          <p className="mt-1 text-xs text-text-muted">
            {inApp
              ? "This server runs an app's project: its .env is the app's, written here by each deploy."
              : "The .env compose reads for ${VARIABLES}, and every env_file its services load, read live from the server."}
          </p>
        </div>
        {canWrite && !inApp && (
          <Button variant="secondary" onClick={load} disabled={loading}>
            {loading ? "Reading…" : "Read again"}
          </Button>
        )}
      </div>

      {!canWrite && <p className="text-sm text-text-muted">Env files hold secrets: only those who can change this tab see them.</p>}
      {loading && !app && !project && <p className="text-sm text-text-muted">Reading…</p>}
      {problem && <p className="text-sm text-status-warning">{problem}</p>}

      {app && (
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold text-text-primary">The app&apos;s .env</span>
            {app.env?.trim() ? (
              <pre className="max-h-96 overflow-auto rounded-xl bg-surface-2 p-3 font-mono text-xs leading-relaxed text-text-primary">{app.env}</pre>
            ) : (
              <p className="text-sm text-text-muted">The app has no .env: none is written next to its compose.</p>
            )}
            <p className="text-xs text-text-muted">
              To change it, edit <b>.env</b> under Project on{" "}
              <Link href={`/deploy/compose/${app.id}`} className="font-semibold text-accent hover:underline">
                the {app.name} app
              </Link>{" "}
              and Save; then Deploy to all, or write it to a server below.
            </p>
          </div>
          <EnvSyncPanel appId={app.id} appEnv={app.env ?? ""} serviceId={service.id} onPulled={setApp} />
        </div>
      )}

      {project && <EnvFilesEditor project={project} creds={() => ({ host: service.host, port: service.port, username: service.username, password: "" })} />}
    </section>
  );
}
