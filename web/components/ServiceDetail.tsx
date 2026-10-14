// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";
import { ControlsWarmup } from "@/components/ControlsWarmup";
import { NginxManager } from "@/components/NginxManager";
import { EnvoyManager } from "@/components/EnvoyManager";
import { HAProxyManager } from "@/components/HAProxyManager";
import { PrometheusRules } from "@/components/PrometheusRules";
import { PrometheusStatus } from "@/components/PrometheusStatus";
import { ServiceHistory } from "@/components/ServiceHistory";
import { YamlEditor } from "@/components/YamlEditor";
import type { DeployJob, Service, ServiceStatus } from "@/lib/api";
import { ServiceContainers } from "@/components/ServiceContainers";
import { ServicePorts } from "@/components/ServicePorts";
import { ComposeEnvFiles } from "@/components/ComposeEnvFiles";
import { LocalTime } from "@/components/LocalTime";
import { serviceHealthRole, serviceStatusRole, type StatusRole } from "@/lib/format";

const FIELD_CLASS =
  "field";
const LABEL_CLASS = "flex flex-1 flex-col gap-1.5 text-sm font-medium text-text-secondary";

const JOB_STATUS_ROLE: Record<DeployJob["status"], StatusRole> = {
  running: "warning",
  success: "good",
  failed: "critical",
};

type JobKind = "deploy" | "start" | "stop" | "down" | "destroy" | "removal" | "last";

const JOB_KIND_LABEL: Record<JobKind, string> = {
  deploy: "Deployment log",
  start: "Start log",
  stop: "Stop log",
  down: "Down log",
  destroy: "Delete log",
  // Used when the page is reopened mid-job. "removing" is shared by Down and
  // Delete, so it gets a neutral label; anything already finished is "last".
  removal: "Removal log",
  last: "Last action log",
};

