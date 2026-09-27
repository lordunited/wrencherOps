"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import type { NginxCandidate, PromCandidate, Service } from "@/lib/api";

const LABEL = "flex flex-col gap-1.5 text-sm font-medium text-text-secondary";

type Kind = "prometheus" | "nginx";
type Candidate = PromCandidate & Partial<NginxCandidate>;

const COPY: Record<Kind, { title: string; intro: string; label: string; back: string; list: string; tab: string }> = {
  prometheus: {
    title: "Connect an existing Prometheus",
    intro:
      "The panel looks for Prometheus containers on the server, however they were set up: any compose file, any folder, config mounted from anywhere, custom port or URL prefix. It then manages rules and targets for the one you pick.",
    label: "Prometheus",
    back: "Prometheus stack",
    list: "/deploy/prometheus",
    tab: "rules",
  },
  nginx: {
    title: "Connect an existing Nginx",
    intro:
      "The panel looks for Nginx containers on the server (official Nginx, nginx-unprivileged, OpenResty), however they were set up: any compose file, any folder, config mounted from anywhere. It then manages the configuration of the one you pick.",
    label: "Nginx",
    back: "Nginx",
    list: "/deploy/nginx",
    tab: "nginx",
  },
};

// Finds containers of one kind on a server and connects the chosen one.
// The panel never deploys, overwrites or deletes a connected stack.
export function ConnectExisting({ kind }: { kind: Kind }) {
  const copy = COPY[kind];
  const router = useRouter();
  const [host, setHost] = useState("");
  const [port, setPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [password, setPassword] = useState("");
  const [searching, setSearching] = useState(false);
  const [candidates, setCandidates] = useState<Candidate[] | null>(null);
  const [connected, setConnected] = useState<Record<string, string>>({});
  const [chosen, setChosen] = useState<string>("");
  const [name, setName] = useState("");
  const [connecting, setConnecting] = useState(false);

  const target = { kind, host: host.trim(), port: Number(port) || 22, username: username.trim(), password };

  async function find(event: FormEvent) {
    event.preventDefault();
    setSearching(true);
    setCandidates(null);
    try {
      const res = await fetch("/api/services/connect/discover", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(target),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const body: { candidates: Candidate[]; alreadyConnected: Record<string, string> } = await res.json();
      setCandidates(body.candidates);
      setConnected(body.alreadyConnected ?? {});
      const first = body.candidates.find((c) => !body.alreadyConnected?.[c.container] && c.state === "running");
      setChosen(first?.container ?? "");
      setName(first?.composeProject ?? first?.container ?? "");
    } finally {
      setSearching(false);
    }
  }

  function choose(c: Candidate) {
    setChosen(c.container);
    setName(c.composeProject || c.container);
  }

  async function connect() {
    setConnecting(true);
    try {
      const res = await fetch("/api/services/connect", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...target, container: chosen, name }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const svc: Service = await res.json();
      toast.success(`Connected to ${svc.name}.`);
      router.push(`${copy.list}/${svc.id}#${copy.tab}`);
    } finally {
      setConnecting(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Link href={copy.list} className="w-fit px-1 text-sm font-semibold text-text-secondary hover:text-text-primary">
        ← {copy.back}
      </Link>
      <div>
        <h1 className="page-title">{copy.title}</h1>
        <p className="mt-1 max-w-2xl text-sm text-text-secondary">
          {copy.intro} It never deploys, overwrites or deletes that stack. Start, Stop and Down unlock after it has been
          healthy for 5 minutes in a row.
        </p>
      </div>

      <form onSubmit={find} className="card flex flex-col gap-4 p-4 sm:p-5">
        <h2 className="section-title">Server</h2>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <label className={`${LABEL} lg:col-span-2`}>
            Host
            <input id="host" className="field" value={host} placeholder="172.22.241.41" onChange={(e) => setHost(e.target.value)} required />
          </label>
          <label className={LABEL}>
            SSH port
            <input id="port" className="field" inputMode="numeric" value={port} onChange={(e) => setPort(e.target.value)} />
          </label>
          <label className={LABEL}>
            Username
            <input id="username" className="field" value={username} onChange={(e) => setUsername(e.target.value)} required />
          </label>
          <label className={`${LABEL} lg:col-span-2`}>
            SSH password
            <input
              id="password"
              type="password"
              className="field"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="off"
              required
            />
          </label>
        </div>
        <p className="text-xs text-text-muted">
          The user needs access to Docker on that server. To change config files owned by root, it must be root or be
          allowed to use sudo without a password. The password is saved encrypted when you connect.
        </p>
        <Button type="submit" className="w-fit" disabled={searching}>
          {searching ? `Looking for ${copy.label}…` : `Find ${copy.label}`}
        </Button>
      </form>

      {candidates && (
        <section className="card flex flex-col gap-4 p-4 sm:p-5">
          <h2 className="section-title">
            {candidates.length === 0
              ? `No ${copy.label} containers found`
              : `Found ${candidates.length} ${copy.label} container${candidates.length === 1 ? "" : "s"}`}
          </h2>
          {candidates.length === 0 && (
            <p className="text-sm text-text-muted">
              No container on {host} runs a {copy.label} image. Check the address, or that this user can run docker.
            </p>
          )}
          <div className="flex flex-col gap-2" role="radiogroup" aria-label={`${copy.label} containers`}>
            {candidates.map((c) => {
              const already = connected[c.container];
              const selected = chosen === c.container;
              return (
                <button
                  key={c.container}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  disabled={!!already}
                  onClick={() => choose(c)}
                  className={`flex flex-col gap-2 rounded-2xl p-4 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
                    selected ? "bg-accent-soft ring-2 ring-accent" : "bg-surface-2 hover:bg-surface-hover"
                  }`}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-bold text-text-primary">{c.container}</span>
                    <StatusBadge role={c.state === "running" ? "good" : "critical"} label={c.state} />
                    {already && <StatusBadge role="muted" label="already connected" />}
                    <span className="ml-auto font-mono text-xs text-text-muted">{c.image}</span>
                  </div>
                  <dl className="grid grid-cols-1 gap-x-6 gap-y-1 text-xs sm:grid-cols-[auto_1fr]">
                    {c.composeProject && (
                      <>
                        <dt className="text-text-muted">Compose project</dt>
                        <dd className="break-all font-mono text-text-secondary">
                          {c.composeProject}
                          {c.workingDir ? ` in ${c.workingDir}` : ""}
                        </dd>
                      </>
                    )}
                    {c.composeFiles && c.composeFiles.length > 0 && (
                      <>
                        <dt className="text-text-muted">Compose file</dt>
                        <dd className="break-all font-mono text-text-secondary">{c.composeFiles.join(", ")}</dd>
                      </>
                    )}
                    <dt className="text-text-muted">Config</dt>
                    <dd className="break-all font-mono text-text-secondary">
                      {c.configPath}
                      {c.configHostPath ? (
                        <span className="font-sans text-status-good"> · on disk at {c.configHostPath}</span>
                      ) : (
                        <span className="font-sans text-status-warning"> · inside the container only</span>
                      )}
                    </dd>
                    {kind === "nginx" ? (
                      <>
                        <dt className="text-text-muted">conf.d</dt>
                        <dd className="break-all font-mono text-text-secondary">
                          /etc/nginx/conf.d
                          {c.confDHostPath ? (
                            <span className="font-sans text-status-good"> · on disk at {c.confDHostPath}</span>
                          ) : (
                            <span className="font-sans text-status-warning"> · inside the container only</span>
                          )}
                        </dd>
                      </>
                    ) : (
                      <>
                        <dt className="text-text-muted">API</dt>
                        <dd className="font-mono text-text-secondary">
                          port {c.port}
                          {c.routePrefix ? `, under ${c.routePrefix}` : ""}
                          {c.webConfig && <span className="font-sans text-status-warning"> · uses TLS or a password; may not be readable</span>}
                        </dd>
                      </>
                    )}
                  </dl>
                </button>
              );
            })}
          </div>

          {chosen && (
            <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
              <label className={`${LABEL} flex-1`}>
                Name in the panel
                <input id="name" className="field" value={name} onChange={(e) => setName(e.target.value)} />
              </label>
              <Button onClick={connect} disabled={connecting}>
                {connecting ? "Connecting…" : "Connect"}
              </Button>
            </div>
          )}
        </section>
      )}
    </div>
  );
}
