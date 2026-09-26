import { StatusBadge } from "@/components/StatusBadge";
import { getHealth } from "@/lib/api";

export default async function HealthPage() {
  const health = await getHealth();

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-xl font-semibold text-text-primary">Health</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Signals worth a look: repeated pipeline failures, merge requests sitting open too
          long, and runners that changed state.
        </p>
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-text-primary">
          Failing projects{" "}
          <span className="text-text-muted">({health.failingProjects.length})</span>
        </h2>
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-gridline text-left text-text-secondary">
              <th className="py-2 pr-4 font-medium">Project</th>
              <th className="py-2 pr-4 font-medium">Consecutive failures</th>
              <th className="py-2 font-medium">Latest pipeline</th>
            </tr>
          </thead>
          <tbody>
            {health.failingProjects.map((p) => (
              <tr key={p.projectPath} className="border-b border-gridline">
                <td className="py-2 pr-4 text-text-secondary">{p.projectPath}</td>
                <td className="py-2 pr-4">
                  <StatusBadge role="critical" label={String(p.consecutiveFailures)} />
                </td>
                <td className="py-2">
                  <a
                    href={p.lastPipelineWebUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-text-secondary hover:underline"
                  >
                    View pipeline ↗
                  </a>
                </td>
              </tr>
            ))}
            {health.failingProjects.length === 0 && (
              <tr>
                <td colSpan={3} className="py-3 text-text-muted">
                  No projects with repeated pipeline failures.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-text-primary">
          Stale merge requests{" "}
          <span className="text-text-muted">({health.staleMergeRequests.length})</span>
        </h2>
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-gridline text-left text-text-secondary">
              <th className="py-2 pr-4 font-medium">Project</th>
              <th className="py-2 pr-4 font-medium">Title</th>
              <th className="py-2 pr-4 font-medium">Author</th>
              <th className="py-2 font-medium">Age</th>
            </tr>
          </thead>
          <tbody>
            {health.staleMergeRequests.map((mr) => (
              <tr key={`${mr.projectPath}-${mr.webUrl}`} className="border-b border-gridline">
                <td className="py-2 pr-4 text-text-secondary">{mr.projectPath}</td>
                <td className="py-2 pr-4">
                  <a
                    href={mr.webUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="text-text-primary hover:underline"
                  >
                    {mr.title}
                  </a>
                </td>
                <td className="py-2 pr-4 text-text-secondary">{mr.author || "—"}</td>
                <td className="py-2">
                  <StatusBadge role="warning" label={`${mr.ageDays}d`} />
                </td>
              </tr>
            ))}
            {health.staleMergeRequests.length === 0 && (
              <tr>
                <td colSpan={4} className="py-3 text-text-muted">
                  No merge requests older than 14 days.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold text-text-primary">Runner transitions</h2>
        <p className="text-sm text-text-muted">
          Only shows flips observed since this backend started — not full history.
        </p>
        <ul className="flex flex-col gap-2 text-sm">
          {health.runnerEvents.map((event, i) => (
            <li key={i} className="flex items-center gap-2 border-b border-gridline pb-2">
              <StatusBadge
                role={event.kind === "runner_offline" ? "critical" : "good"}
                label={event.kind === "runner_offline" ? "offline" : "online"}
              />
              <span className="text-text-secondary">{event.message}</span>
              <span className="ml-auto tabular-nums text-text-muted">
                {new Date(event.timestamp).toLocaleString()}
              </span>
            </li>
          ))}
          {health.runnerEvents.length === 0 && (
            <li className="text-text-muted">No runner transitions observed yet.</li>
          )}
        </ul>
      </section>
    </div>
  );
}
