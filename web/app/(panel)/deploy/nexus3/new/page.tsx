"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";

const FIELD_CLASS =
  "rounded-md border border-border bg-background px-3 py-2 text-sm text-text-primary";
const LABEL_CLASS = "flex flex-col gap-1 text-sm text-text-secondary";

function splitLines(value: string, separator: string): string[] {
  return value
    .split(separator)
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

export default function NewNexus3ServicePage() {
  const router = useRouter();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [deployDir, setDeployDir] = useState("/opt/nexus3");

  const [httpPort, setHttpPort] = useState("8081");
  const [extraPort, setExtraPort] = useState("");
  const [imageTag, setImageTag] = useState("latest");
  const [containerName, setContainerName] = useState("nexus3");
  const [networks, setNetworks] = useState("");
  const [extraVolumes, setExtraVolumes] = useState("");
  const [cpuLimit, setCpuLimit] = useState("");
  const [memoryLimit, setMemoryLimit] = useState("");
  const [javaHeap, setJavaHeap] = useState("");

  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);

    try {
      const res = await fetch("/api/services/nexus3", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name || host,
          host,
          port: Number(sshPort) || undefined,
          username,
          deployDir,
          httpPort: Number(httpPort) || undefined,
          extraPort: extraPort ? Number(extraPort) : undefined,
          imageTag,
          containerName,
          networks: splitLines(networks, ","),
          extraVolumes: splitLines(extraVolumes, "\n"),
          cpuLimit,
          memoryLimit,
          javaHeap,
        }),
      });

      if (!res.ok) {
        toast.error(await res.text());
        return;
      }

      const service = await res.json();
      router.push(`/deploy/nexus3/${service.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link href="/deploy/nexus3" className="text-sm text-text-secondary hover:text-text-primary">
        ← Nexus3
      </Link>
      <div>
        <h1 className="text-xl font-semibold text-text-primary">New Nexus3 service</h1>
        <p className="mt-1 text-sm text-text-secondary">
          This creates the service record and generates its docker-compose.yml. Nothing is
          deployed until you open the service and hit Deploy.
        </p>
        <p className="mt-2 text-sm text-status-warning">
          Nexus has no config knob to set an admin password up front — it generates one into
          /nexus-data/admin.password inside the container on first boot. Check the deploy log
          for exactly where.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-8">
        <fieldset className="flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">Target server</legend>
          <div className="grid grid-cols-2 gap-4">
            <label className={LABEL_CLASS}>
              Name
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="defaults to host"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Host
              <input
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="203.0.113.10"
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={LABEL_CLASS}>
              SSH port
              <input
                value={sshPort}
                onChange={(e) => setSshPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Username
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={`${LABEL_CLASS} col-span-2`}>
              Deploy directory (on the target server)
              <input
                value={deployDir}
                onChange={(e) => setDeployDir(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
          </div>
          <p className="text-xs text-text-muted">
            No password here — you&apos;ll enter it when you actually deploy.
          </p>
        </fieldset>

        <fieldset className="flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">
            Nexus3 configuration
          </legend>
          <div className="grid grid-cols-2 gap-4">
            <label className={LABEL_CLASS}>
              HTTP port
              <input
                value={httpPort}
                onChange={(e) => setHttpPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Additional port (optional)
              <input
                value={extraPort}
                onChange={(e) => setExtraPort(e.target.value)}
                inputMode="numeric"
                placeholder="e.g. a docker registry connector"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Image tag
              <input
                value={imageTag}
                onChange={(e) => setImageTag(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Container name
              <input
                value={containerName}
                onChange={(e) => setContainerName(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
          </div>
        </fieldset>

        <fieldset className="flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">
            Advanced (networks, volumes, production sizing)
          </legend>
          <div className="grid grid-cols-3 gap-4">
            <label className={LABEL_CLASS}>
              CPU limit (optional)
              <input
                value={cpuLimit}
                onChange={(e) => setCpuLimit(e.target.value)}
                placeholder="e.g. 2"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Memory limit (optional)
              <input
                value={memoryLimit}
                onChange={(e) => setMemoryLimit(e.target.value)}
                placeholder="e.g. 4g"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              JVM heap (optional)
              <input
                value={javaHeap}
                onChange={(e) => setJavaHeap(e.target.value)}
                placeholder="e.g. 2703m"
                className={FIELD_CLASS}
              />
            </label>
          </div>
          <p className="text-xs text-text-muted">
            JVM heap sets Nexus&apos;s min/max heap and max direct memory to the same value —
            Sonatype&apos;s own production sizing guidance. Leave blank to let the image
            auto-size from container memory (fine for evaluation, not for production).
          </p>
          <label className={LABEL_CLASS}>
            Networks (comma-separated; created fresh, container joins them)
            <input
              value={networks}
              onChange={(e) => setNetworks(e.target.value)}
              placeholder="shared_net, monitoring_net"
              className={FIELD_CLASS}
            />
          </label>
          <label className={LABEL_CLASS}>
            Extra volumes (one per line, host:container[:ro])
            <textarea
              value={extraVolumes}
              onChange={(e) => setExtraVolumes(e.target.value)}
              rows={3}
              className={`${FIELD_CLASS} font-mono`}
            />
          </label>
        </fieldset>

        <Button type="submit" disabled={submitting} className="w-fit">
          {submitting ? "Creating…" : "Create service"}
        </Button>
      </form>
    </div>
  );
}
