"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";

const LABEL = "flex flex-col gap-1.5 text-sm font-medium text-text-secondary";

function splitList(value: string, separator: string): string[] {
  return value
    .split(separator)
    .map((s) => s.trim())
    .filter(Boolean);
}

export default function NewNginxPage() {
  const router = useRouter();
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [deployDir, setDeployDir] = useState("/opt/nginx");
  const [httpPort, setHttpPort] = useState("80");
  const [httpsPort, setHttpsPort] = useState("443");
  const [imageTag, setImageTag] = useState("stable-alpine");
  const [containerName, setContainerName] = useState("nginx");
  const [networks, setNetworks] = useState("");
  const [extraVolumes, setExtraVolumes] = useState("");
  const [cpuLimit, setCpuLimit] = useState("");
  const [memoryLimit, setMemoryLimit] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    try {
      const res = await fetch("/api/services/nginx", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name || host,
          host,
          port: Number(sshPort) || undefined,
          username,
          deployDir,
          httpPort: Number(httpPort) || undefined,
          httpsPort: httpsPort ? Number(httpsPort) : 0,
          imageTag,
          containerName,
          networks: splitList(networks, ","),
          extraVolumes: splitList(extraVolumes, "\n"),
          cpuLimit,
          memoryLimit,
        }),
      });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const service = await res.json();
      router.push(`/deploy/nginx/${service.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <Link href="/deploy/nginx" className="w-fit px-1 text-sm font-semibold text-text-secondary hover:text-text-primary">
        ← Nginx
      </Link>
      <div>
        <h1 className="page-title">New Nginx</h1>
        <p className="mt-1 max-w-2xl text-sm text-text-secondary">
          Runs the official Nginx image with its configuration in a folder on the server. It starts with a default
          server that answers <span className="font-mono">/healthz</span> and nothing else. Add reverse proxies, load
          balancers and redirects from its Nginx config tab once it&apos;s deployed.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-6">
        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend>Target server</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={LABEL}>
              Name
              <input id="name" className="field" value={name} placeholder="defaults to host" onChange={(e) => setName(e.target.value)} />
            </label>
            <label className={LABEL}>
              Host
              <input id="host" className="field" value={host} placeholder="203.0.113.10" onChange={(e) => setHost(e.target.value)} required />
            </label>
            <label className={LABEL}>
              SSH port
              <input id="ssh-port" className="field" inputMode="numeric" value={sshPort} onChange={(e) => setSshPort(e.target.value)} />
            </label>
            <label className={LABEL}>
              Username
              <input id="username" className="field" value={username} onChange={(e) => setUsername(e.target.value)} required />
            </label>
            <label className={`${LABEL} sm:col-span-2`}>
              Deploy directory (on the target server)
              <input id="deploy-dir" className="field" value={deployDir} onChange={(e) => setDeployDir(e.target.value)} />
              <span className="text-xs font-normal text-text-muted">
                Config lives in <span className="font-mono">conf.d/</span> here, certificates in{" "}
                <span className="font-mono">certs/</span> (mounted read-only at /etc/nginx/certs).
              </span>
            </label>
          </div>
          <p className="text-xs text-text-muted">No password here. You&apos;ll enter it when you deploy.</p>
        </fieldset>

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend>Nginx</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={LABEL}>
              HTTP port on the server
              <input id="http-port" className="field" inputMode="numeric" value={httpPort} onChange={(e) => setHttpPort(e.target.value)} />
            </label>
            <label className={LABEL}>
              HTTPS port on the server (empty for none)
              <input id="https-port" className="field" inputMode="numeric" value={httpsPort} onChange={(e) => setHttpsPort(e.target.value)} />
            </label>
            <label className={LABEL}>
              Image tag
              <input id="image-tag" className="field" value={imageTag} onChange={(e) => setImageTag(e.target.value)} />
            </label>
            <label className={LABEL}>
              Container name
              <input id="container-name" className="field" value={containerName} onChange={(e) => setContainerName(e.target.value)} />
            </label>
          </div>
        </fieldset>

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend>Advanced</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={`${LABEL} sm:col-span-2`}>
              Docker networks to join (comma-separated), to reach the apps Nginx proxies to
              <input id="networks" className="field" value={networks} placeholder="app_net, api_net" onChange={(e) => setNetworks(e.target.value)} />
            </label>
            <label className={`${LABEL} sm:col-span-2`}>
              Extra volumes (one per line, host:container[:ro])
              <textarea id="volumes" className="field min-h-20 font-mono" value={extraVolumes} placeholder="/etc/letsencrypt:/etc/letsencrypt:ro" onChange={(e) => setExtraVolumes(e.target.value)} />
            </label>
            <label className={LABEL}>
              CPU limit (optional)
              <input id="cpu" className="field" value={cpuLimit} placeholder="e.g. 1" onChange={(e) => setCpuLimit(e.target.value)} />
            </label>
            <label className={LABEL}>
              Memory limit (optional)
              <input id="memory" className="field" value={memoryLimit} placeholder="e.g. 256m" onChange={(e) => setMemoryLimit(e.target.value)} />
            </label>
          </div>
        </fieldset>

        <Button type="submit" disabled={submitting} className="w-fit">
          {submitting ? "Creating…" : "Create service"}
        </Button>
      </form>
    </div>
  );
}
