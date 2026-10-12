// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useCanWrite } from "@/components/Access";
import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import { SummaryChips, TF_KIND, TfRunView, tfStatusRole } from "@/components/TfRunView";
import { TfStandards } from "@/components/TfStandards";
import { YamlEditor, languageFor } from "@/components/YamlEditor";
import type { EditorView } from "@codemirror/view";
import { importBlock, isImportAddress } from "@/lib/terraform";
import type { TfFile, TfRule, TfRun, TfStandardView, TfState, TfVariable, TfWorkspace } from "@/lib/api";
import { timeAgo } from "@/lib/format";

const LOCK = ".terraform.lock.hcl";
const VALID_PATH = /^[A-Za-z0-9._@+-][A-Za-z0-9._@+/-]*$/;
type Tab = "code" | "standards" | "variables" | "state" | "runs";


// The usual role of a root module file, by name, for the tree.
function roleOf(p: string): { group: string; order: number } {
  if (p === LOCK) return { group: "Lock file", order: 9 };
  if (/^(versions|providers|backend|terraform|provider)\.tf$/.test(p)) return { group: "Settings", order: 0 };
  if (/^variables?\.tf$/.test(p) || /\.tfvars$/.test(p)) return { group: "Variables", order: 1 };
  if (/^outputs?\.tf$/.test(p)) return { group: "Outputs", order: 3 };
  if (/\.tf(\.json)?$/.test(p)) return { group: "Resources", order: 2 };
  return { group: "Other files", order: 8 };
}

function Tree({ files, selected, onSelect }: { files: TfFile[]; selected: string; onSelect: (p: string) => void }) {
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const root = files.filter((f) => !f.path.includes("/"));
  const modules = new Map<string, TfFile[]>();
  const other: TfFile[] = [];
  for (const f of files) {
    const m = /^((?:.*\/)?modules\/[^/]+)\//.exec(f.path);
    if (m) modules.set(m[1], [...(modules.get(m[1]) ?? []), f]);
    else if (f.path.includes("/")) other.push(f);
  }
  const groups = new Map<string, { order: number; files: TfFile[] }>();
  for (const f of root) {
    const r = roleOf(f.path);
    const g = groups.get(r.group) ?? { order: r.order, files: [] };
    g.files.push(f);
    groups.set(r.group, g);
  }
  const item = (f: TfFile, label: string, indent = false) => (
    <button
      key={f.path}
      type="button"
      onClick={() => onSelect(f.path)}
      aria-current={selected === f.path}
      title={f.path}
      className={`flex w-full rounded-lg py-1 pr-2 text-left font-mono text-xs transition-colors ${indent ? "pl-6" : "pl-3"} ${
        selected === f.path ? "bg-accent font-semibold text-on-accent" : "text-text-primary hover:bg-surface-hover"
      }`}
    >
      <span className="truncate">{label}</span>
    </button>
  );
  const heading = (t: string) => <div className="mt-2 px-1 text-[11px] font-semibold uppercase tracking-wide text-text-muted first:mt-0">{t}</div>;
  return (
    <nav className="flex flex-col gap-0.5" aria-label="Workspace files">
      {heading("Root module")}
      {[...groups.entries()]
        .sort((a, b) => a[1].order - b[1].order)
        .map(([g, { files: list }]) => (
          <div key={g} className="flex flex-col">
            <span className="px-1 text-[11px] font-semibold text-text-secondary">{g.toLowerCase()}</span>
            {list.sort((a, b) => a.path.localeCompare(b.path)).map((f) => item(f, f.path, true))}
          </div>
        ))}
      {modules.size > 0 && heading(`Modules (${modules.size})`)}
      {[...modules.entries()]
        .sort((a, b) => a[0].localeCompare(b[0]))
        .map(([base, list]) => {
          const expanded = open[base] ?? (modules.size <= 3 || selected.startsWith(base + "/"));
          return (
            <div key={base} className="flex flex-col">
              <button type="button" onClick={() => setOpen({ ...open, [base]: !expanded })} aria-expanded={expanded} className="flex items-center gap-1.5 rounded-lg px-2 py-1 text-left text-sm font-bold text-text-primary hover:bg-surface-hover" title={base}>
                <span className="w-3 text-[10px] text-text-muted">{expanded ? "▾" : "▸"}</span>
                <span className="min-w-0 flex-1 truncate">{base.split("/").pop()}</span>
                <span className="text-[10px] font-normal text-text-muted">{list.length}</span>
              </button>
              {expanded && list.sort((a, b) => a.path.localeCompare(b.path)).map((f) => item(f, f.path.slice(base.length + 1), true))}
            </div>
          );
        })}
      {other.length > 0 && heading("Other files")}
      {other.sort((a, b) => a.path.localeCompare(b.path)).map((f) => item(f, f.path))}
    </nav>
  );
}

