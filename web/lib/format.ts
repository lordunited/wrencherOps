// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

export function compactNumber(value: number): string {
  return new Intl.NumberFormat("en-US", { notation: "compact" }).format(value);
}

export type StatusRole = "good" | "warning" | "serious" | "critical" | "muted";

const PIPELINE_STATUS_ROLE: Record<string, StatusRole> = {
  success: "good",
  running: "warning",
  pending: "warning",
  created: "warning",
  failed: "critical",
  canceled: "muted",
  canceling: "muted",
  skipped: "muted",
  manual: "muted",
  scheduled: "muted",
};

const RUNNER_STATUS_ROLE: Record<string, StatusRole> = {
  online: "good",
  offline: "muted",
  stale: "muted",
  never_contacted: "muted",
};

const USER_STATE_ROLE: Record<string, StatusRole> = {
  active: "good",
  blocked: "critical",
  deactivated: "muted",
};

export function pipelineStatusRole(status: string): StatusRole {
  return PIPELINE_STATUS_ROLE[status] ?? "muted";
}

export function runnerStatusRole(status: string): StatusRole {
  return RUNNER_STATUS_ROLE[status] ?? "muted";
}

export function userStateRole(state: string): StatusRole {
  return USER_STATE_ROLE[state] ?? "muted";
}

export function mergeRequestRole(draft: boolean): StatusRole {
  return draft ? "warning" : "good";
}

const ACCESS_LEVEL_LABEL: Record<number, string> = {
  5: "Minimal access",
  10: "Guest",
  15: "Planner",
  20: "Reporter",
  30: "Developer",
  40: "Maintainer",
  50: "Owner",
};

export function accessLevelLabel(level: number): string {
  return ACCESS_LEVEL_LABEL[level] ?? `Level ${level}`;
}

const SERVICE_STATUS_ROLE: Record<string, StatusRole> = {
  pending: "muted",
  deploying: "warning",
  deployed: "good",
  starting: "warning",
  stopping: "warning",
  stopped: "warning",
  removing: "warning",
  down: "muted",
  failed: "critical",
  connected: "good",
};

export function serviceStatusRole(status: string): StatusRole {
  return SERVICE_STATUS_ROLE[status] ?? "muted";
}

const SERVICE_HEALTH_ROLE: Record<string, StatusRole> = {
  healthy: "good",
  starting: "warning",
  degraded: "warning",
  unreachable: "critical",
  unknown: "muted",
  "not deployed": "muted",
  down: "muted",
  paused: "warning",
  stopped: "muted",
};

export function serviceHealthRole(health: string): StatusRole {
  return SERVICE_HEALTH_ROLE[health] ?? "muted";
}

const LOG_STATUS_ROLE: Record<string, StatusRole> = {
  running: "warning",
  success: "good",
  failed: "critical",
  healthy: "good",
  starting: "warning",
  degraded: "warning",
  unreachable: "critical",
};

export function logStatusRole(status: string): StatusRole {
  return LOG_STATUS_ROLE[status] ?? "muted";
}

// Human-readable duration between two ISO timestamps, e.g. "4s" or "2m 13s".
export function formatDuration(startIso: string, endIso?: string): string {
  if (!endIso) return "";
  const seconds = Math.max(0, Math.round((Date.parse(endIso) - Date.parse(startIso)) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`;
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
}

export function bytesLabel(b: number): string {
  if (!Number.isFinite(b) || b < 1024) return `${Math.max(0, Math.round(b || 0))} B`;
  const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
  let v = b / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1).replace(/\.0$/, "")} ${units[i]}`;
}

// TimeStyle is how LocalTime shows a moment.
export type TimeStyle = "datetime" | "date" | "time" | "ago";

const pad = (n: number) => String(n).padStart(2, "0");

// formatUTC is a moment as the same text everywhere — server, browser, any
// locale or time zone — for HTML that must hydrate unchanged.
export function formatUTC(d: Date, style: TimeStyle): string {
  const date = `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
  const time = `${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
  if (style === "date") return date;
  if (style === "time") return `${time} UTC`;
  return `${date} ${time} UTC`;
}

// localTime is a moment in the viewer's locale and time zone, or relative
// to now; only for the browser, after hydration (LocalTime).
export function localTime(d: Date, style: TimeStyle): string {
  if (style === "ago") return timeAgo(d.toISOString());
  if (style === "date") return d.toLocaleDateString();
  if (style === "time") return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return d.toLocaleString();
}

export function timeAgo(iso?: string): string {
  if (!iso) return "never";
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return `${Math.round(s / 86400)} d ago`;
}

export function uptimeLabel(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return d > 0 ? `${d}d ${h}h` : h > 0 ? `${h}h ${m}m` : `${m}m`;
}

// usageRole colors a percentage: fine, getting full, nearly full.
export function usageRole(percent: number, warn = 85, crit = 95): StatusRole {
  return percent >= crit ? "critical" : percent >= warn ? "warning" : "good";
}
