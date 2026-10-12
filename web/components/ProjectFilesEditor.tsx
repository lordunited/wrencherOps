// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useRef, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { YamlEditor, languageFor } from "@/components/YamlEditor";
import type { ProjectFile } from "@/lib/api";

export interface ProjectState {
  manifest: string;
  env: string;
  files: ProjectFile[];
}

const COMPOSE = "docker-compose.yml";
const ENV = ".env";
const MAX_FILE = 1 << 20;
const COMPOSE_NAMES = new Set(["docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"]);
const SKIPPED_DIRS = /(^|\/)(\.git|node_modules|\.idea|\.vscode|__pycache__)(\/|$)/;
const VALID_PATH = /^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$/;

function sizeLabel(s: string) {
  const n = new Blob([s]).size;
  return n < 1024 ? `${n} B` : `${(n / 1024).toFixed(n < 10240 ? 1 : 0)} KiB`;
}

// pathProblem mirrors the server's rules for a project file's path, so a
// bad one is caught while typing rather than on save.
function pathProblem(p: string, existing: ProjectState): string | null {
  if (!p) return "Enter a path.";
  if (COMPOSE_NAMES.has(p) || p === ENV || p === ".panel-files") return `${p} is managed separately.`;
  if (!VALID_PATH.test(p) || p.includes("//") || p.endsWith("/")) return "Use letters, digits, . _ - @ + and / between folders.";
  if (p.split("/").some((seg) => seg === ".." || seg === ".")) return "The path must stay inside the deploy folder.";
  if (existing.files.some((f) => f.path === p)) return `${p} already exists.`;
  return null;
}