// Statuses that mean a job was in flight when the page was loaded, and the
// log kind to show when picking that job back up.
const RESUME_KIND: Partial<Record<ServiceStatus, JobKind>> = {
  deploying: "deploy",
  starting: "start",
  stopping: "stop",
  removing: "removal",
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

// What the panel does with a connected stack of each type, beyond watching it.
const CONNECTED_SCOPE: Record<string, string> = {
  prometheus: "The panel manages its Prometheus rules and targets only.",
  nginx: "The panel manages its Nginx configuration only.",
  haproxy: "The panel manages its HAProxy configuration only.",
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
  const [jobKind, setJobKind] = useState<JobKind>("deploy");

  const [showSyncPassword, setShowSyncPassword] = useState(false);
  const [syncPassword, setSyncPassword] = useState("");
  const [syncing, setSyncing] = useState(false);

  const [controlAction, setControlAction] = useState<ControlAction | null>(null);
  const [controlPassword, setControlPassword] = useState("");
  const [controlBusy, setControlBusy] = useState(false);

  // Delete asks for approval in the page first: "teardown" deletes the
  // containers and their data (typing the name approves it), "record" only
  // removes the panel's record.
  const [deleteMode, setDeleteMode] = useState<"teardown" | "record" | null>(null);
  const canWrite = useCanWrite();
  const [approval, setApproval] = useState("");
  const [destroyPassword, setDestroyPassword] = useState("");

  const isConnected = service.origin === "connected";
  // A server of a custom compose app: its compose, folder and name are the
  // app's, edited on the app page so every server stays the same.
  const isMember = !!service.appId;
  // Panel-deployed Compose stacks always have Start/Stop/Down. A connected
  // Compose stack shows them too, but they stay locked until the panel has
  // watched it stay healthy for 5 minutes in a row.
  const isCompose = service.type !== "kubernetes" && !isConnected;
  const showControls = isCompose || (isConnected && !!service.composeProject);
  const controlsLocked = isConnected && !service.controlsUnlockedAt;
  const lockedTitle = "Unlocks after the stack has been healthy for 5 minutes in a row";
  const hasPrometheus =
    service.type === "prometheus" || (service.type === "docker" && service.manifest.includes("prom/prometheus"));
  const isNginx = service.type === "nginx";
  const isHAProxy = service.type === "haproxy";
  const isEnvoy = service.type === "envoy";
  type Tab = "service" | "rules" | "status" | "nginx" | "haproxy" | "envoy";
  const tabs: [Tab, string][] = hasPrometheus
    ? [
        ["service", "Service"],
        ["rules", "Rules & targets"],
        ["status", "Status"],
      ]
    : isNginx
      ? [
          ["service", "Service"],
          ["nginx", "Nginx config"],
        ]
      : isHAProxy
        ? [
            ["service", "Service"],
            ["haproxy", "HAProxy config"],
          ]
        : isEnvoy
          ? [
              ["service", "Service"],
              ["envoy", "Envoy"],
            ]
          : [];
  const [tab, setTab] = useState<Tab>(
    isConnected && hasPrometheus ? "rules" : isConnected && isNginx ? "nginx" : isConnected && isHAProxy ? "haproxy" : isConnected && isEnvoy ? "envoy" : "service",
  );
  const tabKeys = tabs.map(([k]) => k).join(",");
  useEffect(() => {
    const fromHash = window.location.hash.slice(1) as Tab;
    if (!tabKeys.split(",").includes(fromHash) || tab === fromHash) return;
    const t = setTimeout(() => setTab(fromHash), 0);
    return () => clearTimeout(t);
  }, [tabKeys, tab]);
  function switchTab(next: Tab) {
    setTab(next);
    history.replaceState(null, "", next === "service" ? window.location.pathname : `#${next}`);
  }

  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const logRef = useRef<HTMLPreElement | null>(null);
  const unmountedRef = useRef(false);

  useEffect(() => {
    unmountedRef.current = false;
    return () => {
      unmountedRef.current = true;
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, []);

  // The job log only lives in this component's state, so navigating away and
  // back would otherwise lose it. On load, pick the service's last job back
  // up: show its log, and keep polling if it's still running. Jobs are kept
  // in the backend's memory, so after a backend restart there's nothing to show.
  const lastJobId = initial.lastJobId;
  const initialStatus = initial.status;
  const serviceId = initial.id;
  useEffect(() => {
    if (!lastJobId) return;
    let cancelled = false;

    // Once a resumed job finishes, the backend records the outcome a moment
    // later (or deletes the record, for Delete). Re-read the service until
    // it settles instead of guessing which action the job was.
    async function settleService() {
      for (let attempt = 0; attempt < 10; attempt++) {
        if (unmountedRef.current) return;
        const res = await fetch(`/api/services/${serviceId}`, { cache: "no-store" });
        if (unmountedRef.current) return;
        if (res.status === 404) {
          router.push(backHref);
          return;
        }
        if (res.ok) {
          const updated: Service = await res.json();
          setService(updated);
          if (!(updated.status in RESUME_KIND)) return;
        }
        await new Promise((resolve) => setTimeout(resolve, 1000));
      }
    }

    (async () => {
      const res = await fetch(`/api/deploy/jobs/${lastJobId}`, { cache: "no-store" });
      if (cancelled || !res.ok) return;
      const snapshot: DeployJob = await res.json();
      if (cancelled) return;

      setJobKind(RESUME_KIND[initialStatus] ?? "last");
      setJob(snapshot);
      if (snapshot.status !== "running") return;

      pollRef.current = setInterval(async () => {
        const pollRes = await fetch(`/api/deploy/jobs/${lastJobId}`, { cache: "no-store" });
        if (pollRes.status === 404) {
          // Backend restarted mid-job; the log is gone for good.
          if (pollRef.current) clearInterval(pollRef.current);
          pollRef.current = null;
          setJob(null);
          return;
        }
        if (!pollRes.ok) return;
        const next: DeployJob = await pollRes.json();
        setJob(next);
        if (next.status === "running") return;
        if (pollRef.current) clearInterval(pollRef.current);
        pollRef.current = null;
        await settleService();
      }, 1500);
    })();

    return () => {
      cancelled = true;
    };
  }, [lastJobId, initialStatus, serviceId, router, backHref]);

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
      setDeleteMode(null);
      setApproval("");
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
    setApproval("");
    setDestroyPassword("");
    setDeleteMode(isConnected || !needsTeardown ? "record" : "teardown");
  }

  const containerNames = (service.healthStats ?? []).map((c) => c.name);
  const approved = approval.trim() === service.name.trim();

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
      // The backend saves a supplied password, so later actions can skip the prompt.
      setService((s) => ({ ...s, status: "deploying", hasSavedPassword: s.hasSavedPassword || password !== "" }));
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
      setService((s) => ({
        ...s,
        status: CONTROL_TRANSIENT_STATUS[action],
        hasSavedPassword: s.hasSavedPassword || suppliedPassword !== "",
      }));
      setJob({ id: jobId, status: "running", log: [], startedAt: "", updatedAt: "" });
      pollJob(jobId, isConnected && action === "start" ? "connected" : CONTROL_SUCCESS_STATUS[action]);
    } finally {
      setControlBusy(false);
    }
  }

  function requestControl(action: ControlAction) {
    if (action === "down") {
      const confirmed = confirm(
        isConnected
          ? `Take down "${service.name}"? This runs "docker compose down" for its Compose project ${service.composeProject} on the server: its containers and network are removed, named volumes and data are kept. Start brings it back.`
          : `Take down "${service.name}"? This runs "docker compose down" — containers and the default network are removed. Named volumes (persisted data) are left intact.`,
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
    <div className="flex flex-col gap-5">
      <Link
        href={backHref}
        className="w-fit rounded-full px-1 text-sm font-semibold text-text-secondary hover:text-text-primary"
      >
        ← {backLabel}
      </Link>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <h1 className="page-title break-words">{service.name}</h1>
          <p className="mt-1 break-all font-mono text-xs text-text-secondary">
            {service.type} · {service.username}@{service.host}:{service.port} · {service.deployDir}
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <StatusBadge role={serviceStatusRole(service.status)} label={service.status} />
            <StatusBadge role={serviceHealthRole(service.health)} label={service.health} />
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="secondary"
            onClick={() => (service.hasSavedPassword ? handleSync() : setShowSyncPassword(true))}
            disabled={busy}
          >
            {syncing ? "Syncing…" : "Sync"}
          </Button>
          {showControls && canWrite && (
            <>
              <Button
                variant="success"
                onClick={() => requestControl("start")}
                disabled={busy || !canStart || controlsLocked}
                title={controlsLocked ? lockedTitle : canStart ? undefined : "Deploy first — nothing to start yet"}
              >
                {controlBusy && jobKind === "start" ? "Starting…" : "Start"}
              </Button>
              <Button
                variant="warning"
                onClick={() => requestControl("stop")}
                disabled={busy || !canStop || controlsLocked}
                title={controlsLocked ? lockedTitle : canStop ? undefined : "Nothing running to stop"}
              >
                {controlBusy && jobKind === "stop" ? "Stopping…" : "Stop"}
              </Button>
              <Button
                variant="danger"
                onClick={() => requestControl("down")}
                disabled={busy || !canDown || controlsLocked}
                title={controlsLocked ? lockedTitle : canDown ? undefined : "Already down"}
              >
                {controlBusy && jobKind === "down" ? "Removing…" : "Down"}
              </Button>
            </>
          )}
          {canWrite && (
            <Button variant="danger" onClick={handleDeleteClick} disabled={deleting || busy}>
              {deleting ? "Removing…" : isConnected ? "Remove from panel" : "Delete"}
            </Button>
          )}
        </div>
      </div>

      {deleteMode === "teardown" && (
        <div className="card flex flex-col gap-3 border-l-4 p-4 sm:p-5" style={{ borderLeftColor: "var(--status-critical)" }} role="alertdialog" aria-labelledby="delete-title">
          <p id="delete-title" className="text-base font-bold text-status-critical">
            ⚠ You are deleting {containerNames.length === 1 ? "a container" : "containers"} and all their data
          </p>
          <p className="text-sm text-text-primary">
            Deleting <b>{service.name}</b> runs <span className="font-mono">docker compose down -v</span> on{" "}
            <span className="font-mono">{host}</span>. It stops and <b>deletes its containers</b>
            {containerNames.length > 0 && (
              <>
                {" "}
                (<span className="font-mono">{containerNames.join(", ")}</span>)
              </>
            )}
            , their networks and <b>volumes — all their data</b>, then deletes the folder{" "}
            <span className="font-mono">{service.deployDir}</span>. This cannot be undone.
          </p>
          <label className="flex max-w-md flex-col gap-1 text-sm text-text-secondary">
            <span>
              To approve, type the service&apos;s name: <b className="font-mono text-text-primary">{service.name}</b>
            </span>
            <input
              value={approval}
              onChange={(e) => setApproval(e.target.value)}
              className="field font-mono"
              autoComplete="off"
              spellCheck={false}
              autoFocus
              aria-label="Type the service name to approve deleting its containers"
            />
          </label>
          {!service.hasSavedPassword && (
            <div className="max-w-md">
              <SshPassword label={`SSH password for ${username}@${host}`} saved={false} value={destroyPassword} onChange={setDestroyPassword} />
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            <Button
              variant="danger"
              onClick={() => destroyAndDelete(destroyPassword)}
              disabled={!approved || (!destroyPassword && !service.hasSavedPassword) || busy || deleting}
            >
              {deleting ? "Deleting…" : "Delete containers and data"}
            </Button>
            <Button variant="secondary" onClick={removeRecordOnly} disabled={busy || deleting} title="Keeps the containers, volumes and files on the server">
              Remove from panel only
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setDeleteMode(null);
                setApproval("");
                setDestroyPassword("");
              }}
            >
              Cancel
            </Button>
          </div>
          <p className="text-xs text-text-muted">
            Only want it gone from this list? <b>Remove from panel only</b> deletes nothing on the server.
          </p>
        </div>
      )}

      {deleteMode === "record" && (
        <div className="card flex flex-col gap-3 border-l-4 p-4 sm:p-5" style={{ borderLeftColor: "var(--status-warning)" }} role="alertdialog" aria-labelledby="remove-title">
          <p id="remove-title" className="text-base font-bold text-text-primary">
            Remove {service.name} from the panel?
          </p>
          <p className="text-sm text-text-primary">
            This only removes the panel&apos;s record. <b>No container is deleted</b>:{" "}
            {isConnected
              ? `its containers, config and data on ${host} stay exactly as they are and keep running.`
              : isCompose
                ? "nothing has been deployed for it yet, so there's nothing on the server to delete."
                : "nothing already applied to the cluster is touched."}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="danger" onClick={removeRecordOnly} disabled={busy || deleting}>
              {deleting ? "Removing…" : "Yes, remove from the panel"}
            </Button>
            <Button variant="secondary" onClick={() => setDeleteMode(null)}>
              Cancel
            </Button>
          </div>
        </div>
      )}

      {controlAction && (
        <div className="card flex flex-col gap-3 p-4">
          <p className="text-sm text-text-primary">
            {controlAction === "down" ? (
              <>
                <b className="text-status-critical">⚠ Down deletes the containers</b> of {service.name} on {host} (
                <span className="font-mono">docker compose down</span>). Volumes and data are kept, and Start or Deploy
                creates the containers again.
              </>
            ) : controlAction === "stop" ? (
              <>
                Stop stops the containers of {service.name} on {host}; nothing is deleted.
              </>
            ) : (
              <>
                Start starts the containers of {service.name} on {host}.
              </>
            )}
          </p>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <SshPassword
              label={`SSH password for ${username}@${host}`}
              saved={service.hasSavedPassword}
              value={controlPassword}
              onChange={setControlPassword}
            />
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
        </div>
      )}
      {showControls && controlsLocked && <ControlsWarmup service={service} onUpdate={setService} />}

      {tabs.length > 0 && (
        <div className="inline-flex w-fit gap-1 rounded-full bg-surface-1 p-1 shadow-card" role="tablist" aria-label="Service sections">
          {tabs.map(([key, label]) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={tab === key}
              onClick={() => switchTab(key)}
              className={`rounded-full px-4 py-1.5 text-sm font-semibold transition-colors ${
                tab === key ? "bg-accent text-on-accent" : "text-text-secondary hover:text-text-primary"
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      )}

      {tab === "rules" && hasPrometheus ? (
        <PrometheusRules
          serviceId={service.id}
          hasSavedPassword={service.hasSavedPassword}
          sshLabel={`${service.username}@${service.host}`}
        />
      ) : tab === "status" && hasPrometheus ? (
        <PrometheusStatus serviceId={service.id} />
      ) : tab === "nginx" && isNginx ? (
        <NginxManager serviceId={service.id} hasSavedPassword={service.hasSavedPassword} sshLabel={`${service.username}@${service.host}`} />
      ) : tab === "envoy" && isEnvoy ? (
        <EnvoyManager serviceId={service.id} hasSavedPassword={service.hasSavedPassword} sshLabel={`${service.username}@${service.host}`} />
      ) : tab === "haproxy" && isHAProxy ? (
        <HAProxyManager serviceId={service.id} hasSavedPassword={service.hasSavedPassword} sshLabel={`${service.username}@${service.host}`} />
      ) : (
      <>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <div className="col-span-2 sm:col-span-1">
          <StatTile
            hero
            label="Health"
            value={service.health.charAt(0).toUpperCase() + service.health.slice(1)}
            hint={service.healthDetail || "Click Sync to check it now"}
          />
        </div>
        <StatTile
          label="Last sync"
          value={<LocalTime value={service.lastSyncedAt} style="time" empty="Never" />}
          hint={<LocalTime value={service.lastSyncedAt} style="date" empty="Not checked yet" />}
        />
        <StatTile
          label="Status"
          value={service.status.charAt(0).toUpperCase() + service.status.slice(1)}
          hint={
            <>
              Updated <LocalTime value={service.updatedAt} style="date" />
            </>
          }
        />
      </div>

      <ServicePorts ports={service.healthPorts} host={service.host} checkedAt={service.lastSyncedAt} />
      <ServiceContainers containers={service.healthContainers} stats={service.healthStats} />
      <ComposeEnvFiles service={service} />


      {service.hasSavedPassword && !showSyncPassword && (
        <button
          onClick={() => setShowSyncPassword(true)}
          className="w-fit text-xs text-text-secondary hover:underline"
        >
          Use a different password for sync
        </button>
      )}
      {showSyncPassword && (
        <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
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

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <div className="flex items-center justify-between gap-3">
          <h2 className="section-title">Connection</h2>
          {(isConnected || isMember) && canWrite && (
            <div className="flex items-center gap-3">
              {justSaved && <span className="text-xs text-status-good">Saved</span>}
              <Button variant="secondary" onClick={handleSave} disabled={busy}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          )}
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <label className={LABEL_CLASS}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className={FIELD_CLASS} readOnly={isMember} />
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
          <label className={`${LABEL_CLASS} sm:col-span-2 lg:col-span-4`}>
            {isConnected ? "Compose folder (on the server)" : "Deploy directory"}
            <input
              value={deployDir}
              onChange={(e) => setDeployDir(e.target.value)}
              className={FIELD_CLASS}
              readOnly={isConnected || isMember}
            />
          </label>
          {service.type === "signoz" && (
            <p className="text-xs text-text-muted sm:col-span-2 lg:col-span-4">
              SigNoz UI:{" "}
              <a href={`http://${service.host.trim()}:8080`} target="_blank" rel="noreferrer" className="font-mono font-semibold text-accent hover:underline">
                http://{service.host.trim()}:8080
              </a>{" "}
              (the default port; first start takes a few minutes). Send telemetry over OTLP to port 4317 (gRPC) or 4318
              (HTTP) on the same host.
            </p>
          )}
          {isMember && (
            <p className="text-xs text-text-muted sm:col-span-2 lg:col-span-4">
              This server is part of{" "}
              <Link href={backHref} className="font-semibold text-accent hover:underline">
                {backLabel}
              </Link>
              . Its compose file, folder and name are the app&apos;s — edit them there and every server gets the change.
            </p>
          )}
          {isConnected && (
            <p className="text-xs text-text-muted sm:col-span-2 lg:col-span-4">
              {service.container ? (
                <>
                  Connected to container <span className="font-mono">{service.container}</span>
                  {service.composeProject ? " in " : ""}
                </>
              ) : (
                "Watching "
              )}
              {service.composeProject ? (
                <>
                  Compose project <span className="font-mono">{service.composeProject}</span>
                </>
              ) : null}
              . {CONNECTED_SCOPE[service.type] ?? "The panel only watches it; it never deploys, edits or removes it."}
            </p>
          )}
        </div>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex items-center justify-between gap-3">
          <h2 className="section-title">
            {isConnected
              ? "Compose file (read-only copy from the server)"
              : isMember
                ? "Compose file (from the app)"
                : service.type === "kubernetes"
                  ? "Manifest"
                  : service.type === "signoz"
                    ? "casting.yaml (SigNoz installer)"
                    : "docker-compose.yml"}
          </h2>
          {!isConnected && !isMember && canWrite && (
            <div className="flex items-center gap-3">
              {justSaved && <span className="text-xs text-status-good">Saved</span>}
              <Button variant="secondary" onClick={handleSave} disabled={busy}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          )}
        </div>
        <YamlEditor value={manifest} onChange={setManifest} readOnly={busy || isConnected || isMember || !canWrite} />
      </section>

      {!isConnected && canWrite && (
      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Deploy</h2>
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
          <div className="card flex flex-col gap-3 p-4 sm:flex-row sm:items-end">
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
      )}

      {job && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <div className="flex items-center gap-3">
            <span className="section-title">{JOB_KIND_LABEL[jobKind]}</span>
            <StatusBadge role={JOB_STATUS_ROLE[job.status]} label={job.status} />
          </div>
          <pre
            ref={logRef}
            className="log-panel max-h-96 overflow-y-auto whitespace-pre-wrap p-4"
          >
            {job.log.length > 0 ? job.log.join("") : "Waiting for output…"}
          </pre>
        </section>
      )}

      <ServiceHistory
        serviceId={service.id}
        host={service.host}
        refreshKey={`${job?.id ?? ""}:${job?.status ?? ""}:${service.status}:${service.lastSyncedAt ?? ""}`}
      />
      </>
      )}
    </div>
  );
}

// SshPassword asks for the SSH password only when none is saved for this
// server; with one saved it just says so, with a way to type another.
function SshPassword({
  label,
  saved,
  value,
  onChange,
}: {
  label: string;
  saved: boolean;
  value: string;
  onChange: (v: string) => void;
}) {
  const [typing, setTyping] = useState(false);
  if (saved && !typing) {
    return (
      <div className={LABEL_CLASS}>
        {label}
        <span className="flex h-[38px] items-center gap-2 text-sm text-status-good">
          Saved ✓
          <button type="button" onClick={() => setTyping(true)} className="text-xs font-semibold text-accent hover:underline">
            use another
          </button>
        </span>
      </div>
    );
  }
  return (
    <label className={LABEL_CLASS}>
      {label}
      <input
        type="password"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={FIELD_CLASS}
        autoComplete="off"
        autoFocus
      />
    </label>
  );
}
