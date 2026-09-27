"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";

const FIELD_CLASS =
  "field";
const LABEL_CLASS = "flex flex-col gap-1 text-sm text-text-secondary";

function splitLines(value: string, separator: string): string[] {
  return value
    .split(separator)
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

export default function NewGitLabServicePage() {
  const router = useRouter();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [deployDir, setDeployDir] = useState("/opt/gitlab");

  const [externalUrl, setExternalUrl] = useState("");
  const [httpPort, setHttpPort] = useState("80");
  const [httpsPort, setHttpsPort] = useState("");
  const [gitSshPort, setGitSshPort] = useState("2222");
  const [rootPassword, setRootPassword] = useState("");
  const [imageTag, setImageTag] = useState("latest");
  const [containerName, setContainerName] = useState("gitlab");
  const [networks, setNetworks] = useState("");
  const [extraVolumes, setExtraVolumes] = useState("");
  const [extraOmnibusConfig, setExtraOmnibusConfig] = useState("");
  const [cpuLimit, setCpuLimit] = useState("");
  const [memoryLimit, setMemoryLimit] = useState("");

  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);

    try {
      const res = await fetch("/api/services/gitlab", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name || host,
          host,
          port: Number(sshPort) || undefined,
          username,
          deployDir,
          externalUrl,
          httpPort: Number(httpPort) || undefined,
          httpsPort: httpsPort ? Number(httpsPort) : undefined,
          sshPort: Number(gitSshPort) || undefined,
          rootPassword,
          imageTag,
          containerName,
          networks: splitLines(networks, ","),
          extraVolumes: splitLines(extraVolumes, "\n"),
          extraOmnibusConfig,
          cpuLimit,
          memoryLimit,
        }),
      });

      if (!res.ok) {
        toast.error(await res.text());
        return;
      }

      const service = await res.json();
      router.push(`/deploy/gitlab/${service.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link href="/deploy/gitlab" className="text-sm text-text-secondary hover:text-text-primary">
        ← GitLab
      </Link>
      <div>
        <h1 className="page-title">New GitLab service</h1>
        <p className="mt-1 text-sm text-text-secondary">
          This creates the service record and generates its docker-compose.yml. Nothing is
          deployed until you open the service and hit Deploy.
        </p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-8">
        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">Target server</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
            <label className={`${LABEL_CLASS} sm:col-span-2`}>
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

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">
            GitLab configuration
          </legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={`${LABEL_CLASS} sm:col-span-2`}>
              External URL
              <input
                value={externalUrl}
                onChange={(e) => setExternalUrl(e.target.value)}
                placeholder="http://203.0.113.10"
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={LABEL_CLASS}>
              Initial root password
              <input
                type="password"
                value={rootPassword}
                onChange={(e) => setRootPassword(e.target.value)}
                placeholder="min. 8 characters"
                className={FIELD_CLASS}
                autoComplete="off"
                required
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
              HTTP port
              <input
                value={httpPort}
                onChange={(e) => setHttpPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              HTTPS port (optional)
              <input
                value={httpsPort}
                onChange={(e) => setHttpsPort(e.target.value)}
                inputMode="numeric"
                placeholder="leave blank to skip"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Git SSH port
              <input
                value={gitSshPort}
                onChange={(e) => setGitSshPort(e.target.value)}
                inputMode="numeric"
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

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">
            Advanced (networks, volumes, extra config)
          </legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
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
          </div>
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
              placeholder={"/opt/gitlab/certs:/etc/gitlab/ssl:ro"}
              rows={3}
              className={`${FIELD_CLASS} font-mono`}
            />
          </label>
          <label className={LABEL_CLASS}>
            Extra GitLab configuration (raw gitlab.rb lines, one per line)
            <textarea
              value={extraOmnibusConfig}
              onChange={(e) => setExtraOmnibusConfig(e.target.value)}
              placeholder={
                "registry_external_url 'https://registry.example.com'\ngitlab_rails['smtp_enable'] = true\ngitlab_rails['smtp_address'] = \"smtp.example.com\""
              }
              rows={6}
              className={`${FIELD_CLASS} font-mono`}
            />
            <span className="text-xs text-text-muted">
              Appended verbatim into GITLAB_OMNIBUS_CONFIG. Any setting from GitLab&apos;s own
              gitlab.rb reference works here — SMTP, LDAP, registry, backups, and so on.
            </span>
          </label>
        </fieldset>

        <Button type="submit" disabled={submitting} className="w-fit">
          {submitting ? "Creating…" : "Create service"}
        </Button>
      </form>
    </div>
  );
}