// Every file of a custom compose project — docker-compose.yml, .env and any
// others its compose uses (configs, scripts, a static site) — with
// uploading of single files or a whole project folder, and editing of each.
export function ProjectFilesEditor({
  value,
  onChange,
  disabled,
  composeFile = COMPOSE,
}: {
  value: ProjectState;
  onChange: (next: ProjectState) => void;
  disabled?: boolean;
  // The name the compose file is written under on the server.
  composeFile?: string;
}) {
  const [selected, setSelected] = useState<string>(COMPOSE);
  const [newPath, setNewPath] = useState("");
  const [adding, setAdding] = useState(false);
  const filesInput = useRef<HTMLInputElement | null>(null);
  const folderInput = useRef<HTMLInputElement | null>(null);

  const current = selected === COMPOSE || selected === ENV ? null : value.files.find((f) => f.path === selected);
  const shownPath = selected !== COMPOSE && selected !== ENV && !current ? COMPOSE : selected;

  async function ingest(list: FileList | null, fromFolder: boolean) {
    if (!list || list.length === 0) return;
    const next: ProjectState = { ...value, files: [...value.files] };
    const added: string[] = [];
    const skipped: string[] = [];
    for (const f of Array.from(list)) {
      // A folder upload names each file "<folder>/<path>"; the folder itself
      // is the deploy folder, so its name is dropped.
      const p = fromFolder ? f.webkitRelativePath.split("/").slice(1).join("/") : f.name;
      if (!p || SKIPPED_DIRS.test(p) || p === ".panel-files" || p.endsWith(".DS_Store")) continue;
      if (f.size > MAX_FILE) {
        skipped.push(`${p} (over 1 MiB — keep data in a volume)`);
        continue;
      }
      const text = await f.text();
      if (text.includes("\u0000") || text.includes("�")) {
        skipped.push(`${p} (not a text file)`);
        continue;
      }
      if (COMPOSE_NAMES.has(p)) {
        next.manifest = text;
      } else if (p === ENV) {
        next.env = text;
      } else if (!VALID_PATH.test(p)) {
        skipped.push(`${p} (name has characters a project path can't)`);
        continue;
      } else {
        next.files = [...next.files.filter((x) => x.path !== p), { path: p, content: text }];
      }
      added.push(COMPOSE_NAMES.has(p) ? COMPOSE : p);
    }
    next.files.sort((a, b) => a.path.localeCompare(b.path));
    onChange(next);
    if (added.length) {
      toast.success(`Added ${added.length} file${added.length === 1 ? "" : "s"}.`);
      setSelected(added.includes(COMPOSE) ? COMPOSE : added[0]);
    }
    if (skipped.length) toast.error(`Skipped: ${skipped.join(", ")}`, { duration: 10000 });
  }

  function addFile() {
    const p = newPath.trim();
    const problem = pathProblem(p, value);
    if (problem) {
      toast.error(problem);
      return;
    }
    onChange({ ...value, files: [...value.files, { path: p, content: "" }].sort((a, b) => a.path.localeCompare(b.path)) });
    setSelected(p);
    setNewPath("");
    setAdding(false);
  }

  function setContent(content: string) {
    if (shownPath === COMPOSE) onChange({ ...value, manifest: content });
    else if (shownPath === ENV) onChange({ ...value, env: content });
    else onChange({ ...value, files: value.files.map((f) => (f.path === shownPath ? { ...f, content } : f)) });
  }

  const entries: { path: string; content: string; note?: string }[] = [
    { path: COMPOSE, content: value.manifest },
    { path: ENV, content: value.env, note: value.env.trim() ? undefined : "empty — not written" },
    ...value.files,
  ];
  const shownContent = shownPath === COMPOSE ? value.manifest : shownPath === ENV ? value.env : (current?.content ?? "");

  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-[15rem_minmax(0,1fr)]">
      <div className="flex flex-col gap-2">
        <ul className="flex flex-col gap-1" aria-label="Project files">
          {entries.map((e) => (
            <li key={e.path}>
              <button
                type="button"
                onClick={() => setSelected(e.path)}
                aria-current={shownPath === e.path}
                className={`flex w-full flex-col items-start rounded-xl px-3 py-2 text-left transition-colors ${
                  shownPath === e.path ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary hover:bg-surface-hover"
                }`}
              >
                <span className="break-all font-mono text-xs font-semibold">{e.path === COMPOSE ? composeFile : e.path}</span>
                <span className={`text-[11px] ${shownPath === e.path ? "text-on-accent/80" : "text-text-muted"}`}>
                  {e.note ?? sizeLabel(e.content)}
                </span>
              </button>
            </li>
          ))}
        </ul>
        {adding ? (
          <div className="flex flex-col gap-1.5 rounded-xl bg-surface-2 p-2">
            <input
              autoFocus
              value={newPath}
              onChange={(e) => setNewPath(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addFile();
                }
              }}
              placeholder="config/app.conf"
              className="field font-mono text-xs"
              disabled={disabled}
            />
            <div className="flex gap-1.5">
              <Button type="button" className="px-3 py-1 text-xs" onClick={addFile} disabled={disabled}>
                Add
              </Button>
              <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => setAdding(false)}>
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => setAdding(true)} disabled={disabled}>
              + New file
            </Button>
            <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => filesInput.current?.click()} disabled={disabled}>
              Upload files
            </Button>
            <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => folderInput.current?.click()} disabled={disabled}>
              Upload folder
            </Button>
          </div>
        )}
        <input
          ref={filesInput}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            ingest(e.target.files, false);
            e.target.value = "";
          }}
        />
        <input
          ref={(el) => {
            folderInput.current = el;
            el?.setAttribute("webkitdirectory", "");
          }}
          type="file"
          multiple
          hidden
          onChange={(e) => {
            ingest(e.target.files, true);
            e.target.value = "";
          }}
        />
        <p className="text-[11px] text-text-muted">
          Uploading a folder takes the whole project: its compose file (any of the usual names), .env and every
          other text file. Each file is written into the deploy folder on deploy; one you remove here is removed from
          the server on the next deploy.
        </p>
      </div>

      <div className="flex min-w-0 flex-col gap-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="break-all font-mono text-sm font-semibold text-text-primary">{shownPath === COMPOSE ? composeFile : shownPath}</span>
          {shownPath === ENV && value.env && (
            <Button type="button" variant="secondary" className="px-3 py-1 text-xs" onClick={() => onChange({ ...value, env: "" })} disabled={disabled}>
              Clear .env
            </Button>
          )}
          {shownPath !== COMPOSE && shownPath !== ENV && (
            <Button
              type="button"
              variant="danger"
              className="px-3 py-1 text-xs"
              onClick={() => {
                onChange({ ...value, files: value.files.filter((f) => f.path !== shownPath) });
                setSelected(COMPOSE);
              }}
              disabled={disabled}
            >
              Remove file
            </Button>
          )}
        </div>
        {shownPath === ENV && (
          <p className="text-xs text-text-muted">
            KEY=VALUE per line. Compose uses it to fill in <span className="font-mono">${"{"}VARIABLES{"}"}</span> in the
            compose file; a service only gets these as environment variables if the compose passes them on.
          </p>
        )}
        <YamlEditor
          key={shownPath}
          value={shownContent}
          onChange={setContent}
          readOnly={disabled}
          language={shownPath === COMPOSE ? "yaml" : languageFor(shownPath)}
        />
      </div>
    </div>
  );
}
