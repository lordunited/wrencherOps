// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useCallback, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { AnsibleModulePicker } from "@/components/AnsibleModulePicker";
import { AnsibleProjectTree, breadcrumb, roleSkeleton } from "@/components/AnsibleProjectTree";
import { AnsibleRunView, runStatusRole } from "@/components/AnsibleRunView";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import { YamlEditor, languageFor } from "@/components/YamlEditor";
import type { EditorView } from "@codemirror/view";
import type { AnsibleFile, AnsiblePlaybook, AnsibleRun } from "@/lib/api";
import { insertTask } from "@/lib/ansibleSnippet";
import { timeAgo } from "@/lib/format";

const ENTRY = "playbook.yml";
const VALID_PATH = /^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$/;
const RESERVED = new Set(["inventory.ini", "ansible.cfg", "extra_vars.yml"]);
const SKIPPED_DIRS = /(^|\/)(\.git|node_modules|\.idea|\.vscode|__pycache__)(\/|$)/;

interface ServerOption {
  id: string;
  name: string;
  host: string;
  username: string;
  reachable: boolean;
}

function sameSet(a: string[], b: string[]) {
  return a.length === b.length && [...a].sort().join(",") === [...b].sort().join(",");
}

// PlaybookEditor is one playbook: its files, running it on servers from the
// Servers list (a dry run first, then for real), and its runs.
export function PlaybookEditor({ initial, servers, initialRuns }: { initial: AnsiblePlaybook; servers: ServerOption[]; initialRuns: AnsibleRun[] }) {
  const router = useRouter();
  const [saved, setSaved] = useState(initial);
  const canWrite = useCanWrite();
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [files, setFiles] = useState<AnsibleFile[]>(initial.files ?? []);
  // The file ansible-playbook runs: playbook.yml, or an uploaded project's own.
  const [entry, setEntry] = useState(initial.entry || ENTRY);
  const [selected, setSelected] = useState(initial.entry || ENTRY);
  const [newPath, setNewPath] = useState("");
  const [saving, setSaving] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [targets, setTargets] = useState<string[]>([]);
  const [extraVars, setExtraVars] = useState("");
  const [runs, setRuns] = useState(initialRuns);
  const [active, setActive] = useState<AnsibleRun | null>(initialRuns.find((r) => r.status === "running") ?? null);
  const [starting, setStarting] = useState<"check" | "apply" | null>(null);
  const [forceOpen, setForceOpen] = useState(false);
  const [approve, setApprove] = useState("");
  const [newRole, setNewRole] = useState<string | null>(null);
  const [picker, setPicker] = useState(false);
  const editorView = useRef<EditorView | null>(null);
  const newPathRef = useRef<HTMLInputElement | null>(null);
  const filesInput = useRef<HTMLInputElement | null>(null);
  const folderInput = useRef<HTMLInputElement | null>(null);
  const runRef = useRef<HTMLDivElement | null>(null);

  const dirty =
    name !== saved.name ||
    description !== saved.description ||
    entry !== (saved.entry || ENTRY) ||
    JSON.stringify(files) !== JSON.stringify(saved.files ?? []);
  const current = files.find((f) => f.path === selected) ?? files.find((f) => f.path === entry);
  // What can run: top-level YAML files (playbooks), not role or vars files.
  const yamlFiles = files.filter((f) => (/^[^/]+\.ya?ml$/.test(f.path) && !f.path.includes("/")) || f.path === entry);

  // A passed dry run of exactly this: the saved playbook (run after its last
  // save), these servers and these extra vars. The server checks the same,
  // and also that it's from the last day.
  const passedDryRun = runs.find(
    (r) =>
      r.mode === "check" &&
      r.status === "success" &&
      Date.parse(r.startedAt) >= Date.parse(saved.updatedAt) &&
      r.extraVars.trim() === extraVars.trim() &&
      sameSet(
        r.targets.map((t) => t.serverId),
        targets,
      ),
  );

  async function save() {
    setSaving(true);
    try {
      const res = await fetch(`/api/ansible/playbooks/${saved.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, description, files, entry }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const p: AnsiblePlaybook = await res.json();
      setSaved(p);
      setFiles(p.files ?? []);
      setEntry(p.entry || ENTRY);
      toast.success("Saved.");
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    const res = await fetch(`/api/ansible/playbooks/${saved.id}`, { method: "DELETE" });
    if (!res.ok) {
      toast.error(await res.text());
      return;
    }
    toast.success(`Deleted ${saved.name}. Its runs stay in the history.`);
    router.push("/ansible");
    router.refresh();
  }

  function addFile() {
    const p = newPath.trim();
    if (!p || !VALID_PATH.test(p) || p.includes("//") || p.endsWith("/") || p.split("/").some((s) => s === ".." || s === ".")) {
      toast.error("Use a relative path: letters, digits, . _ - @ + and / between folders.");
      return;
    }
    if (RESERVED.has(p)) {
      toast.error(`${p} is written by the panel for each run.`);
      return;
    }
    if (files.some((f) => f.path === p)) {
      toast.error(`${p} already exists.`);
      return;
    }
    setFiles([...files, { path: p, content: "" }].sort((a, b) => a.path.localeCompare(b.path)));
    setSelected(p);
    setNewPath("");
  }

  // createAt starts a file from the project tree: the standard main.yml of
  // a role folder right away, or (askName) the name box filled with its
  // folder, for a template or file of your naming.
  function createAt(p: string, askName?: boolean) {
    if (askName) {
      setNewPath(p.endsWith("/") ? p : p.slice(0, p.lastIndexOf("/") + 1));
      setTimeout(() => newPathRef.current?.focus(), 0);
      return;
    }
    if (!files.some((f) => f.path === p)) {
      const section = p.split("/").slice(-2, -1)[0];
      setFiles([...files, { path: p, content: `# ${section} of this role\n` }].sort((a, b) => a.path.localeCompare(b.path)));
    }
    setSelected(p);
  }

  // insertModule puts a task built in the module picker into the open file at
  // the cursor, or into a new task file next to it.
  function insertModule(yaml: string, where: "cursor" | "file", module: string) {
    if (where === "cursor") {
      if (!current || !/\.ya?ml$/.test(current.path) || !editorView.current) {
        toast.error("Open a YAML file (a playbook or a tasks file) to insert into.");
        return;
      }
      insertTask(editorView.current, yaml);
      setPicker(false);
      toast.success(`Inserted a ${module.replace(/^ansible\.builtin\./, "")} task into ${current.path}. Fill in its values, then Save.`);
      return;
    }
    const role = /^((?:.*\/)?roles\/[^/]+)\//.exec(current?.path ?? "")?.[1];
    const base = `${role ? role + "/" : ""}tasks/${module.replace(/^ansible\.builtin\./, "")}`;
    let path = `${base}.yml`;
    for (let n = 2; files.some((f) => f.path === path); n++) path = `${base}_${n}.yml`;
    setFiles([...files, { path, content: yaml }].sort((a, b) => a.path.localeCompare(b.path)));
    setSelected(path);
    setPicker(false);
    const ref = role ? path.slice(role.length + 1) : path;
    toast.success(`Created ${path}. Use it from ${role ? "the role's tasks" : "a play"} with: ansible.builtin.include_tasks: ${role ? ref.replace(/^tasks\//, "") : ref}`, { duration: 12000 });
  }

  function addRole() {
    const n = (newRole ?? "").trim();
    if (!/^[A-Za-z0-9_][A-Za-z0-9_.-]*$/.test(n)) {
      toast.error("A role name is letters, digits, _ . and -.");
      return;
    }
    if (files.some((f) => f.path.startsWith(`roles/${n}/`))) {
      toast.error(`The role ${n} already exists.`);
      return;
    }
    const skel = roleSkeleton(n);
    setFiles([...files, ...skel].sort((a, b) => a.path.localeCompare(b.path)));
    setSelected(skel[0].path);
    setNewRole(null);
    toast.success(`Started the ${n} role. Add it to a play's roles: list to use it, then Save.`);
  }

  async function ingest(list: FileList | null, fromFolder: boolean) {
    if (!list) return;
    const next = [...files];
    const skipped: string[] = [];
    let added = 0;
    for (const f of Array.from(list)) {
      const p = fromFolder ? f.webkitRelativePath.split("/").slice(1).join("/") : f.name;
      if (!p || SKIPPED_DIRS.test(p) || p.endsWith(".DS_Store")) continue;
      if (f.size > 1 << 20) {
        skipped.push(`${p} (over 1 MiB)`);
        continue;
      }
      const text = await f.text();
      if (text.includes("\u0000") || !VALID_PATH.test(p) || RESERVED.has(p)) {
        skipped.push(p);
        continue;
      }
      const i = next.findIndex((x) => x.path === p);
      if (i >= 0) next[i] = { path: p, content: text };
      else next.push({ path: p, content: text });
      added++;
    }
    setFiles(next.sort((a, b) => a.path.localeCompare(b.path)));
    if (added) toast.success(`Added ${added} file${added === 1 ? "" : "s"}. Save to keep them.`);
    if (skipped.length) toast.error(`Skipped: ${skipped.join(", ")}`, { duration: 10000 });
  }

  async function start(mode: "check" | "apply") {
    setStarting(mode);
    try {
      const res = await fetch(`/api/ansible/playbooks/${saved.id}/runs`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ serverIds: targets, mode, extraVars, approve: mode === "apply" && !passedDryRun ? approve : "" }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 12000 });
        return;
      }
      const run: AnsibleRun = await res.json();
      setActive(run);
      setRuns((rs) => [run, ...rs]);
      setForceOpen(false);
      setApprove("");
      setTimeout(() => runRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
    } finally {
      setStarting(null);
    }
  }

  const onFinished = useCallback((run: AnsibleRun) => {
    setRuns((rs) => rs.map((r) => (r.id === run.id ? run : r)));
    if (run.status === "success") toast.success(run.mode === "check" ? "Dry run passed — you can apply it now." : "Applied.");
    else if (run.status === "failed") toast.error(run.error || "The run failed.", { duration: 10000 });
  }, []);

  const running = active?.status === "running" && runs.find((r) => r.id === active.id)?.status === "running";
  const canRun = targets.length > 0 && !dirty && !starting && !running;

  return (
    <div className="flex flex-col gap-5">
      <Link href="/ansible" className="text-sm text-text-secondary hover:text-text-primary">
        ← Playbooks
      </Link>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <label className="flex flex-col gap-1 text-sm text-text-secondary">
            Name
            <input className="field" value={name} onChange={(e) => setName(e.target.value)} />
          </label>
          <label className="flex flex-col gap-1 text-sm text-text-secondary sm:col-span-2">
            Description
            <input className="field" value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canWrite && (
            <Button onClick={save} disabled={!dirty || saving}>
              {saving ? "Saving…" : "Save"}
            </Button>
          )}
          {dirty && <span className="text-xs text-status-warning">Unsaved changes — runs use the saved version.</span>}
          <span className="ml-auto text-xs text-text-muted">saved {timeAgo(saved.updatedAt)}</span>
          {!canWrite ? null : confirmDelete ? (
            <span className="flex items-center gap-2 text-xs">
              Delete this playbook? Its runs stay in the history; nothing on any server changes.
              <Button variant="danger" className="px-3 py-1 text-xs" onClick={remove}>
                Yes, delete
              </Button>
              <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setConfirmDelete(false)}>
                Cancel
              </Button>
            </span>
          ) : (
            <Button variant="danger" className="px-3 py-1 text-xs" onClick={() => setConfirmDelete(true)}>
              Delete
            </Button>
          )}
        </div>
      </section>

      <section className="card grid grid-cols-1 gap-3 p-4 sm:p-5 md:grid-cols-[16rem_minmax(0,1fr)]">
        <div className="flex flex-col gap-2">
          <div className="max-h-[36rem] overflow-y-auto rounded-xl bg-surface-2 p-1.5">
            <AnsibleProjectTree files={files} selected={current?.path ?? ""} entry={entry} onSelect={setSelected} onCreate={createAt} />
          </div>
          {newRole === null ? (
            <Button variant="secondary" className="w-fit px-3 py-1 text-xs" onClick={() => setNewRole("")}>
              + New role
            </Button>
          ) : (
            <div className="flex gap-1">
              <input
                autoFocus
                className="field min-w-0 flex-1 font-mono text-xs"
                placeholder="role name, e.g. nginx"
                value={newRole}
                onChange={(e) => setNewRole(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addRole();
                  }
                  if (e.key === "Escape") setNewRole(null);
                }}
              />
              <Button variant="secondary" className="px-3 py-1 text-xs" onClick={addRole}>
                Add
              </Button>
            </div>
          )}
          <div className="flex gap-1">
            <input
              ref={newPathRef}
              className="field min-w-0 flex-1 font-mono text-xs"
              placeholder="new file, e.g. roles/web/templates/site.conf.j2"
              value={newPath}
              onChange={(e) => setNewPath(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), addFile())}
            />
            <Button variant="secondary" className="px-3 py-1 text-xs" onClick={addFile}>
              Add
            </Button>
          </div>
          <div className="flex flex-wrap gap-1.5">
            <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => filesInput.current?.click()}>
              Upload files
            </Button>
            <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => folderInput.current?.click()}>
              Upload folder
            </Button>
          </div>
          <input ref={filesInput} type="file" multiple hidden onChange={(e) => (ingest(e.target.files, false), (e.target.value = ""))} />
          <input
            ref={(el) => {
              folderInput.current = el;
              el?.setAttribute("webkitdirectory", "");
            }}
            type="file"
            multiple
            hidden
            onChange={(e) => (ingest(e.target.files, true), (e.target.value = ""))}
          />
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            Runs
            <select className="field font-mono text-xs" value={entry} onChange={(e) => setEntry(e.target.value)}>
              {yamlFiles.map((f) => (
                <option key={f.path} value={f.path}>
                  {f.path}
                </option>
              ))}
            </select>
          </label>
          <p className="text-[11px] text-text-muted">
            The file above is what ansible-playbook runs. Roles, templates, files and vars go beside it, as in any Ansible
            project. Only ansible.builtin modules are available (ansible-core, no extra collections).
          </p>
        </div>
        {current && (
          <div className="flex min-w-0 flex-col gap-2">
            <div className="flex items-center gap-2">
              <span className="flex min-w-0 flex-wrap items-center gap-1 font-mono text-sm text-text-secondary">
                {breadcrumb(current.path).map((part, i, all) => (
                  <span key={i} className="flex items-center gap-1">
                    {i > 0 && <span className="text-text-muted">›</span>}
                    <span className={i === all.length - 1 ? "font-semibold text-text-primary" : ""}>{part}</span>
                  </span>
                ))}
              </span>
              <Button variant="secondary" className="ml-auto px-3 py-1 text-xs" onClick={() => setPicker(!picker)}>
                {picker ? "Hide modules" : "+ Insert module"}
              </Button>
              {current.path !== entry && (
                <Button
                  variant="danger"
                  className="px-3 py-1 text-xs"
                  onClick={() => {
                    setFiles(files.filter((f) => f.path !== current.path));
                    setSelected(entry);
                  }}
                >
                  Remove file
                </Button>
              )}
            </div>
            {picker && (
              <div className="rounded-2xl border border-border p-3">
                <AnsibleModulePicker onInsert={insertModule} onClose={() => setPicker(false)} />
                <p className="mt-2 text-[11px] text-text-muted">
                  Insert at cursor puts the task after the one your cursor is in, at its indentation (or right under tasks:,
                  handlers: or block:). Place the cursor in {current.path} first.
                </p>
              </div>
            )}
            <YamlEditor
              key={current.path}
              value={current.content}
              onChange={(content) => setFiles(files.map((f) => (f.path === current.path ? { ...f, content } : f)))}
              language={languageFor(current.path)}
              onEditor={(view) => {
                editorView.current = view;
              }}
            />
          </div>
        )}
      </section>

      {canWrite && (
      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Run it</h2>
        {servers.length === 0 ? (
          <p className="text-sm text-text-muted">
            No servers yet. Add them on{" "}
            <Link href="/servers" className="font-semibold text-accent hover:underline">
              Servers
            </Link>
            ; a playbook runs on the ones you pick here, logging in with their saved passwords.
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <label className="flex items-center gap-2 text-sm text-text-primary">
                <input
                  type="checkbox"
                  checked={targets.length === servers.length}
                  onChange={() => setTargets(targets.length === servers.length ? [] : servers.map((s) => s.id))}
                />
                All servers ({servers.length})
              </label>
            </div>
            <div className="flex flex-wrap gap-1.5">
              {servers.map((s) => {
                const on = targets.includes(s.id);
                return (
                  <label
                    key={s.id}
                    className={`flex cursor-pointer items-center gap-2 rounded-full px-3 py-1 text-xs transition-colors ${on ? "bg-accent text-on-accent" : "bg-surface-2 text-text-primary hover:bg-surface-hover"}`}
                  >
                    <input type="checkbox" className="h-3.5 w-3.5" checked={on} onChange={() => setTargets(on ? targets.filter((x) => x !== s.id) : [...targets, s.id])} />
                    <span className="font-semibold">{s.name}</span>
                    <span className={`font-mono ${on ? "text-on-accent/80" : "text-text-muted"}`}>
                      {s.username}@{s.host}
                    </span>
                    {!s.reachable && <span title="Its last check couldn't reach it">⚠</span>}
                  </label>
                );
              })}
            </div>
            <label className="flex flex-col gap-1 text-sm text-text-secondary">
              Extra vars (YAML, optional)
              <YamlEditor value={extraVars} onChange={setExtraVars} language="yaml" />
            </label>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="secondary" onClick={() => start("check")} disabled={!canRun}>
                {starting === "check" ? "Starting…" : "Dry run (--check --diff)"}
              </Button>
              <Button onClick={() => start("apply")} disabled={!canRun || !passedDryRun} title={passedDryRun ? undefined : "Run a dry run of exactly this first"}>
                {starting === "apply" ? "Starting…" : `Apply to ${targets.length || "…"} server${targets.length === 1 ? "" : "s"}`}
              </Button>
              {passedDryRun ? (
                <span className="text-xs text-status-good">✓ Dry run passed {timeAgo(passedDryRun.startedAt)}</span>
              ) : (
                targets.length > 0 &&
                !dirty && (
                  <button type="button" className="text-xs text-text-muted hover:underline" onClick={() => setForceOpen(!forceOpen)}>
                    Apply without a dry run…
                  </button>
                )
              )}
            </div>
            {forceOpen && !passedDryRun && (
              <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 p-3 text-sm">
                <p className="text-text-primary">
                  <b className="text-status-critical">⚠ No passed dry run of this exact playbook, vars and servers.</b> Some playbooks
                  can&apos;t dry-run (a task that needs an earlier one to have really happened). To apply anyway, type the
                  playbook&apos;s name: <b className="font-mono">{saved.name}</b>
                </p>
                <div className="flex flex-wrap gap-2">
                  <input className="field max-w-xs font-mono" value={approve} onChange={(e) => setApprove(e.target.value)} autoComplete="off" />
                  <Button variant="danger" onClick={() => start("apply")} disabled={approve.trim() !== saved.name || !canRun}>
                    Apply without a dry run
                  </Button>
                </div>
              </div>
            )}
            <p className="text-xs text-text-muted">
              Runs with <span className="font-mono">hosts: all</span> reach every server you picked. It logs in with each
              server&apos;s saved password and becomes root with it (sudo). Every run starts with a syntax check; a dry run
              changes nothing and shows the diff. Apply needs a passed dry run of the same saved playbook, extra vars and
              servers. Saved passwords are masked in the logs. A playbook has root on the servers it runs on, so only
              let people you trust write them.
            </p>
            {dirty && <p className="text-xs text-status-warning">Save the playbook to run it.</p>}
          </>
        )}
      </section>
      )}

      {active && (
        <section ref={runRef} className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="section-title">{active.status === "running" ? "Running" : "Last run"}</h2>
          <AnsibleRunView key={active.id} initial={active} onFinished={onFinished} />
        </section>
      )}

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="section-title">Runs</h2>
        {runs.length === 0 ? (
          <p className="text-sm text-text-muted">No runs yet.</p>
        ) : (
          <ul className="flex flex-col gap-1">
            {runs.slice(0, 30).map((r) => (
              <li key={r.id}>
                <Link href={`/ansible/runs/${r.id}`} className="flex flex-wrap items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs hover:bg-surface-hover">
                  <StatusBadge role={r.mode === "check" ? "muted" : "warning"} label={r.mode === "check" ? "dry run" : "apply"} />
                  <StatusBadge role={runStatusRole(r.status)} label={r.status} />
                  <span className="text-text-primary">{r.targets.map((t) => t.name).join(", ")}</span>
                  {r.error && <span className="text-status-critical">{r.error}</span>}
                  <span className="ml-auto text-text-muted">{timeAgo(r.startedAt)}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
