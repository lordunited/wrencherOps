// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { TableCard } from "@/components/TableCard";
import { StatusBadge } from "@/components/StatusBadge";
import { getRunners } from "@/lib/api";
import { runnerStatusRole } from "@/lib/format";

export default async function RunnersPage() {
  const runners = await getRunners();

  return (
    <div className="flex flex-col gap-4">
      <h1 className="page-title">
        Runners <span className="text-text-muted">({runners.length})</span>
      </h1>
      <TableCard>
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="border-b border-gridline text-left text-text-secondary">
            <th className="py-2 pr-4 font-medium">Description</th>
            <th className="py-2 pr-4 font-medium">Status</th>
            <th className="py-2 font-medium">Type</th>
          </tr>
        </thead>
        <tbody>
          {runners.map((runner) => (
            <tr key={runner.id} className="border-b border-gridline">
              <td className="py-2 pr-4 text-text-primary">
                {runner.description || <span className="text-text-muted">(no description)</span>}
              </td>
              <td className="py-2 pr-4">
                <StatusBadge role={runnerStatusRole(runner.status)} label={runner.status} />
              </td>
              <td className="py-2 text-text-secondary">{runner.runner_type}</td>
            </tr>
          ))}
        </tbody>
      </table>
      </TableCard>
    </div>
  );
}
