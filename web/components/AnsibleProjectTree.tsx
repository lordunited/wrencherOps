// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import type { AnsibleFile } from "@/lib/api";

// A role's standard folders, in the order Ansible's docs list them, with
// what each holds — the file Ansible loads from it on its own, if any.
export const ROLE_SECTIONS: { key: string; label: string; hint: string; main?: string }[] = [
  { key: "tasks", label: "tasks", hint: "what the role does", main: "main.yml" },
  { key: "handlers", label: "handlers", hint: "run when notified, e.g. restart a service", main: "main.yml" },
  { key: "defaults", label: "defaults", hint: "default variables, lowest precedence", main: "main.yml" },
  { key: "vars", label: "vars", hint: "role variables, high precedence", main: "main.yml" },
  { key: "meta", label: "meta", hint: "dependencies and role info", main: "main.yml" },
  { key: "files", label: "files", hint: "copied as they are (copy, script)" },
  { key: "templates", label: "templates", hint: "Jinja2 templates (template)" },
];

const SECTION_KEYS = new Set(ROLE_SECTIONS.map((s) => s.key));

interface Role {
  name: string;
  base: string; // "roles/web" (or "ansible/roles/web")
  sections: Map<string, AnsibleFile[]>; // section key -> files in it
  other: AnsibleFile[]; // files outside the standard sections (library, README, ...)
}

interface Grouped {
  playbooks: AnsibleFile[];
  roles: Role[];
  vars: Map<string, AnsibleFile[]>; // "group_vars" / "host_vars" -> files
  other: AnsibleFile[];
}

// isInventory is a project's own inventory, which a run doesn't use.
function isInventory(p: string) {
  return /^(inventory|inventories)\//.test(p) || /^(hosts|hosts\.ini|inventory\.ya?ml)$/.test(p);
}

const roleRe = /^((?:.*\/)?roles\/([^/]+))\/(.+)$/;

function group(files: AnsibleFile[]): Grouped {
  const g: Grouped = { playbooks: [], roles: [], vars: new Map(), other: [] };
  const roles = new Map<string, Role>();
  for (const f of files) {
    const m = roleRe.exec(f.path);
    if (m) {
      const [, base, name, rest] = m;
      let role = roles.get(base);
      if (!role) {
        role = { name, base, sections: new Map(), other: [] };
        roles.set(base, role);
      }
      const section = rest.split("/")[0];
      if (SECTION_KEYS.has(section) && rest.includes("/")) {
        role.sections.set(section, [...(role.sections.get(section) ?? []), f]);
      } else {
        role.other.push(f);
      }
      continue;
    }
    const top = f.path.split("/")[0];
    if (!f.path.includes("/") && /\.ya?ml$/.test(f.path)) g.playbooks.push(f);
    else if (top === "group_vars" || top === "host_vars") g.vars.set(top, [...(g.vars.get(top) ?? []), f]);
    else g.other.push(f);
  }
  g.roles = [...roles.values()].sort((a, b) => a.base.localeCompare(b.base));
  return g;
}

function FileButton({
  file,
  label,
  selected,
  entry,
  onSelect,
  indent = 0,
  note,
}: {
  file: AnsibleFile;
  label: string;
  selected: string;
  entry: string;
  onSelect: (p: string) => void;
  indent?: number;
  note?: string;
}) {
  const on = selected === file.path;
  return (
    <button
      type="button"
      onClick={() => onSelect(file.path)}
      aria-current={on}
      title={note ? `${file.path} — ${note}` : file.path}
      style={{ paddingLeft: `${0.75 + indent * 0.75}rem` }}
      className={`flex w-full items-center gap-1.5 rounded-lg py-1 pr-2 text-left font-mono text-xs transition-colors ${
        on ? "bg-accent font-semibold text-on-accent" : "text-text-primary hover:bg-surface-hover"
      }`}
    >
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {file.path === entry && <span className={`shrink-0 font-sans text-[10px] ${on ? "text-on-accent/80" : "text-status-good"}`}>▶ runs</span>}
      {note && <span className={`shrink-0 font-sans text-[10px] ${on ? "text-on-accent/80" : "text-text-muted"}`}>not used</span>}
    </button>
  );
}

function Heading({ children, count }: { children: React.ReactNode; count?: number }) {
  return (
    <div className="mt-2 flex items-center gap-1.5 px-1 text-[11px] font-semibold uppercase tracking-wide text-text-muted first:mt-0">
      {children}
      {count !== undefined && <span className="font-normal normal-case">({count})</span>}
    </div>
  );
}