// TfWorkspaceEditor is one Terraform workspace: its code, variables and
// state, planning it, and applying a plan.
export function TfWorkspaceEditor({ initial, initialRuns }: { initial: TfWorkspace; initialRuns: TfRun[] }) {
  const router = useRouter();
  const [saved, setSaved] = useState(initial);
  const canWrite = useCanWrite();
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [files, setFiles] = useState<TfFile[]>(initial.files ?? []);
  const [selected, setSelected] = useState(() => (initial.files ?? []).find((f) => f.path === "main.tf")?.path ?? (initial.files ?? [])[0]?.path ?? "");
  const [newPath, setNewPath] = useState("");
  const [saving, setSaving] = useState(false);
  const [tab, setTab] = useState<Tab>("code");
  const [runs, setRuns] = useState(initialRuns);
  const [active, setActive] = useState<TfRun | null>(initialRuns.find((r) => r.status === "running") ?? initialRuns.find((r) => r.kind !== "apply" && r.status === "success" && !r.appliedByRunId) ?? null);
  const [starting, setStarting] = useState(false);
  const [state, setState] = useState<TfState | null>(null);
  const [vars, setVars] = useState<TfVariable[] | null>(null);
  const [varsDirty, setVarsDirty] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleteApproval, setDeleteApproval] = useState("");
  const runRef = useRef<HTMLDivElement | null>(null);
  const [std, setStd] = useState<TfStandardView | null>(null);
  const [rules, setRules] = useState<TfRule[]>([]);
  const [replace, setReplace] = useState<string[]>([]);
  const [showReplace, setShowReplace] = useState(false);
  const [customReplace, setCustomReplace] = useState("");
  const [showImport, setShowImport] = useState(false);
  const [importTo, setImportTo] = useState("");
  const [importId, setImportId] = useState("");
  const [formatting, setFormatting] = useState(false);
  const editorRef = useRef<EditorView | null>(null);
  const jumpTo = useRef<number | null>(null);

  const dirty = name !== saved.name || description !== saved.description || JSON.stringify(files) !== JSON.stringify(saved.files ?? []);
  const current = files.find((f) => f.path === selected);
  const running = runs.some((r) => r.status === "running");

  const loadState = useCallback(async () => {
    const res = await fetch(`/api/terraform/workspaces/${saved.id}/state`, { cache: "no-store" });
    if (res.ok) setState(await res.json());
  }, [saved.id]);

  const loadVars = useCallback(async () => {
    const res = await fetch(`/api/terraform/workspaces/${saved.id}/variables`, { cache: "no-store" });
    if (res.ok) {
      setVars(await res.json());
      setVarsDirty(false);
    }
  }, [saved.id]);

  useEffect(() => {
    const t = setTimeout(() => {
      loadState();
      loadVars();
    }, 0);
    return () => clearTimeout(t);
  }, [loadState, loadVars]);

  useEffect(() => {
    fetch("/api/terraform/standard", { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null))
      .then((s: { rules: TfRule[] } | null) => s && setRules(s.rules));
  }, []);

  // The code is checked against the standard as it's edited (the files in
  // the editor, saved or not), and again when variables change.
  useEffect(() => {
    const ctl = new AbortController();
    const t = setTimeout(async () => {
      try {
        const res = await fetch(`/api/terraform/workspaces/${saved.id}/analyze`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ files }),
          signal: ctl.signal,
        });
        if (res.ok) setStd(await res.json());
      } catch {
        // aborted by a newer edit
      }
    }, 500);
    return () => {
      clearTimeout(t);
      ctl.abort();
    };
  }, [files, saved.id, vars]);

  // openAt shows a file in the code tab, at a line.
  const openAt = useCallback((file: string, line?: number) => {
    setTab("code");
    setSelected(file);
    jumpTo.current = line ?? null;
  }, []);

  function onEditor(view: EditorView) {
    editorRef.current = view;
    const line = jumpTo.current;
    jumpTo.current = null;
    if (line && line <= view.state.doc.lines) {
      const pos = view.state.doc.line(line).from;
      setTimeout(() => {
        view.dispatch({ selection: { anchor: pos }, scrollIntoView: true });
        view.focus();
      }, 0);
    }
  }

  async function format() {
    setFormatting(true);
    try {
      const res = await fetch("/api/terraform/fmt", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ files }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const out: { files: TfFile[]; changed: string[]; errors: Record<string, string> } = await res.json();
      const errs = Object.entries(out.errors);
      if (errs.length > 0) toast.error(`Couldn't format ${errs.map(([f, e]) => `${f}: ${e}`).join("; ")}`, { duration: 15000 });
      if (out.changed.length === 0) {
        if (errs.length === 0) toast.success("Already formatted.");
        return;
      }
      setFiles(out.files);
      toast.success(`Formatted ${out.changed.join(", ")}. Save to keep it.`);
    } finally {
      setFormatting(false);
    }
  }

  function addImport() {
    const to = importTo.trim();
    const id = importId.trim();
    if (!isImportAddress(to) || !id) {
      toast.error("Give the resource's address in the code (e.g. aws_instance.web) and the real object's ID.");
      return;
    }
    const path = "imports.tf";
    const existing = files.find((f) => f.path === path);
    const block = importBlock(to, id);
    setFiles(
      existing
        ? files.map((f) => (f.path === path ? { ...f, content: f.content.replace(/\n*$/, "\n\n") + block } : f))
        : [...files, { path, content: block }].sort((a, b) => a.path.localeCompare(b.path)),
    );
    setSelected(path);
    setImportTo("");
    setImportId("");
    setShowImport(false);
    toast.success(`Import block added to ${path}. The resource block for ${to} must exist too; save, then plan to see the import.`);
  }

  async function save() {
    setSaving(true);
    try {
      const res = await fetch(`/api/terraform/workspaces/${saved.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name, description, files }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const w: TfWorkspace = await res.json();
      setSaved(w);
      setFiles(w.files ?? []);
      toast.success("Saved. Plan to see what it would change.");
    } finally {
      setSaving(false);
    }
  }

  async function remove() {
    const res = await fetch(`/api/terraform/workspaces/${saved.id}?approve=${encodeURIComponent(deleteApproval)}`, { method: "DELETE" });
    if (!res.ok) {
      toast.error(await res.text(), { duration: 12000 });
      return;
    }
    toast.success(`Deleted ${saved.name}.`);
    router.push("/terraform");
    router.refresh();
  }

  async function plan(kind: "plan" | "destroy-plan") {
    setStarting(true);
    try {
      const res = await fetch(`/api/terraform/workspaces/${saved.id}/runs`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(kind === "plan" && replace.length > 0 ? { kind, replace } : { kind }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const run: TfRun = await res.json();
      setRuns((rs) => [run, ...rs]);
      setActive(run);
      setReplace([]);
      setShowReplace(false);
      setTimeout(() => runRef.current?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
    } finally {
      setStarting(false);
    }
  }

  const onFinished = useCallback(
    (run: TfRun) => {
      setRuns((rs) => rs.map((r) => (r.id === run.id ? run : r)));
      if (run.status === "success") {
        if (run.kind === "apply") {
          toast.success("Applied.");
          loadState();
        } else toast.success(run.summary.add + run.summary.change + run.summary.destroy + run.summary.replace > 0 ? "Planned — review the changes, then apply." : "Planned: no changes.");
      } else if (run.status === "failed") toast.error(run.error || "The run failed.", { duration: 10000 });
      if (run.kind === "apply") loadState();
      fetch(`/api/terraform/workspaces/${saved.id}`, { cache: "no-store" })
        .then((r) => (r.ok ? r.json() : null))
        .then((w: TfWorkspace | null) => {
          // The lock file a run wrote, without losing unsaved edits.
          if (w && !dirty) {
            setSaved(w);
            setFiles(w.files ?? []);
          }
        });
    },
    [loadState, saved.id, dirty],
  );

  const onApplied = useCallback((apply: TfRun) => {
    setRuns((rs) => [apply, ...rs.map((r) => (r.id === apply.appliesRunId ? { ...r, appliedByRunId: apply.id } : r))]);
    setActive(apply);
  }, []);

  function addFile() {
    const p = newPath.trim();
    if (!p || !VALID_PATH.test(p) || p.includes("//") || p.split("/").some((s) => s === ".." || s === ".")) {
      toast.error("Use a relative path: letters, digits, . _ - @ + and / between folders.");
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

  async function saveVars() {
    if (!vars) return;
    const res = await fetch(`/api/terraform/workspaces/${saved.id}/variables`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      // A sensitive value never comes back from the server: one left as it
      // was keeps its stored value.
      body: JSON.stringify(vars.map((v) => (v.sensitive && v.value === undefined ? { ...v, keep: true } : v))),
    });
    if (!res.ok) {
      toast.error(await res.text(), { duration: 10000 });
      return;
    }
    setVars(await res.json());
    setVarsDirty(false);
    toast.success("Variables saved. They're used from the next plan.");
  }

  function setVar(i: number, patch: Partial<TfVariable>) {
    setVars((vs) => (vs ?? []).map((v, j) => (j === i ? { ...v, ...patch } : v)));
    setVarsDirty(true);
  }

  return (
    <div className="flex flex-col gap-5">
      <Link href="/terraform" className="text-sm text-text-secondary hover:text-text-primary">
        ← Workspaces
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
          {dirty && canWrite && <span className="text-xs text-status-warning">Unsaved changes — plans use the saved code.</span>}
          <span className="ml-auto text-xs text-text-muted">
            {saved.ownBackend ? `state in its own ${saved.ownBackend} backend` : saved.stateSerial ? `state serial ${saved.stateSerial}, kept by the panel` : "no state yet"} · saved {timeAgo(saved.updatedAt)}
          </span>
          {canWrite && (
            <Button variant="danger" className="px-3 py-1 text-xs" onClick={() => setConfirmDelete(!confirmDelete)}>
              Delete
            </Button>
          )}
        </div>
        {confirmDelete && canWrite && (
          <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 p-3 text-sm">
            <p className="text-text-primary">
              Delete this workspace, its variables and its state history? <b>Nothing in your infrastructure is deleted</b> — but if
              its state still has resources, the panel loses track of them: destroy them first. To delete anyway, type its name:{" "}
              <b className="font-mono">{saved.name}</b>
            </p>
            <div className="flex flex-wrap gap-2">
              <input className="field max-w-xs font-mono" value={deleteApproval} onChange={(e) => setDeleteApproval(e.target.value)} placeholder="only needed if it has resources" />
              <Button variant="danger" onClick={remove}>
                Delete workspace
              </Button>
            </div>
          </div>
        )}
      </section>

      <section ref={runRef} className="card flex flex-col gap-3 p-4 sm:p-5">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="section-title">Plan and apply</h2>
          <div className={`ml-auto flex-wrap gap-2 ${canWrite ? "flex" : "hidden"}`}>
            <Button variant="secondary" onClick={() => setShowReplace(!showReplace)} disabled={running || starting} title="Plan chosen resources for replacement (the successor of terraform taint)">
              Replace{replace.length > 0 ? ` (${replace.length})` : "…"}
            </Button>
            <Button onClick={() => plan("plan")} disabled={dirty || running || starting || Boolean(std?.blocking)} title={std?.blocking ? "The code breaks an enforced rule of the standard" : undefined}>
              {starting ? "Starting…" : replace.length > 0 ? `Plan with ${replace.length} replaced` : "Plan"}
            </Button>
            <Button variant="danger" onClick={() => plan("destroy-plan")} disabled={dirty || running || starting}>
              Plan destroy
            </Button>
          </div>
        </div>
        {std?.blocking && (
          <div className="flex flex-col gap-2 rounded-2xl border-l-4 bg-surface-2 p-3" style={{ borderLeftColor: "var(--status-critical)" }}>
            <p className="text-sm text-text-primary">
              <b className="text-status-critical">Plan is blocked</b> — the code breaks {std.findings.filter((f) => f.severity === "error").length} enforced
              rule{std.findings.filter((f) => f.severity === "error").length === 1 ? "" : "s"} of the standard.{" "}
              <button type="button" className="font-semibold text-accent hover:underline" onClick={() => setTab("standards")}>
                See Standards
              </button>
            </p>
          </div>
        )}
        {showReplace && canWrite && (
          <div className="flex flex-col gap-2 rounded-2xl bg-surface-2 p-3 text-sm">
            <p className="text-xs text-text-secondary">
              Replace (<span className="font-mono">-replace</span>, what <span className="font-mono">terraform taint</span> became): the plan
              destroys and recreates the chosen resources even though their code didn&apos;t change — e.g. a broken server. Applying it
              needs the workspace&apos;s name typed.
            </p>
            {state && state.resources.filter((r) => r.mode !== "data").length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {state.resources
                  .filter((r) => r.mode !== "data")
                  .map((r) => {
                    const on = replace.includes(r.address);
                    const many = r.instances > 1;
                    return (
                      <label
                        key={r.address}
                        className={`flex items-center gap-2 rounded-full px-3 py-1 font-mono text-xs ${many ? "opacity-50" : "cursor-pointer"} ${on ? "bg-accent-soft text-accent" : "bg-surface-1 text-text-secondary"}`}
                        title={many ? "Several instances: type the instance's address below, e.g. " + r.address + "[0]" : undefined}
                      >
                        <input type="checkbox" checked={on} disabled={many} onChange={() => setReplace(on ? replace.filter((a) => a !== r.address) : [...replace, r.address])} />
                        {r.address}
                      </label>
                    );
                  })}
              </div>
            ) : (
              <p className="text-xs text-text-muted">No resources in the state yet.</p>
            )}
            <div className="flex flex-wrap gap-2">
              <input
                className="field max-w-sm font-mono text-xs"
                placeholder='or an address, e.g. aws_instance.web["a"]'
                value={customReplace}
                onChange={(e) => setCustomReplace(e.target.value)}
              />
              <Button
                variant="secondary"
                className="px-3 py-1 text-xs"
                onClick={() => {
                  const a = customReplace.trim();
                  if (a && !replace.includes(a)) setReplace([...replace, a]);
                  setCustomReplace("");
                }}
              >
                Add
              </Button>
              {replace.length > 0 && (
                <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setReplace([])}>
                  Clear
                </Button>
              )}
            </div>
            {replace.length > 0 && <p className="font-mono text-xs text-text-primary">replace: {replace.join(", ")}</p>}
          </div>
        )}
        <p className="text-xs text-text-muted">
          Plan validates the code and saves what applying would change — nothing changes yet. Apply then applies exactly that
          saved plan. Runs use OpenTofu, Terraform&apos;s open-source fork (same code, same providers); HashiCorp&apos;s own
          provider registry isn&apos;t available in this region.
        </p>
        {active ? (
          <TfRunView key={active.id} initial={active} workspaceName={saved.name} onFinished={onFinished} onApplied={onApplied} />
        ) : (
          <p className="text-sm text-text-muted">No run yet. Plan to see what this would create.</p>
        )}
      </section>

      <nav className="inline-flex w-fit gap-1 rounded-full bg-surface-1 p-1 shadow-card" aria-label="Workspace sections">
        {(
          [
            ["code", "Code"],
            ["standards", `Standards${std ? (std.blocking ? " ⚠" : " ✓") : ""}`],
            ["variables", `Variables${vars ? ` (${vars.length})` : ""}`],
            ["state", `State${state ? ` (${state.resources.length})` : ""}`],
            ["runs", `Runs (${runs.length})`],
          ] as const
        ).map(([k, label]) => (
          <button
            key={k}
            type="button"
            onClick={() => setTab(k)}
            aria-current={tab === k ? "page" : undefined}
            className={`rounded-full px-4 py-1.5 text-sm font-semibold transition-colors ${tab === k ? "bg-accent text-on-accent" : "text-text-secondary hover:text-text-primary"}`}
          >
            {label}
          </button>
        ))}
      </nav>

      {tab === "code" && (
        <section className="card grid grid-cols-1 gap-3 p-4 sm:p-5 md:grid-cols-[16rem_minmax(0,1fr)]">
          <div className="flex flex-col gap-2">
            <div className="max-h-[36rem] overflow-y-auto rounded-xl bg-surface-2 p-1.5">
              <Tree files={files} selected={selected} onSelect={setSelected} />
            </div>
            <div className="flex gap-1">
              <input
                className="field min-w-0 flex-1 font-mono text-xs"
                placeholder="new file, e.g. network.tf"
                value={newPath}
                onChange={(e) => setNewPath(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && (e.preventDefault(), addFile())}
              />
              <Button variant="secondary" className="px-3 py-1 text-xs" onClick={addFile}>
                Add
              </Button>
            </div>
            <p className="text-[11px] text-text-muted">
              The root module is the .tf files at the top; modules/&lt;name&gt;/ hold local modules. {LOCK} is kept up to date by
              each run.
            </p>
            {canWrite && (
              <div className="flex flex-col gap-2 rounded-xl bg-surface-2 p-2">
                <div className="flex flex-wrap gap-1">
                  <Button variant="secondary" className="px-3 py-1 text-xs" onClick={format} disabled={formatting} title="tofu fmt every .tf and .tfvars file (rule TF010)">
                    {formatting ? "Formatting…" : "Format"}
                  </Button>
                  <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => setShowImport(!showImport)} title="Bring an existing object under management">
                    Import…
                  </Button>
                </div>
                {showImport && (
                  <div className="flex flex-col gap-1.5">
                    <input className="field font-mono text-xs" placeholder="address, e.g. aws_instance.web" value={importTo} onChange={(e) => setImportTo(e.target.value)} />
                    <input className="field font-mono text-xs" placeholder="the object's ID, e.g. i-0abc123" value={importId} onChange={(e) => setImportId(e.target.value)} />
                    <Button className="px-3 py-1 text-xs" onClick={addImport}>
                      Add import block
                    </Button>
                  </div>
                )}
              </div>
            )}
          </div>
          {current ? (
            <div className="flex min-w-0 flex-col gap-2">
              <div className="flex items-center gap-2">
                <span className="break-all font-mono text-sm font-semibold text-text-primary">{current.path}</span>
                <Button
                  variant="danger"
                  className="ml-auto px-3 py-1 text-xs"
                  onClick={() => {
                    setFiles(files.filter((f) => f.path !== current.path));
                    setSelected(files.find((f) => f.path !== current.path)?.path ?? "");
                  }}
                >
                  Remove file
                </Button>
              </div>
              {std && std.findings.some((f) => f.file === current.path) && (
                <ul className="flex flex-col gap-0.5">
                  {std.findings
                    .filter((f) => f.file === current.path)
                    .map((f, i) => (
                      <li key={i}>
                        <button
                          type="button"
                          onClick={() => openAt(current.path, f.line)}
                          className={`text-left text-xs hover:underline ${f.severity === "error" ? "text-status-critical" : f.severity === "warning" ? "text-status-warning" : "text-text-muted"}`}
                        >
                          {f.line ? `line ${f.line}: ` : ""}
                          {f.rule} {f.message}
                        </button>
                      </li>
                    ))}
                </ul>
              )}
              <YamlEditor
                key={current.path}
                value={current.content}
                onChange={(content) => setFiles(files.map((f) => (f.path === current.path ? { ...f, content } : f)))}
                language={languageFor(current.path)}
                readOnly={current.path === LOCK || !canWrite}
                onEditor={onEditor}
              />
            </div>
          ) : (
            <p className="text-sm text-text-muted">Pick a file.</p>
          )}
        </section>
      )}

      {tab === "standards" && <TfStandards std={std} rules={rules} onOpen={openAt} />}

      {tab === "variables" && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <p className="text-xs text-text-muted">
            <b>Terraform</b> variables fill the code&apos;s <span className="font-mono">variable</span> blocks (as TF_VAR_name);
            tick HCL for a list, map or number. <b>Environment</b> variables are for providers, e.g. AWS_ACCESS_KEY_ID. Values are
            stored encrypted; a <b>sensitive</b> one is never shown again and is masked in run logs.
          </p>
          {!vars ? (
            <p className="text-sm text-text-muted">Reading…</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {vars.map((v, i) => {
                const hidden = v.sensitive && v.keep !== false && v.value === undefined;
                return (
                  <li key={i} className="grid grid-cols-1 gap-2 rounded-2xl bg-surface-2 p-3 sm:grid-cols-[9rem_12rem_minmax(0,1fr)_auto]">
                    <select className="field text-xs" value={v.kind} onChange={(e) => setVar(i, { kind: e.target.value as TfVariable["kind"] })}>
                      <option value="terraform">Terraform</option>
                      <option value="env">Environment</option>
                    </select>
                    <input className="field font-mono text-xs" placeholder="name" value={v.key} onChange={(e) => setVar(i, { key: e.target.value })} />
                    {hidden ? (
                      <span className="flex items-center gap-2 text-xs text-text-muted">
                        •••••••• saved
                        <button type="button" className="font-semibold text-accent hover:underline" onClick={() => setVar(i, { keep: false, value: "" })}>
                          replace
                        </button>
                      </span>
                    ) : (
                      <input
                        className="field font-mono text-xs"
                        type={v.sensitive ? "password" : "text"}
                        placeholder={v.hcl ? '["a", "b"] or { k = "v" }' : "value"}
                        value={v.value ?? ""}
                        onChange={(e) => setVar(i, { value: e.target.value, keep: false })}
                        autoComplete="off"
                      />
                    )}
                    <span className="flex flex-wrap items-center gap-3 text-xs text-text-secondary">
                      <label className="flex items-center gap-1">
                        <input type="checkbox" checked={v.sensitive} onChange={(e) => setVar(i, { sensitive: e.target.checked, keep: false })} />
                        sensitive
                      </label>
                      {v.kind === "terraform" && (
                        <label className="flex items-center gap-1">
                          <input type="checkbox" checked={v.hcl} onChange={(e) => setVar(i, { hcl: e.target.checked })} />
                          HCL
                        </label>
                      )}
                      <button type="button" className="text-status-critical hover:underline" onClick={() => (setVars(vars.filter((_, j) => j !== i)), setVarsDirty(true))}>
                        remove
                      </button>
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
          <div className={`flex-wrap gap-2 ${canWrite ? "flex" : "hidden"}`}>
            <Button variant="secondary" onClick={() => (setVars([...(vars ?? []), { key: "", kind: "terraform", value: "", sensitive: false, hcl: false }]), setVarsDirty(true))}>
              + Add variable
            </Button>
            <Button onClick={saveVars} disabled={!varsDirty}>
              Save variables
            </Button>
          </div>
        </section>
      )}

      {tab === "state" && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          {!state ? (
            <p className="text-sm text-text-muted">Reading…</p>
          ) : !state.managed ? (
            <p className="text-sm text-text-secondary">
              The code declares its own <b className="font-mono">{state.ownBackend}</b> backend, so the state lives there, not in the
              panel. Set the backend&apos;s credentials as environment variables.
            </p>
          ) : (
            <>
              <p className="text-xs text-text-muted">
                Kept by the panel, encrypted · serial {state.serial || "—"} · {state.resources.length} resource{state.resources.length === 1 ? "" : "s"}
              </p>
              {state.resources.length === 0 ? (
                <p className="text-sm text-text-muted">No resources yet. Plan and apply to create them.</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[30rem] text-left text-xs">
                    <thead className="text-text-muted">
                      <tr>
                        <th className="py-1 pr-3 font-semibold">Resource</th>
                        <th className="py-1 pr-3 font-semibold">Provider</th>
                        <th className="py-1 font-semibold">Instances</th>
                      </tr>
                    </thead>
                    <tbody className="font-mono">
                      {state.resources.map((r) => (
                        <tr key={r.address} className="border-t border-border">
                          <td className="py-1 pr-3 text-text-primary">{r.address}</td>
                          <td className="py-1 pr-3 text-text-secondary">{r.provider}</td>
                          <td className="py-1 text-text-secondary">{r.instances}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              {state.outputs.length > 0 && (
                <div className="flex flex-col gap-1">
                  <span className="text-xs font-semibold text-text-secondary">Outputs</span>
                  {state.outputs.map((o) => (
                    <div key={o.name} className="flex flex-wrap gap-2 rounded-xl bg-surface-2 px-3 py-1.5 font-mono text-xs">
                      <span className="font-bold text-text-primary">{o.name}</span>
                      <span className="break-all text-text-secondary">{o.sensitive ? "(sensitive)" : JSON.stringify(o.value)}</span>
                    </div>
                  ))}
                </div>
              )}
              {state.versions.length > 0 && (
                <details className="text-xs text-text-secondary">
                  <summary className="cursor-pointer">State versions ({state.versions.length})</summary>
                  <ul className="mt-1 flex flex-col gap-0.5 font-mono">
                    {state.versions.map((v) => (
                      <li key={v.serial}>
                        serial {v.serial} · {new Date(v.createdAt).toLocaleString()}
                        {v.runId && (
                          <Link href={`/terraform/runs/${v.runId}`} className="ml-2 text-accent hover:underline">
                            run
                          </Link>
                        )}
                      </li>
                    ))}
                  </ul>
                </details>
              )}
            </>
          )}
        </section>
      )}

      {tab === "runs" && (
        <section className="card flex flex-col gap-2 p-4 sm:p-5">
          {runs.length === 0 ? (
            <p className="text-sm text-text-muted">No runs yet.</p>
          ) : (
            runs.slice(0, 50).map((r) => (
              <Link key={r.id} href={`/terraform/runs/${r.id}`} className="flex flex-wrap items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs hover:bg-surface-hover">
                <StatusBadge role={r.kind === "apply" ? "warning" : r.kind === "destroy-plan" ? "critical" : "muted"} label={TF_KIND[r.kind]} />
                <StatusBadge role={tfStatusRole(r.status)} label={r.status} />
                <SummaryChips summary={r.summary} />
                {r.appliedByRunId && <span className="text-status-good">applied</span>}
                {r.error && <span className="text-status-critical">{r.error}</span>}
                <span className="ml-auto text-text-muted">{timeAgo(r.startedAt)}</span>
              </Link>
            ))
          )}
        </section>
      )}
    </div>
  );
}
