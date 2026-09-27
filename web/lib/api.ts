import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";

export const SESSION_COOKIE_NAME = "session";

export interface Project {
  id: number;
  name: string;
  path_with_namespace: string;
  description: string | null;
  visibility: string;
  default_branch: string;
  web_url: string;
  created_at: string;
  last_activity_at: string;
  open_issues_count: number;
  star_count: number;
  forks_count: number;
}

export interface Runner {
  id: number;
  description: string;
  status: string;
  online: boolean;
  is_shared: boolean;
  runner_type: string;
}

export interface GitLabUser {
  id: number;
  username: string;
  name: string;
  state: string;
  web_url: string;
}

export interface PipelineInfo {
  id: number;
  status: string;
  ref: string;
  created_at: string;
  web_url: string;
}

export interface ProjectPipelines {
  projectId: number;
  projectPath: string;
  totalCount: number;
  pipelines: PipelineInfo[];
}

export interface BasicUser {
  username: string;
  name: string;
}

export interface MergeRequestInfo {
  iid: number;
  title: string;
  source_branch: string;
  target_branch: string;
  draft: boolean;
  author: BasicUser | null;
  created_at: string;
  web_url: string;
}

export interface ProjectMergeRequests {
  projectId: number;
  projectPath: string;
  totalCount: number;
  mergeRequests: MergeRequestInfo[];
}

export interface ProjectMember {
  username: string;
  name: string;
  access_level: number;
  state: string;
  web_url: string;
}

const API_BASE_URL = process.env.API_BASE_URL ?? "http://localhost:8080";

async function fetchJSON<T>(path: string): Promise<T> {
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get(SESSION_COOKIE_NAME);

  // no-store: this request carries the caller's session cookie, and Next's
  // fetch cache key isn't guaranteed to vary by header — caching a response
  // here risks serving one user's authenticated data to another request.
  // The backend's own TTL cache still provides the actual speed-up.
  const res = await fetch(`${API_BASE_URL}${path}`, {
    cache: "no-store",
    headers: sessionCookie
      ? { Cookie: `${SESSION_COOKIE_NAME}=${sessionCookie.value}` }
      : undefined,
  });

  if (res.status === 401) {
    redirect("/login");
  }
  if (res.status === 404) {
    // e.g. a service that was deleted while its page was still open.
    notFound();
  }
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`${path} failed: ${res.status} ${body}`);
  }
  return res.json();
}

export function getProjects() {
  return fetchJSON<Project[]>("/api/gitlab/projects");
}

export function getRunners() {
  return fetchJSON<Runner[]>("/api/gitlab/runners");
}

export function getUsers() {
  return fetchJSON<GitLabUser[]>("/api/gitlab/users");
}

export function getPipelines() {
  return fetchJSON<ProjectPipelines[]>("/api/gitlab/pipelines");
}

export function getMergeRequests() {
  return fetchJSON<ProjectMergeRequests[]>("/api/gitlab/merge-requests");
}

export function getProject(id: number) {
  return fetchJSON<Project>(`/api/gitlab/projects/${id}`);
}

export function getProjectPipelines(id: number) {
  return fetchJSON<PipelineInfo[]>(`/api/gitlab/projects/${id}/pipelines`);
}

export function getProjectMergeRequests(id: number) {
  return fetchJSON<MergeRequestInfo[]>(`/api/gitlab/projects/${id}/merge-requests`);
}

export function getProjectMembers(id: number) {
  return fetchJSON<ProjectMember[]>(`/api/gitlab/projects/${id}/members`);
}

export interface HealthEvent {
  kind: string;
  message: string;
  timestamp: string;
}

export interface FailingProject {
  projectPath: string;
  consecutiveFailures: number;
  lastPipelineWebUrl: string;
}

export interface StaleMergeRequest {
  projectPath: string;
  title: string;
  webUrl: string;
  ageDays: number;
  author: string;
}

export interface Health {
  runnerEvents: HealthEvent[];
  failingProjects: FailingProject[];
  staleMergeRequests: StaleMergeRequest[];
}

export function getHealth() {
  return fetchJSON<Health>("/api/gitlab/health");
}

export interface Me {
  username: string;
}

export function getMe() {
  return fetchJSON<Me>("/api/auth/me");
}

export type ServiceStatus =
  | "pending"
  | "deploying"
  | "deployed"
  | "starting"
  | "stopping"
  | "stopped"
  | "removing"
  | "down"
  | "failed"
  | "connected";

