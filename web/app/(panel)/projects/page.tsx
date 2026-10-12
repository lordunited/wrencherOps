// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { TableCard } from "@/components/TableCard";
import Link from "next/link";
import { getProjects } from "@/lib/api";

export default async function ProjectsPage() {
  const projects = await getProjects();

  return (
    <div className="flex flex-col gap-4">
      <h1 className="page-title">
        Projects <span className="text-text-muted">({projects.length})</span>
      </h1>
      <TableCard>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gridline text-left text-text-secondary">
            <th className="py-2 pr-4 font-medium">Project</th>
            <th className="py-2 pr-4 font-medium">Visibility</th>
            <th className="py-2 pr-4 font-medium">Default branch</th>
            <th className="py-2 pr-4 font-medium">Open issues</th>
            <th className="py-2 font-medium">Last activity</th>
          </tr>
        </thead>
        <tbody>
          {projects.map((project) => (
            <tr key={project.id} className="border-b border-gridline">
              <td className="py-2 pr-4">
                <Link href={`/projects/${project.id}`} className="text-text-primary hover:underline">
                  {project.path_with_namespace}
                </Link>
              </td>
              <td className="py-2 pr-4 text-text-secondary">{project.visibility}</td>
              <td className="py-2 pr-4 text-text-secondary">{project.default_branch}</td>
              <td className="py-2 pr-4 tabular-nums text-text-secondary">
                {project.open_issues_count}
              </td>
              <td className="py-2 tabular-nums text-text-secondary">
                {new Date(project.last_activity_at).toLocaleString()}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      </TableCard>
    </div>
  );
}
