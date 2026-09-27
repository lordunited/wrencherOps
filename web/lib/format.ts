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
  degraded: "warning",
  unreachable: "critical",
  unknown: "muted",
};

export function serviceHealthRole(health: string): StatusRole {
  return SERVICE_HEALTH_ROLE[health] ?? "muted";
}

const LOG_STATUS_ROLE: Record<string, StatusRole> = {
  running: "warning",
  success: "good",
  failed: "critical",
  healthy: "good",
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
