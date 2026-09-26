import { StatusBadge } from "@/components/StatusBadge";
import { getMergeRequests } from "@/lib/api";
import { mergeRequestRole } from "@/lib/format";

const MAX_ROWS = 200;

export default async function MergeRequestsPage() {
  const groups = await getMergeRequests();

  const flattened = groups
    .flatMap((group) =>
      group.mergeRequests.map((mr) => ({ projectPath: group.projectPath, ...mr }))
    )
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));

  const grandTotal = groups.reduce((sum, g) => sum + g.totalCount, 0);
  const rows = flattened.slice(0, MAX_ROWS);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold text-text-primary">
        Merge requests <span className="text-text-muted">({grandTotal})</span>
      </h1>
      <p className="text-sm text-text-muted">
        Open merge requests only. Showing the {rows.length} most recent (up to {MAX_ROWS}, up to
        20 per project) across {groups.length} projects.
      </p>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gridline text-left text-text-secondary">
            <th className="py-2 pr-4 font-medium">Project</th>
            <th className="py-2 pr-4 font-medium">Title</th>
            <th className="py-2 pr-4 font-medium">Author</th>
            <th className="py-2 pr-4 font-medium">Branches</th>
            <th className="py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((mr) => (
            <tr key={`${mr.projectPath}-${mr.iid}`} className="border-b border-gridline">
              <td className="py-2 pr-4 text-text-secondary">{mr.projectPath}</td>
              <td className="py-2 pr-4">
                <div className="flex items-center gap-2">
                  <StatusBadge
                    role={mergeRequestRole(mr.draft)}
                    label={mr.draft ? "draft" : "open"}
                  />
                  <a
                    href={mr.web_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-text-primary hover:underline"
                  >
                    {mr.title}
                  </a>
                </div>
              </td>
              <td className="py-2 pr-4 text-text-secondary">{mr.author?.username ?? "—"}</td>
              <td className="py-2 pr-4 text-text-secondary">
                {mr.source_branch} → {mr.target_branch}
              </td>
              <td className="py-2 tabular-nums text-text-secondary">
                {new Date(mr.created_at).toLocaleString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
