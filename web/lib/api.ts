import { cookies } from "next/headers";
import { redirect } from "next/navigation";

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
  | "failed";

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
