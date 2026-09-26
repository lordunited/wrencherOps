import Link from "next/link";
import { StatTile } from "@/components/StatTile";
import {
  getHealth,
  getMergeRequests,
  getPipelines,
  getProjects,
  getRunners,
  getUsers,
} from "@/lib/api";
import { compactNumber } from "@/lib/format";

export default async function OverviewPage() {
  const [projects, runners, users, pipelineGroups, mergeRequestGroups, health] =
    await Promise.all([
      getProjects(),
      getRunners(),
      getUsers(),
      getPipelines(),
      getMergeRequests(),
      getHealth(),
    ]);

  const onlineRunners = runners.filter((r) => r.online).length;
  const totalPipelines = pipelineGroups.reduce((sum, g) => sum + g.totalCount, 0);
  const activeUsers = users.filter((u) => u.state === "active").length;
  const openMergeRequests = mergeRequestGroups.reduce((sum, g) => sum + g.totalCount, 0);
  const healthIssueCount = health.failingProjects.length + health.staleMergeRequests.length;

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-xl font-semibold text-text-primary">Overview</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Read-only snapshot of everything this token can see.
        </p>
      </div>

      {healthIssueCount > 0 && (
        <Link
          href="/health"
          className="flex items-center gap-2 rounded-lg bg-surface-1 px-4 py-3 text-sm transition-colors hover:bg-surface-hover"
        >
          <span
            className="inline-block h-2 w-2 rounded-full"
            style={{ backgroundColor: "var(--status-critical)" }}
            aria-hidden
          />
          <span className="text-text-primary">
            {health.failingProjects.length} project{health.failingProjects.length === 1 ? "" : "s"}{" "}
            with repeated pipeline failures, {health.staleMergeRequests.length} stale merge
            request{health.staleMergeRequests.length === 1 ? "" : "s"}
          </span>
          <span className="ml-auto text-text-secondary">View health →</span>
        </Link>
      )}

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
        <StatTile label="Projects" value={compactNumber(projects.length)} href="/projects" />
        <StatTile
          label="Runners online"
          value={`${compactNumber(onlineRunners)} / ${compactNumber(runners.length)}`}
          href="/runners"
        />
        <StatTile label="Pipelines" value={compactNumber(totalPipelines)} href="/pipelines" />
        <StatTile
          label="Open MRs"
          value={compactNumber(openMergeRequests)}
          href="/merge-requests"
        />
        <StatTile
          label="Users active"
          value={`${compactNumber(activeUsers)} / ${compactNumber(users.length)}`}
          href="/users"
        />
      </div>
    </div>
  );
}
