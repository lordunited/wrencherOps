// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { StatusBadge } from "@/components/StatusBadge";
import type { ComposeAnalysis, ComposeMount } from "@/lib/api";

const KIND_LABEL: Record<ComposeMount["kind"], string> = {
  bind: "folder/file on the server",
  volume: "Docker volume",
  anonymous: "anonymous volume",
  tmpfs: "in memory",
};

function where(m: ComposeMount) {
  switch (m.kind) {
    case "bind":
      return m.hostPath;
    case "volume":
      return m.external ? `${m.volume} (external — must already exist)` : m.volume;
    case "anonymous":
      return "a new unnamed volume on each fresh container";
    case "tmpfs":
      return "lost when the container stops";
  }
}

// What the project's volumes come to on each server — where every piece of
// data actually lives — and what in the compose is likely to break.
export function ComposeVolumes({ analysis, stale }: { analysis?: ComposeAnalysis; stale: boolean }) {
  if (!analysis) return null;
  const warnings = analysis.warnings ?? [];
  const mounts = analysis.mounts ?? [];

  return (
    <section className="card flex flex-col gap-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="section-title">Volumes</h2>
        {analysis.project && (
          <span className="text-xs text-text-muted">
            Compose project <span className="font-mono text-text-secondary">{analysis.project}</span>
          </span>
        )}
      </div>
      {stale && <p className="text-xs text-status-warning">Shows the last saved version — save to update it.</p>}
      {analysis.parseError && (
        <p className="rounded-2xl bg-surface-2 px-4 py-3 text-sm text-status-critical">
          The compose file isn&apos;t valid YAML: <span className="font-mono">{analysis.parseError}</span>
        </p>
      )}
      {warnings.length > 0 && (
        <ul className="flex flex-col gap-1.5">
          {warnings.map((w) => (
            <li key={w} className="rounded-2xl bg-[color-mix(in_srgb,var(--status-warning)_12%,transparent)] px-4 py-2.5 text-sm text-text-primary">
              {w}
            </li>
          ))}
        </ul>
      )}
      {mounts.length === 0 && !analysis.parseError ? (
        <p className="text-sm text-text-muted">The compose mounts no volumes: nothing a container writes outlives it.</p>
      ) : (
        <ul className="flex flex-col gap-1.5">
          {mounts.map((m, i) => (
            <li key={`${m.service}:${m.target}:${i}`} className="flex flex-col gap-1 rounded-2xl bg-surface-2 px-4 py-3">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-mono text-sm font-bold text-text-primary">{m.service}</span>
                <span className="font-mono text-xs text-text-secondary">{m.target}</span>
                <StatusBadge role="muted" label={KIND_LABEL[m.kind]} />
                {m.readOnly && <StatusBadge role="muted" label="read-only" />}
                {m.kind === "bind" && m.inProject && <StatusBadge role="good" label="from the project" />}
              </div>
              <p className="break-all font-mono text-xs text-text-muted">{where(m)}</p>
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs text-text-muted">
        Down keeps all of this. Delete on a server runs <span className="font-mono">docker compose down -v</span> and
        removes the deploy folder there — that erases its Docker volumes and every file in the folder, but not paths
        mounted from outside it.
      </p>
    </section>
  );
}
