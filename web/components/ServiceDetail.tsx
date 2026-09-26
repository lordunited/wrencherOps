"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import { YamlEditor } from "@/components/YamlEditor";
import type { DeployJob, Service, ServiceStatus } from "@/lib/api";
import { serviceHealthRole, serviceStatusRole, type StatusRole } from "@/lib/format";

const FIELD_CLASS =
  "rounded-md border border-border bg-background px-3 py-2 text-sm text-text-primary";
const LABEL_CLASS = "flex flex-col gap-1 text-sm text-text-secondary";

const JOB_STATUS_ROLE: Record<DeployJob["status"], StatusRole> = {
  running: "warning",
  success: "good",
  failed: "critical",
};

const JOB_KIND_LABEL: Record<"deploy" | "start" | "stop" | "down" | "destroy", string> = {
  deploy: "Deployment log",
  start: "Start log",
  stop: "Stop log",
  down: "Down log",
  destroy: "Delete log",
};

type ControlAction = "start" | "stop" | "down";

const CONTROL_TRANSIENT_STATUS: Record<ControlAction, ServiceStatus> = {
  start: "starting",
  stop: "stopping",
  down: "removing",
};

const CONTROL_SUCCESS_STATUS: Record<ControlAction, ServiceStatus> = {
  start: "deployed",
  stop: "stopped",
  down: "down",
};