export interface Service {
  id: string;
  name: string;
  type: string;
  host: string;
  port: number;
  username: string;
  deployDir: string;
  manifest: string;
  status: ServiceStatus;
  lastJobId?: string;
  health: string;
  healthDetail: string;
  lastSyncedAt?: string;
  hasSavedPassword: boolean;
  // "connected" services are existing stacks the panel attached to: it
  // configures their Prometheus but never deploys, stops or removes them.
  origin: "panel" | "connected";
  container: string;
  composeProject: string;
  composeFiles: string;
  // When the current unbroken run of healthy checks began.
  healthySince?: string;
  // When a connected stack first stayed healthy for 5 minutes, which
  // unlocks its Start/Stop/Down.
  controlsUnlockedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export function getServices() {
  return fetchJSON<Service[]>("/api/services");
}

export function getService(id: string) {
  return fetchJSON<Service>(`/api/services/${id}`);
}

export interface DeployJob {
  id: string;
  status: "running" | "success" | "failed";
  log: string[];
  startedAt: string;
  updatedAt: string;
}

// One saved action against a service. For jobs, status is running, success
// or failed. For a sync it is the health result: healthy, degraded or
// unreachable.
export interface ServiceLogEntry {
  id: string;
  serviceId: string;
  serviceName: string;
  host: string;
  action: "deploy" | "start" | "stop" | "down" | "destroy" | "sync" | "rules" | "move-config" | "connect" | "unlock" | "nginx";
  status: string;
  log?: string;
  startedAt: string;
  finishedAt?: string;
}

// ---- Prometheus rule management ----

export interface PromFile {
  path: string;
  hostPath?: string;
  content: string;
  sha: string;
}

export interface PromTarget {
  instance: string;
  note?: string;
  scrapeUrl: string;
  health: "up" | "down" | "unknown";
  lastError?: string;
  lastScrape?: string;
}

export interface PromJob {
  name: string;
  kind: "exporter" | "blackbox";
  module?: string;
  exporter?: string;
  targets: PromTarget[];
}

export interface PromRule {
  name: string;
  type: "alerting" | "recording";
  query: string;
  duration: number;
  health: string;
  state?: "firing" | "pending" | "inactive";
  lastError?: string;
  active: number;
}

export interface PromRuleGroup {
  file: string;
  name: string;
  rules: PromRule[];
}

export interface PromReport {
  container: string;
  image: string;
  version: string;
  configPath: string;
  onDisk: boolean;
  canMoveToDisk: boolean;
  reloadOk: boolean;
  lastReload: string;
  config: PromFile;
  ruleFiles: PromFile[];
  rulePatterns: string[] | null;
  jobs: PromJob[];
  ruleGroups: PromRuleGroup[];
  targetsUp: number;
  targetsDown: number;
  alertsFiring: number;
  alertsPending: number;
}

export interface PromChange {
  path: string;
  before: string;
  after: string;
  baseSha: string;
  isNew: boolean;
}

export interface PromPreview {
  changes: PromChange[];
  skipped?: string[];
  summary: string;
}

export interface PromApplyResult {
  ok: boolean;
  log: string;
  error?: string;
}

export interface PromCandidate {
  container: string;
  image: string;
  state: string;
  composeProject?: string;
  composeService?: string;
  workingDir?: string;
  composeFiles?: string[];
  configPath: string;
  configHostPath?: string;
  port: string;
  routePrefix?: string;
  webConfig: boolean;
}

export interface PromFinding {
  severity: "critical" | "warning";
  title: string;
  detail: string;
}

export interface PromNameValue {
  name: string;
  value: number;
}

export interface PromServerStatus {
  build: {
    version: string;
    revision: string;
    branch: string;
    buildUser: string;
    buildDate: string;
    goVersion: string;
  };
  runtime: {
    startTime: string;
    CWD: string;
    hostname?: string;
    reloadConfigSuccess: boolean;
    lastConfigTime: string;
    corruptionCount: number;
    goroutineCount: number;
    GOMAXPROCS: number;
    GOMEMLIMIT?: number;
    GOGC: string;
    storageRetention: string;
  };
  flags: Record<string, string>;
  tsdb: {
    headStats: { numSeries: number; numLabelPairs: number; chunkCount: number; minTime: number; maxTime: number };
    seriesCountByMetricName: PromNameValue[];
    labelValueCountByLabelName: PromNameValue[];
    memoryInBytesByLabelName: PromNameValue[];
    seriesCountByLabelValuePair: PromNameValue[];
  } | null;
  walReplay: { min: number; max: number; current: number } | null;
  alertmanagers: { activeAlertmanagers: { url: string }[]; droppedAlertmanagers: { url: string }[] } | null;
  metrics: Record<string, number>;
  disk: {
    dataPath: string;
    dataBytes: number;
    walBytes: number;
    walSegments: number;
    blocks: number;
    fsSizeBytes: number;
    fsFreeBytes: number;
    error?: string;
  };
  attention: PromFinding[];
}


// ---- Nginx configuration management ----

export interface NginxLocation {
  match: string;
  proxyPass?: string;
  return?: string;
  root?: string;
  alias?: string;
  tryFiles?: string;
}

export interface NginxServer {
  file: string;
  line: number;
  context: "http" | "stream";
  names: string[];
  listen: string[];
  defaultServer: boolean;
  ssl: boolean;
  certificate?: string;
  return?: string;
  proxyPass?: string;
  locations: NginxLocation[];
}

export interface NginxUpstream {
  file: string;
  line: number;
  context: string;
  name: string;
  method: string;
  servers: string[];
  keepalive?: string;
}

export interface NginxReport {
  container: string;
  image: string;
  version: string;
  testOk: boolean;
  testOutput: string;
  files: PromFile[];
  overview: {
    globals: {
      workerProcesses?: string;
      workerConnections?: string;
      clientMaxBodySize?: string;
      keepaliveTimeout?: string;
      serverTokens?: string;
      gzip?: string;
      sendfile?: string;
      errorLog?: string;
      logFormats?: string[];
      includes?: string[];
    };
    servers: NginxServer[];
    upstreams: NginxUpstream[];
    parseErrors?: string[];
  };
  newFileDir: string;
  newFileDirOnDisk: boolean;
  recentErrors: string[];
  stats: { cpu: string; memory: string; memShare: string; netIO: string; pids: string };
}

export interface NginxCandidate {
  container: string;
  image: string;
  state: string;
  composeProject?: string;
  workingDir?: string;
  composeFiles?: string[];
  configPath: string;
  configHostPath?: string;
  confDHostPath?: string;
}
