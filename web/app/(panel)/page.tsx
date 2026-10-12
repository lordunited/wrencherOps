// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import Link from "next/link";
import { ServersOverview } from "@/components/ServersOverview";
import { StatTile } from "@/components/StatTile";
import {
  getHealth,
  getMe,
  getMergeRequests,
  getPipelines,
  getProjects,
  getRunners,
  getUsers,
  getWatchedServers,
} from "@/lib/api";
import { compactNumber } from "@/lib/format";
import { visibleCategories } from "@/lib/nav";
import { unstable_rethrow } from "next/navigation";

export default async function OverviewPage() {
  const me = await getMe();
  const can = me.tabs;
  // A section that fails (GitLab unreachable, say) is left out with a
  // note, instead of taking the whole overview down.
  const failures = new Set<string>();
  // when fetches only what the user can see: a section they have no
  // access to isn't asked for (the backend would refuse it).
  const when = async <T,>(allowed: unknown, load: () => Promise<T>): Promise<T | null> => {
    if (!allowed) return null;
    try {
      return await load();
    } catch (err) {
      unstable_rethrow(err); // sign-in, no access, not found: Next's own
      failures.add(err instanceof Error ? err.message : String(err));
      return null;
    }
  };
  const [projects, runners, users, pipelineGroups, mergeRequestGroups, health, servers] = await Promise.all([
    when(can["gitlab.projects"], getProjects),
    when(can["gitlab.runners"], getRunners),
    when(can["gitlab.users"], getUsers),
    when(can["gitlab.pipelines"], getPipelines),
    when(can["gitlab.merge-requests"], getMergeRequests),
    when(can["gitlab.health"], getHealth),
    when(can["servers"], getWatchedServers),
  ]);

  const healthIssueCount = health ? health.failingProjects.length + health.staleMergeRequests.length : 0;
  const tiles = [
    projects && { label: "Projects", value: compactNumber(projects.length), href: "/projects", hint: "Visible to this token" },
    runners && {
      label: "Runners online",
      value: `${compactNumber(runners.filter((r) => r.online).length)} / ${compactNumber(runners.length)}`,
      href: "/runners",
    },
    pipelineGroups && { label: "Pipelines", value: compactNumber(pipelineGroups.reduce((sum, g) => sum + g.totalCount, 0)), href: "/pipelines" },
    mergeRequestGroups && {
      label: "Open MRs",
      value: compactNumber(mergeRequestGroups.reduce((sum, g) => sum + g.totalCount, 0)),
      href: "/merge-requests",
    },
    users && {
      label: "Users active",
      value: `${compactNumber(users.filter((u) => u.state === "active").length)} / ${compactNumber(users.length)}`,
      href: "/users",
    },
  ].filter((t): t is { label: string; value: string; href: string; hint?: string } => Boolean(t));
  const pages = visibleCategories(can).flatMap((c) => c.items.map((i) => ({ ...i, category: c.name })));

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="page-title">Overview</h1>
        <p className="mt-1 text-sm text-text-secondary">Read-only snapshot of everything you can see.</p>
      </div>

      {failures.size > 0 && (
        <div className="card flex flex-col gap-1.5 border-l-4 px-4 py-3" style={{ borderLeftColor: "var(--status-warning)" }} role="status">
          <p className="text-sm font-semibold text-text-primary">Part of the overview couldn&apos;t be loaded</p>
          {[...failures].map((f) => (
            <p key={f} className="text-xs text-text-secondary">
              {f}
            </p>
          ))}
        </div>
      )}

      {servers && <ServersOverview servers={servers} />}

      {health && healthIssueCount > 0 && (
        <Link
          href="/health"
          className="flex items-center gap-2 card px-4 py-3 text-sm transition-colors hover:bg-surface-hover"
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

      {tiles.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
          {tiles.map((t, i) => (
            <div key={t.href} className={i === 0 ? "col-span-2 sm:col-span-1" : undefined}>
              <StatTile hero={i === 0} label={t.label} value={t.value} href={t.href} hint={t.hint} />
            </div>
          ))}
        </div>
      )}

      {!servers && tiles.length === 0 && failures.size === 0 && (
        <div className="card flex flex-col gap-3 p-5">
          <h2 className="text-sm font-bold text-text-primary">Your pages</h2>
          {pages.length === 0 ? (
            <p className="text-sm text-text-secondary">
              Your account doesn&apos;t have access to any part of the panel yet. Ask an admin to give you access in Admin › Users
              &amp; access.
            </p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {pages.map((p) => (
                <Link key={p.href} href={p.href} className="rounded-xl bg-surface-2 px-3 py-2 text-sm font-semibold text-text-primary hover:bg-surface-hover">
                  <span className="text-text-muted">{p.category} › </span>
                  {p.label}
                </Link>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
