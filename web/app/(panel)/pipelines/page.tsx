import { StatusBadge } from "@/components/StatusBadge";
import { getPipelines } from "@/lib/api";
import { pipelineStatusRole } from "@/lib/format";

const MAX_ROWS = 200;

export default async function PipelinesPage() {
  const groups = await getPipelines();

  const flattened = groups
    .flatMap((group) =>
      group.pipelines.map((pipeline) => ({ projectPath: group.projectPath, ...pipeline }))
    )
    .sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at));

  const grandTotal = groups.reduce((sum, g) => sum + g.totalCount, 0);
  const rows = flattened.slice(0, MAX_ROWS);

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold text-text-primary">
        Pipelines <span className="text-text-muted">({grandTotal})</span>
      </h1>
      <p className="text-sm text-text-muted">
        Showing the {rows.length} most recent (up to {MAX_ROWS}, up to 20 per project) across{" "}
        {groups.length} projects.
      </p>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gridline text-left text-text-secondary">
            <th className="py-2 pr-4 font-medium">Project</th>
            <th className="py-2 pr-4 font-medium">Status</th>
            <th className="py-2 pr-4 font-medium">Ref</th>
            <th className="py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((pipeline) => (
            <tr key={`${pipeline.projectPath}-${pipeline.id}`} className="border-b border-gridline">
              <td className="py-2 pr-4 text-text-secondary">{pipeline.projectPath}</td>
              <td className="py-2 pr-4">
                <StatusBadge role={pipelineStatusRole(pipeline.status)} label={pipeline.status} />
              </td>
              <td className="py-2 pr-4 text-text-secondary">{pipeline.ref}</td>
              <td className="py-2 tabular-nums text-text-secondary">
                <a href={pipeline.web_url} target="_blank" rel="noreferrer" className="hover:underline">
                  {new Date(pipeline.created_at).toLocaleString()}
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
