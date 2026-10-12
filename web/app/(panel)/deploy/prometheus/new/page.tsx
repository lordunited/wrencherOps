// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

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

export default function NewPrometheusServicePage() {
  const router = useRouter();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [deployDir, setDeployDir] = useState("/opt/prometheus-stack");

  const [prometheusPort, setPrometheusPort] = useState("9090");
  const [grafanaPort, setGrafanaPort] = useState("3000");
  const [alertmanagerPort, setAlertmanagerPort] = useState("9093");
  const [nodeExporterPort, setNodeExporterPort] = useState("9100");
  const [imageTag, setImageTag] = useState("latest");
  const [grafanaAdminPassword, setGrafanaAdminPassword] = useState("");
  const [retentionTime, setRetentionTime] = useState("15d");
  const [containerPrefix, setContainerPrefix] = useState("monitoring");
  const [networks, setNetworks] = useState("");
  const [extraVolumes, setExtraVolumes] = useState("");
  const [prometheusCpuLimit, setPrometheusCpuLimit] = useState("");
  const [prometheusMemoryLimit, setPrometheusMemoryLimit] = useState("");

  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);

    try {
      const res = await fetch("/api/services/prometheus", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name || host,
          host,
          port: Number(sshPort) || undefined,
          username,
          deployDir,
          prometheusPort: Number(prometheusPort) || undefined,
          grafanaPort: Number(grafanaPort) || undefined,
          alertmanagerPort: Number(alertmanagerPort) || undefined,
          nodeExporterPort: Number(nodeExporterPort) || undefined,
          imageTag,
          grafanaAdminPassword,
          retentionTime,
          containerPrefix,
          networks: splitLines(networks, ","),
          extraVolumes: splitLines(extraVolumes, "\n"),
          prometheusCpuLimit,
          prometheusMemoryLimit,
        }),
      });

      if (!res.ok) {
        toast.error(await res.text());
        return;
      }

      const service = await res.json();
      router.push(`/deploy/prometheus/${service.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link href="/deploy/prometheus" className="text-sm text-text-secondary hover:text-text-primary">
        ← Prometheus stack
      </Link>
      <div>
        <h1 className="page-title">New Prometheus stack</h1>
        <p className="mt-1 text-sm text-text-secondary">
          This creates the service record and generates its docker-compose.yml — Prometheus,
          Grafana, Alertmanager, and node_exporter, wired together: Prometheus scrapes itself,
          node_exporter, and Alertmanager; alerts route to Alertmanager; Grafana is
          pre-provisioned with Prometheus as a datasource. Nothing is deployed until you open
          the service and hit Deploy.
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
          <legend className="text-sm font-semibold text-text-primary">Stack configuration</legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={LABEL_CLASS}>
              Prometheus port
              <input
                value={prometheusPort}
                onChange={(e) => setPrometheusPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Grafana port
              <input
                value={grafanaPort}
                onChange={(e) => setGrafanaPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Alertmanager port
              <input
                value={alertmanagerPort}
                onChange={(e) => setAlertmanagerPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              node_exporter port
              <input
                value={nodeExporterPort}
                onChange={(e) => setNodeExporterPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Grafana admin password
              <input
                type="password"
                value={grafanaAdminPassword}
                onChange={(e) => setGrafanaAdminPassword(e.target.value)}
                placeholder="min. 8 characters"
                className={FIELD_CLASS}
                autoComplete="off"
                required
              />
            </label>
            <label className={LABEL_CLASS}>
              Image tag (shared across all four images)
              <input
                value={imageTag}
                onChange={(e) => setImageTag(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Prometheus data retention
              <input
                value={retentionTime}
                onChange={(e) => setRetentionTime(e.target.value)}
                placeholder="e.g. 15d"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Container name prefix
              <input
                value={containerPrefix}
                onChange={(e) => setContainerPrefix(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
          </div>
        </fieldset>

        <fieldset className="card-fieldset flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">
            Advanced (networks, volumes, sizing)
          </legend>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <label className={LABEL_CLASS}>
              Prometheus CPU limit (optional)
              <input
                value={prometheusCpuLimit}
                onChange={(e) => setPrometheusCpuLimit(e.target.value)}
                placeholder="e.g. 2"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Prometheus memory limit (optional)
              <input
                value={prometheusMemoryLimit}
                onChange={(e) => setPrometheusMemoryLimit(e.target.value)}
                placeholder="e.g. 4g"
                className={FIELD_CLASS}
              />
            </label>
          </div>
          <p className="text-xs text-text-muted">
            Grafana, Alertmanager, and node_exporter are lightweight and left unbounded.
          </p>
          <label className={LABEL_CLASS}>
            Networks (comma-separated; created fresh, every container joins them)
            <input
              value={networks}
              onChange={(e) => setNetworks(e.target.value)}
              placeholder="shared_net, monitoring_net"
              className={FIELD_CLASS}
            />
          </label>
          <label className={LABEL_CLASS}>
            Extra volumes on Prometheus (one per line, host:container[:ro])
            <textarea
              value={extraVolumes}
              onChange={(e) => setExtraVolumes(e.target.value)}
              placeholder={"/opt/prometheus-stack/rules:/etc/prometheus/rules:ro"}
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
