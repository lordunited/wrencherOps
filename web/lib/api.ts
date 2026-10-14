// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { cookies } from "next/headers";
import { forbidden, notFound, redirect } from "next/navigation";
import { cache } from "react";
import type { TabAccess } from "@/lib/nav";

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
  if (res.status === 403) {
    // Signed in, but without access to this tab: the no-access page.
    forbidden();
  }
  if (res.status === 404) {
    // e.g. a service that was deleted while its page was still open.
    notFound();
  }
  if (!res.ok) {
    // The backend's errors are written for people (e.g. why GitLab can't be
    // reached): shown as they are.
    const body = (await res.text()).trim();
    throw new Error(body && body.length < 600 && !body.startsWith("<") ? body : `${path} failed: ${res.status}`);
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

export type Role = "admin" | "writer" | "viewer";

// Me is the signed-in user and what they can do: their level in each tab
// they can see (tabs missing are "none").
export interface Me {
  username: string;
  role: Role;
  builtin: boolean;
  tabs: TabAccess;
}

// getMe is read once per request, however many components ask.
export const getMe = cache(() => fetchJSON<Me>("/api/auth/me"));

// --- user management (admins only)

export interface PanelTab {
  key: string;
  label: string;
  category: string;
  href: string;
  viewOnly?: boolean;
  adminOnly?: boolean;
  everyone?: boolean;
}

export interface PanelUser {
  username: string;
  role: Role;
  disabled: boolean;
  builtin?: boolean;
  groups: string[];
  createdAt: string;
  updatedAt: string;
}

export interface PanelUserDetail extends PanelUser {
  grants: TabAccess;
  effective: TabAccess;
}

export interface PanelGroup {
  id: string;
  name: string;
  description: string;
  members: string[];
  createdAt: string;
  grants: TabAccess;
}

export function getPanelTabs() {
  return fetchJSON<PanelTab[]>("/api/admin/tabs");
}

export function getPanelUsers() {
  return fetchJSON<PanelUser[]>("/api/admin/users");
}

export function getPanelUser(username: string) {
  return fetchJSON<PanelUserDetail>(`/api/admin/users/${encodeURIComponent(username)}`);
}

export function getPanelGroups() {
  return fetchJSON<PanelGroup[]>("/api/admin/groups");
}

export function getPanelGroup(id: string) {
  return fetchJSON<PanelGroup>(`/api/admin/groups/${encodeURIComponent(id)}`);
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

export interface ServiceContainerStats {
  name: string;
  cpu: string;
  memory: string; // e.g. "128MiB / 512MiB"
  memPercent: string;
  netIO: string;
  pids: string;
}

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
  // Every currently-running container's resource use from the last check
  // (empty until the first one, and always empty for Kubernetes services).
  healthStats?: ServiceContainerStats[];
  // Every port the stack published at the last check, tried from the panel.
  healthPorts?: PortStatus[];
  // Every container of the stack at the last check, paused and stopped ones included.
  healthContainers?: ContainerState[];
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
  // The custom compose app this server belongs to; its compose and deploy
  // folder are edited on the app, not here.
  appId?: string;
  createdAt: string;
  updatedAt: string;
}

export function getServices() {
  return fetchJSON<Service[]>("/api/services");
}

export function getService(id: string) {
  return fetchJSON<Service>(`/api/services/${id}`);
}

// A custom compose app: one docker-compose.yml deployed identically to
// several servers. Each server is an ordinary Service whose appId is the
// app's; the app owns the compose and deploy folder they share.
export interface ProjectFile {
  path: string; // relative to the deploy folder
  content: string;
}

export interface ComposeMount {
  service: string;
  kind: "bind" | "volume" | "anonymous" | "tmpfs";
  source?: string;
  target: string;
  hostPath?: string; // bind: where on the server
  volume?: string; // volume: the Docker volume's real name
  readOnly?: boolean;
  inProject?: boolean; // bind: provided by the project's files
  external?: boolean;
}

export interface ComposeAnalysis {
  project: string;
  mounts: ComposeMount[];
  warnings: string[];
  parseError?: string;
}

export interface ComposeApp {
  id: string;
  name: string;
  manifest: string;
  composeFile: string; // docker-compose.yml, or an imported project's own name
  template?: string; // "signoz" when created from that template
  env: string;
  files: ProjectFile[];
  deployDir: string;
  createdAt: string;
  updatedAt: string;
  servers: Service[];
  analysis?: ComposeAnalysis;
}

// A compose project found on a server, and what can safely be done with it.
export interface DetectedProject {
  name: string;
  workingDir: string;
  configFiles: string[];
  status?: string; // e.g. "running(2)"; absent for a compose file Docker has no containers for
  importable: boolean;
  importProblem?: string;
  watchable: boolean;
  watchProblem?: string;
  serviceId?: string;
  serviceName?: string;
  serviceType?: string;
  appId?: string;
  // The .env compose reads for ${VARIABLES}, then every env_file its services load.
  envFiles?: EnvFile[];
}

// EnvSync is how one server's .env compares with its app's (GET
// /api/apps/{id}/env/sync).
export interface EnvSync {
  serviceId: string;
  host: string;
  path: string;
  status: "in-sync" | "differs" | "not-on-server" | "only-on-server" | "unreachable";
  serverExists: boolean;
  serverEnv?: string;
  serverSha?: string;
  error?: string;
  checkedAt: string;
}

export interface EnvFile {
  path: string; // absolute, on the server
  label: string; // as the compose writes it, or ".env"
  exists: boolean;
}

// A ready-to-create app: compose, .env and files, filled into the new-app form.
export interface ComposeTemplate {
  template: string;
  name: string;
  deployDir: string;
  composeFile: string;
  manifest: string;
  env: string;
  files: ProjectFile[];
}

export function getSigNozTemplate() {
  return fetchJSON<ComposeTemplate>("/api/apps/templates/signoz");
}

export interface ContainerState {
  name: string;
  service: string;
  state: string; // Docker's: running, paused, exited, restarting, created, dead
  status: string; // as docker ps shows it
  image?: string; // the image it runs, e.g. "nginx:1.27-alpine"
  role: "running" | "starting" | "unhealthy" | "paused" | "stopped" | "job" | "failing";
  note?: string;
}

export interface PortStatus {
  service: string;
  container: string;
  hostIp: string; // "" for every address
  hostPort: string;
  containerPort: string;
  protocol: "tcp" | "udp" | "sctp";
  // open: the panel connected · closed: running, but no connection · local:
  // loopback only · published: UDP/SCTP, not checkable · stopped: container down
  state: "open" | "closed" | "local" | "published" | "stopped";
  detail?: string;
}

export function getApps() {
  return fetchJSON<ComposeApp[]>("/api/apps");
}

export function getApp(id: string) {
  return fetchJSON<ComposeApp>(`/api/apps/${id}`);
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

export interface NginxHeader {
  name: string;
  value: string;
  always: boolean;
}

export interface NginxLocation {
  match: string;
  proxyPass?: string;
  return?: string;
  root?: string;
  alias?: string;
  tryFiles?: string;
  headers?: NginxHeader[];
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
  headers?: NginxHeader[];
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
    headerWarnings?: string[];
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

export interface HAProxyCandidate {
  container: string;
  image: string;
  state: string;
  composeProject?: string;
  workingDir?: string;
  composeFiles?: string[];
  configPaths: string[];
}

export interface GitLabCandidate {
  container: string;
  image: string;
  state: string;
  composeProject?: string;
  workingDir?: string;
  composeFiles?: string[];
  configHostPath?: string; // /etc/gitlab on the server, when bind-mounted
  dataHostPath?: string; // /var/opt/gitlab on the server, when bind-mounted
}

// ---- HAProxy configuration management ----

export interface HAProxyRoute {
  backend: string;
  hosts?: string[]; // Host header values a host rule matches
  condition?: string; // the raw if/unless clause
  default?: boolean; // the frontend's default_backend
}

export interface HAProxySetting {
  name: string;
  value: string;
}

export interface HAProxySection {
  kind: string;
  name?: string;
  file?: string;
  line?: number;
  binds?: string[];
  tls?: boolean;
  certificates?: string[];
  mode?: string; // effective: its own, else inherited from defaults, else tcp
  balance?: string;
  servers?: string[];
  defaultBackend?: string;
  useBackends?: string[];
  routes?: HAProxyRoute[];
  monitorUri?: string;
  statsEnabled?: boolean;
  statsUri?: string;
  healthCheck?: string;
  redirect?: string;
  settings?: HAProxySetting[]; // global and defaults sections only
}

export interface HAProxyProbe {
  kind: "monitor-uri" | "stats" | "port" | "none";
  port?: number;
  path?: string;
  section?: string;
}

export interface HAProxyReport {
  container: string;
  image: string;
  version: string;
  configPaths: string[];
  testOk: boolean;
  testOutput: string;
  files: PromFile[];
  sections: HAProxySection[];
  probe: HAProxyProbe;
  healthy: boolean;
  healthDetail: string;
  recentErrors: string[];
  stats: { cpu: string; memory: string; memShare: string; netIO: string; pids: string };
  // Whether the config enables a stats page at all — backendServers is
  // always empty without one, since HAProxy's own stats page is the only
  // place live per-server UP/DOWN status exists.
  hasStatsPage: boolean;
  backendServers: HAProxyBackendServer[];
}

export interface HAProxyBackendServer {
  backend: string;
  name: string;
  status: string; // "UP", "DOWN", "MAINT", "DRAIN", "NOLB", "no check", ...
  weight: string;
  checkStatus?: string;
  checkDesc?: string;
  lastCheck?: string; // detail on the last check, e.g. a connection error
  lastChangeSeconds?: string;
  downtimeSeconds?: string;
}

export interface HAProxyChange {
  path: string;
  content: string;
  baseSha: string;
  delete: boolean;
}

export interface HAProxyTestResult {
  ok: boolean;
  output: string;
}

// --- Servers: the servers themselves, and alerts on them.

export interface ServerCount {
  key: string;
  count: number;
}

export interface ServerAuthEvent {
  time: string;
  user: string;
  from: string;
  method?: string;
  invalid?: boolean;
  panel?: boolean; // the panel's own login (its checks)
}

export interface ServerDisk {
  filesystem: string;
  mount: string;
  sizeBytes: number;
  usedBytes: number;
  availBytes: number;
  usedPercent: number;
  inodesUsedPercent: number; // -1 when not reported
}

export interface ServerSnapshot {
  collectedAt: string;
  hostname: string;
  os: string;
  kernel: string;
  cpus: number;
  load1: number;
  load5: number;
  load15: number;
  uptimeSeconds: number;
  memory: { totalBytes: number; availableBytes: number; usedPercent: number; swapTotalBytes: number; swapUsedBytes: number };
  disks: ServerDisk[] | null;
  sessions: { user: string; tty: string; since: string; from: string }[] | null;
  logins: ServerAuthEvent[] | null;
  panelAddress?: string;
  failed: { lastHour: number; last24h: number; topUsers: ServerCount[] | null; topSources: ServerCount[] | null; recent: ServerAuthEvent[] | null };
  authSource: string;
  kernelErrors: { lastHour: number; total: number; recent: { time?: string; message: string }[] | null; readable: boolean };
  connections: {
    established: number;
    listen: number;
    timeWait: number;
    closeWait: number;
    synRecv: number;
    total: number;
    topPeers: ServerCount[] | null;
    readable: boolean;
  };
  notes?: string[];
}

export interface ServerAlert {
  id: string;
  serverId: string;
  serverName?: string;
  ruleId: string;
  ruleName?: string;
  kind?: string;
  subject?: string;
  severity: "warning" | "critical";
  message: string;
  value: number;
  startedAt: string;
  lastSeenAt: string;
  resolvedAt?: string;
  acknowledgedAt?: string;
}

export interface WatchedServer {
  id: string;
  name: string;
  host: string;
  port: number;
  username: string;
  hasSavedPassword: boolean;
  snapshot?: ServerSnapshot;
  checkedAt?: string;
  checkError?: string;
  createdAt: string;
  alerts: ServerAlert[];
}

export interface ServerAlertRule {
  id: string;
  kind: string;
  name: string;
  threshold: number;
  severity: "warning" | "critical";
  enabled: boolean;
  serverId: string;
  builtin: boolean;
  createdAt: string;
}

export interface ServerRuleKind {
  kind: string;
  label: string;
  unit: string;
  description: string;
}

export function getWatchedServers() {
  return fetchJSON<WatchedServer[]>("/api/servers");
}

export function getWatchedServer(id: string) {
  return fetchJSON<WatchedServer>(`/api/servers/${encodeURIComponent(id)}`);
}

export function getServerAlerts(all = false) {
  return fetchJSON<ServerAlert[]>(`/api/servers/alerts${all ? "?all=1" : ""}`);
}

export function getServerRules() {
  return fetchJSON<{ rules: ServerAlertRule[]; kinds: ServerRuleKind[] }>("/api/servers/rules");
}

// --- Envoy

export interface EnvoyRoute {
  virtualHost: string;
  domains: string[] | null;
  match: string;
  target: string;
  cluster?: string;
}

export interface EnvoyListener {
  name: string;
  address: string;
  port: number;
  kind: "http" | "tcp" | "other";
  routes: EnvoyRoute[];
  tcpCluster?: string;
  rds?: string;
  tls: boolean;
}

export interface EnvoyCluster {
  name: string;
  type: string;
  lbPolicy: string;
  endpoints: string[];
  healthCheck?: string;
  tls: boolean;
}

export interface EnvoyEndpoint {
  address: string;
  healthy: boolean;
  status: string;
  cxActive: number;
  rqTotal: number;
  rqError: number;
}

export interface EnvoyReport {
  container: string;
  image: string;
  version: string;
  state: string;
  uptime: string;
  configPath: string;
  files: { path: string; hostPath?: string; content: string; sha: string }[];
  testOk: boolean;
  testOutput: string;
  config: { admin: string; listeners: EnvoyListener[]; clusters: EnvoyCluster[]; dynamic?: string; parseError?: string };
  admin: string;
  adminReachable: boolean;
  adminError?: string;
  liveClusters: { name: string; dynamic: boolean; endpoints: EnvoyEndpoint[] }[];
  healthy: boolean;
  healthDetail: string;
  recentErrors: string[];
  stats: { cpu: string; memory: string; memShare: string; netIO: string; pids: string };
}

// --- Ansible

export interface AnsibleFile {
  path: string;
  content: string;
}

export interface AnsibleHostRecap {
  host: string;
  ok: number;
  changed: number;
  unreachable: number;
  failed: number;
  skipped: number;
  rescued: number;
  ignored: number;
}

export interface AnsibleRun {
  id: string;
  playbookId: string;
  playbookName: string;
  mode: "check" | "apply";
  contentSha: string;
  targets: { serverId: string; name: string; host: string; user: string }[];
  extraVars: string;
  status: "running" | "success" | "failed" | "canceled";
  log?: string;
  recap: AnsibleHostRecap[];
  error?: string;
  startedAt: string;
  finishedAt?: string;
}

export interface AnsiblePlaybook {
  id: string;
  name: string;
  description: string;
  files: AnsibleFile[] | null;
  entry: string;
  createdAt: string;
  updatedAt: string;
  lastRun?: AnsibleRun;
}

export interface AnsibleTemplate {
  key: string;
  name: string;
  description: string;
  files: AnsibleFile[];
}

export function getPlaybooks() {
  return fetchJSON<AnsiblePlaybook[]>("/api/ansible/playbooks");
}

export function getPlaybook(id: string) {
  return fetchJSON<AnsiblePlaybook>(`/api/ansible/playbooks/${encodeURIComponent(id)}`);
}

export function getAnsibleTemplates() {
  return fetchJSON<AnsibleTemplate[]>("/api/ansible/templates");
}

export function getAnsibleRuns(playbookId?: string) {
  return fetchJSON<AnsibleRun[]>(`/api/ansible/runs${playbookId ? `?playbook=${encodeURIComponent(playbookId)}` : ""}`);
}

export function getAnsibleRun(id: string) {
  return fetchJSON<AnsibleRun>(`/api/ansible/runs/${encodeURIComponent(id)}`);
}

// --- Terraform (OpenTofu)

export interface TfFile {
  path: string;
  content: string;
}

export interface TfSummary {
  add: number;
  change: number;
  destroy: number;
  replace: number;
  import?: number;
  changes?: { address: string; action: "create" | "update" | "delete" | "replace" | "read" | "import" }[];
}

// --- the Terraform standard (internal/terraform/analyze.go)

export type TfSeverity = "error" | "warning" | "info";

export interface TfFinding {
  rule: string;
  concept: string;
  severity: TfSeverity;
  file?: string;
  line?: number;
  message: string;
}

export interface TfRule {
  id: string;
  concept: string;
  severity: TfSeverity;
  title: string;
  why: string;
}

// TfSnippet is a concept's working example, added to the code from the
// Standards tab (GET /api/terraform/standard).
export interface TfSnippet {
  concept: string;
  title: string;
  file: string;
  code: string;
}

// TfProviderChoice is a pinned provider the Providers concept can add.
export interface TfProviderChoice {
  name: string;
  source: string;
  version: string;
  label: string;
}

export interface TfStandardInfo {
  rules: TfRule[];
  concepts: string[];
  snippets: TfSnippet[];
  providers: TfProviderChoice[];
}

export interface TfBlock {
  kind: "resource" | "data" | "module";
  type?: string;
  name: string;
  address: string;
  file: string;
  line: number;
  provider?: string;
  count?: boolean;
  forEach?: boolean;
  dependsOn?: string[];
  lifecycle?: { preventDestroy?: boolean; createBeforeDestroy?: boolean; ignoreChanges?: boolean; ignoreAll?: boolean; replaceTriggeredBy?: boolean };
  provisioners?: string[];
  dynamic?: string[];
  source?: string;
  version?: string;
  remote?: boolean;
}

export interface TfConcept {
  name: string;
  panel: string;
  here: string;
  enforced: boolean;
  status: "ok" | "error" | "warning" | "unused";
  rules?: string[];
}

export interface TfStandardView {
  requiredVersion?: string;
  backend?: string;
  providers: { name: string; source?: string; version?: string; declared: boolean; aliases?: string[] }[];
  resources: TfBlock[];
  dataSources: TfBlock[];
  modules: TfBlock[];
  variables: { name: string; type?: string; description?: string; hasDefault: boolean; sensitive: boolean; validations?: number; file: string; line: number }[];
  outputs: { name: string; description?: string; sensitive: boolean; file: string; line: number }[];
  locals: string[];
  imports: { to: string; id: string }[];
  moved?: number;
  functions: Record<string, number>;
  ternaries: number;
  interpolations: number;
  forExpressions: number;
  usesWorkspace: boolean;
  tfvarsFiles?: string[];
  findings: TfFinding[];
  parseErrors?: TfFinding[];
  concepts: TfConcept[];
  blocking: boolean;
}

export function getTfStandard() {
  return fetchJSON<{ rules: TfRule[]; concepts: string[] }>("/api/terraform/standard");
}

export interface TfRun {
  id: string;
  workspaceId: string;
  workspaceName: string;
  kind: "plan" | "destroy-plan" | "apply";
  status: "running" | "success" | "failed" | "canceled";
  contentSha: string;
  log?: string;
  error?: string;
  summary: TfSummary;
  appliesRunId?: string;
  appliedByRunId?: string;
  stateSerial: number;
  startedBy?: string;
  options: { replace?: string[] };
  checks?: TfFinding[];
  startedAt: string;
  finishedAt?: string;
}

export interface TfWorkspace {
  id: string;
  name: string;
  description: string;
  files: TfFile[] | null;
  createdAt: string;
  updatedAt: string;
  stateSerial: number;
  stateUpdatedAt?: string;
  ownBackend?: string;
  lastRun?: TfRun;
}

export interface TfVariable {
  key: string;
  kind: "terraform" | "env";
  value?: string;
  sensitive: boolean;
  hcl: boolean;
  keep?: boolean;
}

export interface TfState {
  managed: boolean;
  ownBackend?: string;
  serial: number;
  resources: { address: string; mode: string; type: string; name: string; module?: string; provider: string; instances: number }[];
  outputs: { name: string; value?: unknown; sensitive: boolean }[];
  versions: { serial: number; runId: string; createdAt: string }[];
}

export interface TfTemplate {
  key: string;
  name: string;
  description: string;
  files: TfFile[];
}

export function getTfWorkspaces() {
  return fetchJSON<TfWorkspace[]>("/api/terraform/workspaces");
}

export function getTfWorkspace(id: string) {
  return fetchJSON<TfWorkspace>(`/api/terraform/workspaces/${encodeURIComponent(id)}`);
}

export function getTfTemplates() {
  return fetchJSON<TfTemplate[]>("/api/terraform/templates");
}

export function getTfRuns(workspaceId?: string) {
  return fetchJSON<TfRun[]>(`/api/terraform/runs${workspaceId ? `?workspace=${encodeURIComponent(workspaceId)}` : ""}`);
}

export function getTfRun(id: string) {
  return fetchJSON<TfRun>(`/api/terraform/runs/${encodeURIComponent(id)}`);
}
