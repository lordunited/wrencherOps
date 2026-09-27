import { TableCard } from "@/components/TableCard";
import Link from "next/link";
import { StatTile } from "@/components/StatTile";
import { StatusBadge } from "@/components/StatusBadge";
import {
  getProject,
  getProjectMembers,
  getProjectMergeRequests,
  getProjectPipelines,
} from "@/lib/api";
import { accessLevelLabel, mergeRequestRole, pipelineStatusRole } from "@/lib/format";

export default async function ProjectDetailPage(props: PageProps<"/projects/[id]">) {
  const { id } = await props.params;
  const projectId = Number(id);

  const [project, pipelines, mergeRequests, members] = await Promise.all([
    getProject(projectId),
    getProjectPipelines(projectId),
    getProjectMergeRequests(projectId),
    getProjectMembers(projectId),
  ]);

  return (
    <div className="flex flex-col gap-8">
      <div>
        <Link href="/projects" className="text-sm text-text-secondary hover:text-text-primary">
          ← All projects
        </Link>
        <div className="mt-2 flex items-baseline gap-3">
          <h1 className="page-title">{project.name}</h1>
          <span className="text-sm text-text-muted">{project.visibility}</span>
        </div>
        <p className="mt-1 text-sm text-text-secondary">{project.path_with_namespace}</p>
        {project.description && (
          <p className="mt-2 text-sm text-text-secondary">{project.description}</p>
        )}
        <a
          href={project.web_url}
          target="_blank"
          rel="noreferrer"
          className="mt-2 inline-block text-sm text-text-secondary hover:underline"
        >
          View on GitLab ↗
        </a>
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatTile label="Open issues" value={String(project.open_issues_count)} />
        <StatTile label="Stars" value={String(project.star_count)} />
        <StatTile label="Forks" value={String(project.forks_count)} />
        <StatTile label="Default branch" value={project.default_branch} />
      </div>

      <section className="flex flex-col gap-3">
        <h2 className="section-title">
          Pipelines <span className="text-text-muted">({pipelines.length})</span>
        </h2>
        <TableCard>
      <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-gridline text-left text-text-secondary">
              <th className="py-2 pr-4 font-medium">Status</th>
              <th className="py-2 pr-4 font-medium">Ref</th>
              <th className="py-2 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {pipelines.map((pipeline) => (
              <tr key={pipeline.id} className="border-b border-gridline">
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
            {pipelines.length === 0 && (
              <tr>
                <td colSpan={3} className="py-3 text-text-muted">
                  No pipelines.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </TableCard>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="section-title">
          Open merge requests <span className="text-text-muted">({mergeRequests.length})</span>
        </h2>
        <TableCard>
      <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-gridline text-left text-text-secondary">
              <th className="py-2 pr-4 font-medium">Title</th>
              <th className="py-2 pr-4 font-medium">Author</th>
              <th className="py-2 pr-4 font-medium">Branches</th>
              <th className="py-2 font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {mergeRequests.map((mr) => (
              <tr key={mr.iid} className="border-b border-gridline">
                <td className="py-2 pr-4">
                  <div className="flex items-center gap-2">
                    <StatusBadge role={mergeRequestRole(mr.draft)} label={mr.draft ? "draft" : "open"} />
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
            {mergeRequests.length === 0 && (
              <tr>
                <td colSpan={4} className="py-3 text-text-muted">
                  No open merge requests.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </TableCard>
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="section-title">
          Members <span className="text-text-muted">({members.length})</span>
        </h2>
        <TableCard>
      <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-gridline text-left text-text-secondary">
              <th className="py-2 pr-4 font-medium">Username</th>
              <th className="py-2 pr-4 font-medium">Name</th>
              <th className="py-2 font-medium">Access level</th>
            </tr>
          </thead>
          <tbody>
            {members.map((member) => (
              <tr key={member.username} className="border-b border-gridline">
                <td className="py-2 pr-4">
                  <a
                    href={member.web_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-text-primary hover:underline"
                  >
                    {member.username}
                  </a>
                </td>
                <td className="py-2 pr-4 text-text-secondary">{member.name}</td>
                <td className="py-2 text-text-secondary">{accessLevelLabel(member.access_level)}</td>
              </tr>
            ))}
            {members.length === 0 && (
              <tr>
                <td colSpan={3} className="py-3 text-text-muted">
                  No members found.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </TableCard>
      </section>
    </div>
  );
}
