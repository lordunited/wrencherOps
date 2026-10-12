// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { YamlEditor } from "@/components/YamlEditor";
import type { DetectedProject, EnvFile } from "@/lib/api";

interface Creds {
  host: string;
  port: number;
  username: string;
  password: string;
}

interface Opened {
  path: string;
  content: string; // as last read or saved
  draft: string;
  sha: string; // "" while the file doesn't exist
  exists: boolean;
}

// A detected project's env files — its .env and every env_file the compose
// loads — read and edited live on the server. A save only goes through if
// the file is still the version that was opened, keeps the previous one as a
// backup, and "Save and apply" brings the running project up again so
// compose recreates exactly the containers whose environment changed.
export function EnvFilesEditor({ project, creds }: { project: DetectedProject; creds: () => Creds }) {
  const files = project.envFiles ?? [];
  const [opened, setOpened] = useState<Opened | null>(null);
  const [busy, setBusy] = useState<"open" | "save" | "apply" | null>(null);
  const [log, setLog] = useState<{ text: string; ok: boolean } | null>(null);
  const running = project.status?.includes("running") ?? false;

  if (files.length === 0) return null;

  async function post(path: string, body: object) {
    return fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...creds(), workingDir: project.workingDir, ...body }),
    });
  }

  async function open(f: EnvFile) {
    if (opened && opened.draft !== opened.content && !confirm(`Discard your changes to ${opened.path}?`)) return;
    setBusy("open");
    setLog(null);
    try {
      const res = await post("/api/apps/envfile", { path: f.path });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const body: { path: string; content: string; sha: string; exists: boolean } = await res.json();
      setOpened({ path: body.path, content: body.content, draft: body.content, sha: body.sha, exists: body.exists });
    } finally {
      setBusy(null);
    }
  }

  async function save(apply: boolean) {
    if (!opened) return;
    setBusy(apply ? "apply" : "save");
    setLog(null);
    try {
      const res = await post("/api/apps/envfile/save", { path: opened.path, content: opened.draft, baseSha: opened.sha, apply });
      if (res.status === 409) {
        toast.error(`${await res.text()} Your edit is still here; copy it before reopening.`, { duration: 12000 });
        return;
      }
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const body: { ok: boolean; log: string; error?: string; sha: string } = await res.json();
      // Saved either way: the new content is what's on the server now.
      setOpened({ ...opened, content: opened.draft, sha: body.sha, exists: true });
      setLog({ text: body.log + (body.error ? `\n${body.error}\n` : ""), ok: body.ok });
      if (body.ok) toast.success(apply && running ? `Saved and applied ${opened.path}.` : `Saved ${opened.path}.`);
      else toast.error(body.error ?? "Applying failed.", { duration: 10000 });
    } finally {
      setBusy(null);
    }
  }

  const dirty = opened !== null && opened.draft !== opened.content;

  return (
    <div className="flex flex-col gap-2 rounded-xl bg-surface-2 p-3">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="mr-1 text-xs font-semibold text-text-secondary">Env files</span>
        {files.map((f) => {
          const active = opened?.path === f.path;
          return (
            <button
              key={f.path}
              type="button"
              title={f.path}
              onClick={() => (active ? null : open(f))}
              disabled={busy !== null}
              aria-current={active}
              className={`rounded-lg px-2.5 py-1 font-mono text-xs transition-colors ${
                active ? "bg-accent text-on-accent" : "bg-surface text-text-primary hover:bg-surface-hover"
              } ${f.exists ? "" : "italic"}`}
            >
              {f.label}
              {!f.exists && <span className="ml-1 not-italic opacity-70">(missing)</span>}
            </button>
          );
        })}
        {busy === "open" && <span className="text-xs text-text-muted">Reading…</span>}
        {opened && (
          <Button
            type="button"
            variant="secondary"
            className="ml-auto px-3 py-1 text-xs"
            onClick={() => {
              if (dirty && !confirm(`Discard your changes to ${opened.path}?`)) return;
              setOpened(null);
              setLog(null);
            }}
          >
            Close
          </Button>
        )}
      </div>

      {opened && (
        <>
          <div className="flex flex-wrap items-center gap-2 text-xs text-text-muted">
            <span className="break-all font-mono">{opened.path}</span>
            {!opened.exists && <span>— doesn&apos;t exist yet; saving creates it.</span>}
            {dirty && <span className="font-semibold text-text-secondary">· unsaved changes</span>}
          </div>
          <YamlEditor
            key={opened.path}
            value={opened.draft}
            onChange={(draft) => setOpened((o) => (o ? { ...o, draft } : o))}
            readOnly={busy !== null}
            language="env"
          />
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" className="px-3 py-1.5 text-xs" onClick={() => save(true)} disabled={!dirty || busy !== null || !running}>
              {busy === "apply" ? "Applying…" : "Save and apply"}
            </Button>
            <Button type="button" variant="secondary" className="px-3 py-1.5 text-xs" onClick={() => save(false)} disabled={!dirty || busy !== null}>
              {busy === "save" ? "Saving…" : "Save only"}
            </Button>
            <Button
              type="button"
              variant="secondary"
              className="px-3 py-1.5 text-xs"
              onClick={() => setOpened({ ...opened, draft: opened.content })}
              disabled={!dirty || busy !== null}
            >
              Revert
            </Button>
          </div>
          <p className="text-[11px] text-text-muted">
            {running
              ? "Save and apply writes the file, then runs docker compose up -d for this project: only containers whose environment changed are recreated. "
              : "The project isn't running, so the file is only saved; it's used when the project next starts. "}
            The previous version is kept under /var/backups/devops-panel. A save is refused if the file changed on the
            server since you opened it.
          </p>
          {log && (
            <pre
              className={`max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface p-3 font-mono text-[11px] ${
                log.ok ? "text-text-secondary" : "text-status-critical"
              }`}
            >
              {log.text}
            </pre>
          )}
        </>
      )}
    </div>
  );
}