// AnsibleProjectTree shows a playbook's files the way an Ansible project is
// laid out: its playbooks; each role with tasks, handlers, defaults, vars,
// meta, files and templates (a missing standard folder can be started from
// here); group_vars and host_vars; and anything else as a folder tree.
export function AnsibleProjectTree({
  files,
  selected,
  entry,
  onSelect,
  onCreate,
}: {
  files: AnsibleFile[];
  selected: string;
  entry: string;
  onSelect: (path: string) => void;
  // onCreate starts a file, or (for files/templates) asks for its name.
  onCreate: (path: string, askName?: boolean) => void;
}) {
  const g = group(files);
  // Roles start open when there are few, or when the open file is in one.
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const isOpen = (r: Role) => open[r.base] ?? (g.roles.length <= 3 || selected.startsWith(r.base + "/"));

  return (
    <nav className="flex flex-col gap-0.5" aria-label="Playbook files">
      <Heading count={g.playbooks.length}>Playbooks</Heading>
      {g.playbooks.map((f) => (
        <FileButton key={f.path} file={f} label={f.path} selected={selected} entry={entry} onSelect={onSelect} />
      ))}

      <Heading count={g.roles.length}>Roles</Heading>
      {g.roles.length === 0 && <p className="px-3 text-[11px] text-text-muted">No roles.</p>}
      {g.roles.map((r) => {
        const expanded = isOpen(r);
        const count = [...r.sections.values()].reduce((n, l) => n + l.length, 0) + r.other.length;
        return (
          <div key={r.base} className="flex flex-col gap-0.5">
            <button
              type="button"
              onClick={() => setOpen({ ...open, [r.base]: !expanded })}
              aria-expanded={expanded}
              className="flex items-center gap-1.5 rounded-lg px-2 py-1 text-left text-sm font-bold text-text-primary hover:bg-surface-hover"
              title={r.base}
            >
              <span className="w-3 text-[10px] text-text-muted">{expanded ? "▾" : "▸"}</span>
              <span className="min-w-0 flex-1 truncate">{r.name}</span>
              <span className="text-[10px] font-normal text-text-muted">{count}</span>
            </button>
            {expanded && (
              <div className="ml-3 flex flex-col gap-0.5 border-l border-border pl-1">
                {ROLE_SECTIONS.map((s) => {
                  const list = (r.sections.get(s.key) ?? []).slice().sort((a, b) => a.path.localeCompare(b.path));
                  const prefix = `${r.base}/${s.key}/`;
                  return (
                    <div key={s.key} className="flex flex-col">
                      <div className="group flex items-center gap-1 px-1" title={s.hint}>
                        <span className={`text-[11px] font-semibold ${list.length ? "text-text-secondary" : "text-text-muted opacity-60"}`}>{s.label}</span>
                        <button
                          type="button"
                          onClick={() => onCreate(s.main ? prefix + s.main : prefix, !s.main || list.length > 0)}
                          className="ml-auto rounded px-1 text-[11px] text-text-muted opacity-0 transition-opacity hover:text-accent group-hover:opacity-100 focus:opacity-100"
                          aria-label={`Add a file to ${r.name}/${s.key}`}
                        >
                          +
                        </button>
                      </div>
                      {list.map((f) => (
                        <FileButton key={f.path} file={f} label={f.path.slice(prefix.length)} selected={selected} entry={entry} onSelect={onSelect} indent={1} />
                      ))}
                    </div>
                  );
                })}
                {r.other.length > 0 && (
                  <div className="flex flex-col">
                    <span className="px-1 text-[11px] font-semibold text-text-secondary">other</span>
                    {r.other.map((f) => (
                      <FileButton key={f.path} file={f} label={f.path.slice(r.base.length + 1)} selected={selected} entry={entry} onSelect={onSelect} indent={1} />
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        );
      })}

      {g.vars.size > 0 && (
        <>
          <Heading>Variables</Heading>
          {["group_vars", "host_vars"].map((k) =>
            g.vars.get(k)?.length ? (
              <div key={k} className="flex flex-col">
                <span className="px-1 text-[11px] font-semibold text-text-secondary">{k}</span>
                {g.vars
                  .get(k)!
                  .slice()
                  .sort((a, b) => a.path.localeCompare(b.path))
                  .map((f) => (
                    <FileButton key={f.path} file={f} label={f.path.slice(k.length + 1)} selected={selected} entry={entry} onSelect={onSelect} indent={1} />
                  ))}
              </div>
            ) : null,
          )}
        </>
      )}

      {g.other.length > 0 && (
        <>
          <Heading count={g.other.length}>Other files</Heading>
          {g.other
            .slice()
            .sort((a, b) => a.path.localeCompare(b.path))
            .map((f) => (
              <FileButton
                key={f.path}
                file={f}
                label={f.path}
                selected={selected}
                entry={entry}
                onSelect={onSelect}
                note={isInventory(f.path) ? "not used: the panel builds the inventory from the servers you pick" : undefined}
              />
            ))}
        </>
      )}
    </nav>
  );
}

// roleSkeleton is a new role's starting files: an empty task list and
// empty defaults, enough for Ansible to load it.
export function roleSkeleton(name: string): AnsibleFile[] {
  return [
    { path: `roles/${name}/tasks/main.yml`, content: `# Tasks of the ${name} role.\n- name: Example task\n  ansible.builtin.debug:\n    msg: "${name} role"\n` },
    { path: `roles/${name}/defaults/main.yml`, content: `# Default variables of the ${name} role (lowest precedence).\n` },
    { path: `roles/${name}/handlers/main.yml`, content: `# Handlers of the ${name} role, run when a task notifies them.\n` },
  ];
}

// breadcrumb splits a file path for display: roles/web/tasks/main.yml ->
// ["roles", "web", "tasks", "main.yml"].
export function breadcrumb(path: string): string[] {
  return path.split("/");
}
