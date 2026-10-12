// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { EnvFilesEditor } from "@/components/EnvFilesEditor";
import { ComposeStatus } from "@/components/ComposeStatus";
import { StatusBadge } from "@/components/StatusBadge";
import type { ComposeApp, DetectedProject, Service } from "@/lib/api";
import { serverLabel, type KnownServer } from "@/lib/servers";

const LABEL = "flex flex-col gap-1 text-sm text-text-secondary";

// Where the panel's own record of a project lives.
function panelHref(p: DetectedProject) {
  if (p.appId) return `/deploy/compose/${p.appId}`;
  if (!p.serviceId) return null;
  if (p.serviceType === "docker") return `/deploy/compose/watch/${p.serviceId}`;
  return `/deploy/${p.serviceType}/${p.serviceId}`;
}

// Finds the compose projects on a server — every one Docker has containers
// for, wherever its files are, plus compose files in the usual folders that
// aren't running — and adds every one with containers to the panel right
// away: as an app when it can be imported, else watched as it is. Compose
// files with no containers are listed, to import by hand.
export function DetectProjects({
  initial,
  known = [],
}: {
  initial?: { host?: string; port?: string; user?: string };
  known?: KnownServer[];
}) {
  const router = useRouter();
  const [host, setHost] = useState(initial?.host ?? "");
  const [port, setPort] = useState(initial?.port ?? "22");
  const [username, setUsername] = useState(initial?.user ?? "root");
  const [password, setPassword] = useState("");
  const [scanning, setScanning] = useState(false);
  const [projects, setProjects] = useState<DetectedProject[] | null>(null);
  const [acting, setActing] = useState<string | null>(null);
  // What the last scan added to the panel, by folder.
  const [added, setAdded] = useState<Record<string, AddedProject>>({});

  const creds = () => ({
    host: host.trim(),
    port: Number(port) || 22,
    username: username.trim(),
    password: askPassword ? password : "",
  });
  // The login typed matches a server whose password is saved: no need to type it.
  const saved = known.some(
    (k) =>
      k.hasSavedPassword &&
      k.host === host.trim() &&
      k.username === username.trim() &&
      k.port === (Number(port) || 22),
  );
  // With a saved login there's no password field, unless you choose to type
  // one, or the server refused the saved one.
  const [typing, setTyping] = useState(false);
  const askPassword = !saved || typing;

  async function scan(event?: FormEvent) {
    event?.preventDefault();
    setScanning(true);
    try {
      const res = await fetch("/api/apps/detect/add", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(creds()),
      });
      if (!res.ok) {
        const msg = await res.text();
        if (msg.includes("refused the saved")) setTyping(true);
        toast.error(msg, { duration: 8000 });
        return;
      }
      const body: { added: AddedProject[]; projects: DetectedProject[] } =
        await res.json();
      setProjects(body.projects);
      setAdded(Object.fromEntries(body.added.map((a) => [a.workingDir, a])));
      const ok = body.added.filter((a) => !a.error);
      const failed = body.added.length - ok.length;
      if (ok.length > 0)
        toast.success(
          `Added ${ok.length} compose project${ok.length === 1 ? "" : "s"} to the panel.`,
        );
      if (failed > 0)
        toast.error(`${failed} couldn't be added; each says why below.`, {
          duration: 10000,
        });
    } finally {
      setScanning(false);
    }
  }

  // Arriving from a server's "Scan again" with its login saved: scan right
  // away, nothing to fill in.
  const autoScanned = useRef(false);
  useEffect(() => {
    if (autoScanned.current || !initial?.host || !saved) return;
    autoScanned.current = true;
    scan();
  });

  async function act(p: DetectedProject, action: "import" | "watch") {
    setActing(`${action}:${p.workingDir}`);
    try {
      const res = await fetch(`/api/apps/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...creds(), workingDir: p.workingDir }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      if (action === "import") {
        const body: { app: ComposeApp; notes: string[] } = await res.json();
        toast.success(`Imported ${p.name} as an app.`);
        for (const n of body.notes) toast.message(n, { duration: 12000 });
        router.push(`/deploy/compose/${body.app.id}`);
      } else {
        const svc: Service = await res.json();
        toast.success(`Watching ${p.name}.`);
        router.push(`/deploy/compose/watch/${svc.id}`);
      }
    } finally {
      setActing(null);
    }
  }

  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="page-title">Detect compose projects</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Scans a server for compose projects: every one Docker has containers
          for, wherever its files are, plus compose files under /opt, /srv,
          /home, /root, /var/www, /docker, /data and /apps that aren&apos;t
          running. Every project with containers is added to the panel as soon
          as it&apos;s found — as an app you can edit and deploy, or watched as
          it is when it can&apos;t be imported. Nothing on the server is
          changed.
        </p>
      </div>

      {known.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs text-text-muted">
            Servers the panel knows:
          </span>
          {known.map((k) => (
            <Button
              key={k.serviceId}
              type="button"
              variant="secondary"
              className="px-3 py-1 font-mono text-xs"
              onClick={() => {
                setHost(k.host);
                setPort(String(k.port));
                setUsername(k.username);
                setPassword("");
              }}
            >
              {serverLabel(k)}
              {k.hasSavedPassword ? "" : " (no saved password)"}
            </Button>
          ))}
        </div>
      )}

      <form
        onSubmit={scan}
        className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:p-5"
      >
        <label className={`${LABEL} flex-1`}>
          Host
          <input
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder="203.0.113.10"
            className="field font-mono"
            required
          />
        </label>
        <label className={`${LABEL} sm:w-24`}>
          SSH port
          <input
            value={port}
            onChange={(e) => setPort(e.target.value)}
            inputMode="numeric"
            className="field"
          />
        </label>
        <label className={`${LABEL} sm:w-36`}>
          SSH user
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="field"
            required
          />
        </label>
        {askPassword ? (
          <label className={`${LABEL} sm:w-44`}>
            Password
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="field"
              autoComplete="off"
              required
            />
          </label>
        ) : (
          <div className={`${LABEL} sm:w-44`}>
            Password
            <span className="flex h-[38px] items-center gap-2 text-sm text-status-good">
              Saved ✓
              <button
                type="button"
                onClick={() => setTyping(true)}
                className="text-xs font-semibold text-accent hover:underline"
              >
                use another
              </button>
            </span>
          </div>
        )}
        <Button
          type="submit"
          disabled={scanning || !host.trim() || (askPassword && !password)}
        >
          {scanning ? "Scanning…" : projects ? "Scan again" : "Scan"}
        </Button>
      </form>

      {projects && projects.length === 0 && (
        <div className="card px-6 py-10 text-center text-sm text-text-muted">
          No compose projects found on {host}.
        </div>
      )}

      {projects &&
        projects.length > 0 &&
        (() => {
          const inPanel = projects.filter((p) => panelHref(p)).length;
          const addedNow = Object.values(added).filter((a) => !a.error).length;
          const onDisk = projects.filter(
            (p) => !p.status && !panelHref(p),
          ).length;
          return (
            <div className="card flex flex-wrap items-center gap-x-4 gap-y-1 px-4 py-3 text-sm">
              <span className="font-semibold text-text-primary">
                {projects.length} found
              </span>
              <span className="text-status-good">
                {inPanel} in the panel
                {addedNow > 0 ? ` (${addedNow} added just now)` : ""}
              </span>
              {onDisk > 0 && (
                <span className="text-text-muted">
                  {onDisk} not running — files on disk only, import by hand
                </span>
              )}
              <Link
                href="/deploy/compose"
                className="ml-auto text-xs font-semibold text-accent hover:underline"
              >
                Go to Custom compose →
              </Link>
            </div>
          );
        })()}

      {projects && projects.length > 0 && (
        <ul className="flex flex-col gap-2">
          {projects.map((p) => {
            const href = panelHref(p);
            const result = added[p.workingDir];
            return (
              <li
                key={p.workingDir}
                className="card flex flex-col gap-3 p-4 sm:p-5"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-base font-bold text-text-primary">
                    {p.name}
                  </span>
{p.status ? (
                    <ComposeStatus status={p.status} />
                  ) : (
                    <StatusBadge
                      role="muted"
                      label="not running — files on disk only"
                    />
                  )}
                  {href && (
                    <Link
                      href={href}
                      className="ml-auto text-xs font-semibold text-accent hover:underline"
                    >
                      In the panel{p.serviceName ? ` as ${p.serviceName}` : ""}{" "}
                      →
                    </Link>
                  )}
                </div>
                <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs">
                  <dt className="text-text-muted">Folder</dt>
                  <dd className="break-all font-mono text-text-secondary">
                    {p.workingDir}
                  </dd>
                  <dt className="text-text-muted">Compose</dt>
                  <dd className="break-all font-mono text-text-secondary">
                    {p.configFiles.join(", ")}
                  </dd>
                </dl>
                {result && (
                  <div
                    className={`flex flex-col gap-1 rounded-xl px-3 py-2 text-xs ${!result.error ? "bg-surface-2 text-status-good" : "bg-surface-2 text-status-critical"}`}
                  >
                    <span className="font-semibold">
                      {!result.error ? "✓ " : "✗ "}
                      {result.error
                        ? `Couldn't add it: ${result.error}`
                        : result.as === "app"
                          ? "Added to the panel as an app."
                          : "Added to the panel, watched as it is (it can't be imported as an app)."}
                      {result.href && (
                        <Link
                          href={result.href}
                          className="ml-2 text-accent hover:underline"
                        >
                          Open →
                        </Link>
                      )}
                    </span>
                    {result.notes?.map((n) => (
                      <span key={n} className="text-text-secondary">
                        {n}
                      </span>
                    ))}
                  </div>
                )}
                <EnvFilesEditor project={p} creds={creds} />
                {!href && (
                  <div className="flex flex-col gap-2 sm:flex-row sm:items-start">
                    <div className="flex flex-1 flex-col gap-1">
                      <Button
                        className="w-fit"
                        onClick={() => act(p, "import")}
                        disabled={!p.importable || acting !== null}
                      >
                        {acting === `import:${p.workingDir}`
                          ? "Importing…"
                          : "Import as app"}
                      </Button>
                      <p className="text-xs text-text-muted">
                        {p.importable
                          ? "Brings its compose, .env and the files it mounts into the panel, with this server as the app's first. Edit it, then deploy it here and to more servers."
                          : `Can't import: ${p.importProblem}.`}
                      </p>
                    </div>
                    <div className="flex flex-1 flex-col gap-1">
                      <Button
                        variant="secondary"
                        className="w-fit"
                        onClick={() => act(p, "watch")}
                        disabled={!p.watchable || acting !== null}
                      >
                        {acting === `watch:${p.workingDir}`
                          ? "Connecting…"
                          : "Watch"}
                      </Button>
                      <p className="text-xs text-text-muted">
                        {p.watchable
                          ? "Monitors it exactly as it is — health, containers, history — and never changes it. Start/Stop/Down unlock after it's been healthy for 5 minutes."
                          : `Can't watch: ${p.watchProblem}.`}
                      </p>
                    </div>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

// What scanning did with one project it added to the panel.
interface AddedProject {
  workingDir: string;
  name: string;
  as: "app" | "watched";
  href?: string;
  notes?: string[];
  error?: string;
}