export function ServiceDetail({
  initial,
  backHref,
  backLabel,
}: {
  initial: Service;
  backHref: string;
  backLabel: string;
}) {
  const router = useRouter();

  const [service, setService] = useState(initial);
  const [name, setName] = useState(initial.name);
  const [host, setHost] = useState(initial.host);
  const [port, setPort] = useState(String(initial.port));
  const [username, setUsername] = useState(initial.username);
  const [deployDir, setDeployDir] = useState(initial.deployDir);
  const [manifest, setManifest] = useState(initial.manifest);

  const [saving, setSaving] = useState(false);
  const [justSaved, setJustSaved] = useState(false);

  const [deleting, setDeleting] = useState(false);

  const [showPassword, setShowPassword] = useState(false);
  const [password, setPassword] = useState("");
  const [deploying, setDeploying] = useState(false);
  const [job, setJob] = useState<DeployJob | null>(null);
  const [jobKind, setJobKind] = useState<"deploy" | "start" | "stop" | "down" | "destroy">("deploy");

  const [showSyncPassword, setShowSyncPassword] = useState(false);
  const [syncPassword, setSyncPassword] = useState("");
  const [syncing, setSyncing] = useState(false);

  const [controlAction, setControlAction] = useState<ControlAction | null>(null);
  const [controlPassword, setControlPassword] = useState("");
  const [controlBusy, setControlBusy] = useState(false);

  const [confirmingDestroy, setConfirmingDestroy] = useState(false);
  const [destroyPassword, setDestroyPassword] = useState("");

  const isCompose = service.type !== "kubernetes";

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  useEffect(() => {
    if (logRef.current) logRef.current.scrollTop = logRef.current.scrollHeight;
  }, [job?.log.length]);

  async function handleSave() {
    setSaving(true);
    try {
      const res = await fetch(`/api/services/${service.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          host,
          port: Number(port) || undefined,
          username,
          deployDir,
          manifest,
        }),
      });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const updated: Service = await res.json();
      setService(updated);
      setJustSaved(true);
      setTimeout(() => setJustSaved(false), 4000);
    } finally {
      setSaving(false);
    }
  }

  async function removeRecordOnly() {
    setDeleting(true);
    try {
      await fetch(`/api/services/${service.id}`, { method: "DELETE" });
      router.push(backHref);
    } finally {
      setDeleting(false);
    }
  }

  async function destroyAndDelete(suppliedPassword: string) {
    setDeleting(true);
    setJobKind("destroy");
    setJob(null);
    try {
      const res = await fetch(`/api/services/${service.id}/destroy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: suppliedPassword }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        setDeleting(false);
        return;
      }
      const { jobId } = await res.json();
      setDestroyPassword("");
      setConfirmingDestroy(false);
      setService((s) => ({ ...s, status: "removing" }));
      setJob({ id: jobId, status: "running", log: [], startedAt: "", updatedAt: "" });
      pollDestroyJob(jobId);
    } finally {
      setDeleting(false);
    }
  }

  function pollDestroyJob(jobId: string) {
    pollRef.current = setInterval(async () => {
      const res = await fetch(`/api/deploy/jobs/${jobId}`, { cache: "no-store" });
      if (!res.ok) return;
      const snapshot: DeployJob = await res.json();
      setJob(snapshot);
      if (snapshot.status === "running") return;
      if (pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
      }
      if (snapshot.status === "success") {
        router.push(backHref);
      } else {
        setService((s) => ({ ...s, status: "failed" }));
      }
    }, 1500);
  }

  // A never-deployed (isCompose && "pending") or Kubernetes-type record has
  // nothing real to tear down, so Delete just removes the record — no SSH
  // needed. Anything else that's ever been deployed via Docker Compose gets
  // the full "docker compose down -v" + delete-the-directory treatment.
  const needsTeardown = isCompose && service.status !== "pending";

  function handleDeleteClick() {
    if (!needsTeardown) {
      const message = isCompose
        ? `Delete "${service.name}"? This only removes the record — nothing has been deployed yet.`
        : `Delete "${service.name}"? This only removes the record — it won't touch anything already applied to the cluster.`;
      if (confirm(message)) removeRecordOnly();
      return;
    }
    const confirmed = confirm(
      `Delete "${service.name}"? This runs "docker compose down -v" and permanently deletes ${service.deployDir} on the server — containers, networks, volumes, and all data. This cannot be undone.`,
    );
    if (!confirmed) return;
    if (service.hasSavedPassword) {
      destroyAndDelete("");
    } else {
      setConfirmingDestroy(true);
    }
  }

  function pollJob(jobId: string, successStatus: ServiceStatus) {
    pollRef.current = setInterval(async () => {
      const res = await fetch(`/api/deploy/jobs/${jobId}`, { cache: "no-store" });
      if (!res.ok) return;
      const snapshot: DeployJob = await res.json();
      setJob(snapshot);
      if (snapshot.status !== "running" && pollRef.current) {
        clearInterval(pollRef.current);
        pollRef.current = null;
        setService((s) => ({ ...s, status: snapshot.status === "success" ? successStatus : "failed" }));
      }
    }, 1500);
  }

  async function handleDeploy() {
    setDeploying(true);
    setJobKind("deploy");
    setJob(null);
    try {
      const res = await fetch(`/api/services/${service.id}/deploy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const { jobId } = await res.json();
      setPassword("");
      setShowPassword(false);
      setService((s) => ({ ...s, status: "deploying" }));
      setJob({ id: jobId, status: "running", log: [], startedAt: "", updatedAt: "" });
      pollJob(jobId, "deployed");
    } finally {
      setDeploying(false);
    }
  }

  async function runControl(action: ControlAction, suppliedPassword: string) {
    setControlBusy(true);
    setJobKind(action);
    setJob(null);
    try {
      const res = await fetch(`/api/services/${service.id}/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: suppliedPassword }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const { jobId } = await res.json();
      setControlPassword("");
      setControlAction(null);
      setService((s) => ({ ...s, status: CONTROL_TRANSIENT_STATUS[action] }));
      setJob({ id: jobId, status: "running", log: [], startedAt: "", updatedAt: "" });
      pollJob(jobId, CONTROL_SUCCESS_STATUS[action]);
    } finally {
      setControlBusy(false);
    }
  }

  function requestControl(action: ControlAction) {
    if (action === "down") {
      const confirmed = confirm(
        `Take down "${service.name}"? This runs "docker compose down" — containers and the default network are removed. Named volumes (persisted data) are left intact.`,
      );
      if (!confirmed) return;
    }
    if (service.hasSavedPassword) {
      runControl(action, "");
    } else {
      setControlAction(action);
    }
  }

  async function handleSync() {
    setSyncing(true);
    try {
      const res = await fetch(`/api/services/${service.id}/sync`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: syncPassword }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 8000 });
        return;
      }
      const updated: Service = await res.json();
      setService(updated);
      setSyncPassword("");
      setShowSyncPassword(false);
    } finally {
      setSyncing(false);
    }
  }

  const busy = saving || deploying || syncing || controlBusy || job?.status === "running";

  // Only meaningful once something's actually been deployed. Start runs
  // "docker compose up -d", which works whether the service is currently
  // stopped or fully down (recreates removed containers), so it's only
  // ruled out pre-deploy. Stop/Down need something still there to act on.
  const canStart = service.status !== "pending";
  const canStop = !["pending", "stopped", "down"].includes(service.status);
  const canDown = !["pending", "down"].includes(service.status);

  return (
    <div className="flex flex-col gap-8">
      <Link href={backHref} className="text-sm text-text-secondary hover:text-text-primary">
        ← {backLabel}
      </Link>
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-xl font-semibold text-text-primary">{service.name}</h1>
            <StatusBadge role={serviceStatusRole(service.status)} label={service.status} />
            <StatusBadge role={serviceHealthRole(service.health)} label={service.health} />
          </div>
          <p className="mt-1 text-sm text-text-secondary">
            {service.type} · {service.host}:{service.port}
          </p>
          <p className="mt-1 text-xs text-text-muted">
            {service.lastSyncedAt
              ? `Last synced ${new Date(service.lastSyncedAt).toLocaleString()} — ${service.healthDetail}`
              : "Never synced"}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => (service.hasSavedPassword ? handleSync() : setShowSyncPassword(true))}
            disabled={busy}
          >
            {syncing ? "Syncing…" : "Sync"}
          </Button>
          {isCompose && (
            <>
              <Button
                variant="success"
                onClick={() => requestControl("start")}
                disabled={busy || !canStart}
                title={canStart ? undefined : "Deploy first — nothing to start yet"}
              >
                {controlBusy && jobKind === "start" ? "Starting…" : "Start"}
              </Button>
              <Button
                variant="warning"
                onClick={() => requestControl("stop")}
                disabled={busy || !canStop}
                title={canStop ? undefined : "Nothing running to stop"}
              >
                {controlBusy && jobKind === "stop" ? "Stopping…" : "Stop"}
              </Button>
              <Button
                variant="danger"
                onClick={() => requestControl("down")}
                disabled={busy || !canDown}
                title={canDown ? undefined : "Already down"}
              >
                {controlBusy && jobKind === "down" ? "Removing…" : "Down"}
              </Button>
            </>
          )}
          <Button variant="danger" onClick={handleDeleteClick} disabled={deleting || busy}>
            {deleting ? "Deleting…" : "Delete"}
          </Button>
        </div>
      </div>

      {confirmingDestroy && (
        <div className="flex items-end gap-3">
          <label className={LABEL_CLASS}>
            SSH password for {username}@{host}
            <input
              type="password"
              value={destroyPassword}
              onChange={(e) => setDestroyPassword(e.target.value)}
              className={FIELD_CLASS}
              placeholder={service.hasSavedPassword ? "leave blank to use saved password" : ""}
              autoComplete="off"
              autoFocus
            />
          </label>
          <Button
            variant="danger"
            onClick={() => destroyAndDelete(destroyPassword)}
            disabled={(!destroyPassword && !service.hasSavedPassword) || busy}
          >
            {deleting ? "Working…" : "Confirm delete"}
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setConfirmingDestroy(false);
              setDestroyPassword("");
            }}
          >
            Cancel
          </Button>
        </div>
      )}

      {controlAction && (
        <div className="flex items-end gap-3">
          <label className={LABEL_CLASS}>
            SSH password for {username}@{host}
            <input
              type="password"
              value={controlPassword}
              onChange={(e) => setControlPassword(e.target.value)}
              className={FIELD_CLASS}
              placeholder={service.hasSavedPassword ? "leave blank to use saved password" : ""}
              autoComplete="off"
              autoFocus
            />
          </label>
          <Button
            variant={controlAction === "down" ? "danger" : controlAction === "stop" ? "warning" : "success"}
            onClick={() => runControl(controlAction, controlPassword)}
            disabled={(!controlPassword && !service.hasSavedPassword) || busy}
          >
            {controlBusy
              ? "Working…"
              : controlAction === "start"
                ? "Confirm start"
                : controlAction === "stop"
                  ? "Confirm stop"
                  : "Confirm down"}
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setControlAction(null);
              setControlPassword("");
            }}
          >
            Cancel
          </Button>
        </div>
      )}

      {service.hasSavedPassword && !showSyncPassword && (
        <button
          onClick={() => setShowSyncPassword(true)}
          className="w-fit text-xs text-text-secondary hover:underline"
        >
          Use a different password for sync
        </button>
      )}
      {showSyncPassword && (
        <div className="flex items-end gap-3">
          <label className={LABEL_CLASS}>
            SSH password for {username}@{host}
            <input
              type="password"
              value={syncPassword}
              onChange={(e) => setSyncPassword(e.target.value)}
              className={FIELD_CLASS}
              placeholder={service.hasSavedPassword ? "leave blank to use saved password" : ""}
              autoComplete="off"
              autoFocus
            />
          </label>
          <Button onClick={handleSync} disabled={(!syncPassword && !service.hasSavedPassword) || busy}>
            {syncing ? "Syncing…" : "Confirm sync"}
          </Button>
          <Button
            variant="secondary"
            onClick={() => {
              setShowSyncPassword(false);
              setSyncPassword("");
            }}
          >
            Cancel
          </Button>
        </div>
      )}

      <section className="flex flex-col gap-4">
        <h2 className="text-sm font-semibold text-text-primary">Connection</h2>
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <label className={LABEL_CLASS}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className={FIELD_CLASS} />
          </label>
          <label className={LABEL_CLASS}>
            Host
            <input value={host} onChange={(e) => setHost(e.target.value)} className={FIELD_CLASS} />
          </label>
          <label className={LABEL_CLASS}>
            SSH port
            <input
              value={port}
              onChange={(e) => setPort(e.target.value)}
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
            />
          </label>
          <label className={`${LABEL_CLASS} col-span-2 sm:col-span-4`}>
            Deploy directory
            <input
              value={deployDir}
              onChange={(e) => setDeployDir(e.target.value)}
              className={FIELD_CLASS}
            />
          </label>
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-text-primary">
            {service.type === "kubernetes" ? "Manifest" : "docker-compose.yml"}
          </h2>
          <div className="flex items-center gap-3">
            {justSaved && <span className="text-xs text-status-good">Saved</span>}
            <Button variant="secondary" onClick={handleSave} disabled={busy}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </div>
        </div>
        <YamlEditor value={manifest} onChange={setManifest} readOnly={busy} />
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-text-primary">Deploy</h2>
        {!showPassword ? (
          <div className="flex items-center gap-3">
            <Button
              onClick={() => (service.hasSavedPassword ? handleDeploy() : setShowPassword(true))}
              disabled={busy}
            >
              {deploying ? "Starting…" : service.status === "deployed" ? "Redeploy" : "Deploy"}
            </Button>
            {service.hasSavedPassword && (
              <button
                onClick={() => setShowPassword(true)}
                className="text-xs text-text-secondary hover:underline"
              >
                Use a different password
              </button>
            )}
          </div>
        ) : (
          <div className="flex items-end gap-3">
            <label className={LABEL_CLASS}>
              SSH password for {username}@{host}
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className={FIELD_CLASS}
                placeholder={service.hasSavedPassword ? "leave blank to use saved password" : ""}
                autoComplete="off"
                autoFocus
              />
            </label>
            <Button onClick={handleDeploy} disabled={(!password && !service.hasSavedPassword) || busy}>
              {deploying ? "Starting…" : "Confirm deploy"}
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setShowPassword(false);
                setPassword("");
              }}
            >
              Cancel
            </Button>
          </div>
        )}
      </section>

      {job && (
        <section className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <span className="text-sm text-text-secondary">{JOB_KIND_LABEL[jobKind]}</span>
            <StatusBadge role={JOB_STATUS_ROLE[job.status]} label={job.status} />
          </div>
          <pre
            ref={logRef}
            className="max-h-96 overflow-y-auto whitespace-pre-wrap rounded-lg border border-border bg-surface-1 p-4 font-mono text-xs text-text-secondary"
          >
            {job.log.length > 0 ? job.log.join("") : "Waiting for output…"}
          </pre>
        </section>
      )}
    </div>
  );
}
