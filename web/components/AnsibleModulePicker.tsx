// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { StatusBadge } from "@/components/StatusBadge";
import { taskYAML, type ModuleDoc, type ModuleOption, type ModuleSummary } from "@/lib/ansibleSnippet";

function short(name: string) {
  return name.replace(/^ansible\.builtin\./, "");
}

function checkRole(c: string) {
  return c === "full" ? "good" : c === "partial" ? "warning" : c === "none" ? "critical" : "muted";
}

function checkLabel(c: string) {
  return c === "full" ? "dry run: yes" : c === "partial" ? "dry run: partly" : c === "none" ? "dry run: skipped" : "dry run: ?";
}

function defaultText(o: ModuleOption) {
  if (o.default === undefined || o.default === null) return "";
  return typeof o.default === "object" ? JSON.stringify(o.default) : String(o.default);
}

// AnsibleModulePicker lists every built-in Ansible module (from the panel's
// own ansible-doc) and builds a task for the one you pick: tick the options
// you want, fill them in, and insert it — at the cursor, into a new task
// file, or copy it. Without onInsert it's a browsable reference.
export function AnsibleModulePicker({
  onInsert,
  onClose,
}: {
  onInsert?: (yaml: string, where: "cursor" | "file", module: string) => void;
  onClose?: () => void;
}) {
  const [list, setList] = useState<ModuleSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [doc, setDoc] = useState<ModuleDoc | null>(null);
  const [loading, setLoading] = useState(false);
  const [picked, setPicked] = useState<Record<string, boolean>>({});
  const [values, setValues] = useState<Record<string, string>>({});
  const [taskName, setTaskName] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [showExamples, setShowExamples] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const res = await fetch("/api/ansible/modules");
      if (cancelled) return;
      if (!res.ok) {
        setError((await res.text()).trim());
        return;
      }
      setList(await res.json());
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function open(name: string) {
    setLoading(true);
    try {
      const res = await fetch(`/api/ansible/modules/${encodeURIComponent(name)}`);
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const d: ModuleDoc = await res.json();
      setDoc(d);
      setPicked(Object.fromEntries(d.options.filter((o) => o.required).map((o) => [o.name, true])));
      setValues({});
      setTaskName(d.short.replace(/\.$/, ""));
      setShowAll(false);
      setShowExamples(false);
    } finally {
      setLoading(false);
    }
  }

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return (list ?? []).filter((m) => !q || m.name.toLowerCase().includes(q) || m.short.toLowerCase().includes(q));
  }, [list, query]);

  const yaml = doc
    ? taskYAML(
        doc.name,
        taskName,
        doc.options.filter((o) => picked[o.name]).map((o) => ({ option: o, value: values[o.name] ?? "" })),
      )
    : "";

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      toast.success("Copied.");
    } catch {
      toast.error("The browser didn't allow copying; select the text instead.");
    }
  }

  const options = doc ? (showAll ? doc.options : doc.options.filter((o) => o.required || picked[o.name] || doc.options.length <= 12)) : [];

  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-[15rem_minmax(0,1fr)]">
      <div className="flex min-h-0 flex-col gap-2">
        <input className="field" placeholder="Search modules (copy, apt, service…)" value={query} onChange={(e) => setQuery(e.target.value)} autoFocus />
        {error && <p className="text-xs text-status-critical">{error}</p>}
        {!list && !error && <p className="text-xs text-text-muted">Reading the modules…</p>}
        <ul className="flex max-h-[30rem] flex-col gap-0.5 overflow-y-auto rounded-xl bg-surface-2 p-1" aria-label="Modules">
          {filtered.map((m) => (
            <li key={m.name}>
              <button
                type="button"
                onClick={() => open(m.name)}
                aria-current={doc?.name === m.name}
                className={`flex w-full flex-col items-start rounded-lg px-2 py-1 text-left transition-colors ${
                  doc?.name === m.name ? "bg-accent text-on-accent" : "hover:bg-surface-hover"
                }`}
              >
                <span className="font-mono text-xs font-semibold">{short(m.name)}</span>
                <span className={`line-clamp-1 text-[11px] ${doc?.name === m.name ? "text-on-accent/80" : "text-text-muted"}`}>{m.short}</span>
              </button>
            </li>
          ))}
        </ul>
        {list && <p className="text-[11px] text-text-muted">{list.length} modules from ansible-core · {filtered.length} shown</p>}
      </div>

      <div className="flex min-w-0 flex-col gap-3">
        {!doc ? (
          <p className="rounded-xl bg-surface-2 px-4 py-8 text-center text-sm text-text-muted">
            {loading ? "Reading…" : "Pick a module to see what it does and build a task with it."}
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-mono text-base font-bold text-text-primary">{doc.name}</span>
              <StatusBadge role={checkRole(doc.checkMode)} label={checkLabel(doc.checkMode)} />
              {doc.diffMode === "full" && <StatusBadge role="muted" label="shows a diff" />}
              {onClose && (
                <Button variant="secondary" className="ml-auto px-3 py-1 text-xs" onClick={onClose}>
                  Close
                </Button>
              )}
            </div>
            <p className="text-sm text-text-secondary">{doc.description}</p>
            {doc.checkMode === "none" && (
              <p className="text-xs text-status-warning">
                A dry run (--check) skips this module, so it won&apos;t show what it would do; tasks after it that need its
                result may fail in a dry run.
              </p>
            )}

            <label className="flex flex-col gap-1 text-xs text-text-secondary">
              Task name
              <input className="field" value={taskName} onChange={(e) => setTaskName(e.target.value)} />
            </label>

            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-2">
                <span className="text-xs font-semibold text-text-secondary">Options</span>
                <span className="text-[11px] text-text-muted">
                  {doc.options.length} · tick the ones to include{doc.options.some((o) => o.required) ? "; required ones are ticked" : ""}
                </span>
                {doc.options.length > 12 && (
                  <button type="button" className="ml-auto text-xs font-semibold text-accent hover:underline" onClick={() => setShowAll(!showAll)}>
                    {showAll ? "Show fewer" : `Show all ${doc.options.length}`}
                  </button>
                )}
              </div>
              <ul className="flex max-h-[22rem] flex-col gap-1 overflow-y-auto">
                {options.map((o) => {
                  const on = !!picked[o.name];
                  return (
                    <li key={o.name} className={`flex flex-col gap-1 rounded-xl px-3 py-2 ${on ? "bg-surface-2" : ""}`}>
                      <div className="flex flex-wrap items-center gap-2">
                        <input
                          type="checkbox"
                          checked={on}
                          disabled={o.required}
                          onChange={() => setPicked({ ...picked, [o.name]: !on })}
                          aria-label={`Include ${o.name}`}
                        />
                        <span className="font-mono text-xs font-bold text-text-primary">{o.name}</span>
                        <span className="font-mono text-[11px] text-text-muted">
                          {o.type}
                          {o.elements ? ` of ${o.elements}` : ""}
                        </span>
                        {o.required && <span className="text-[11px] font-semibold text-status-critical">required</span>}
                        {defaultText(o) && <span className="font-mono text-[11px] text-text-muted">default {defaultText(o)}</span>}
                      </div>
                      <p className="pl-6 text-[11px] text-text-secondary">{o.description}</p>
                      {on &&
                        (o.choices && o.choices.length > 0 ? (
                          <select
                            className="field ml-6 w-fit font-mono text-xs"
                            value={values[o.name] ?? ""}
                            onChange={(e) => setValues({ ...values, [o.name]: e.target.value })}
                          >
                            <option value="">— choose —</option>
                            {o.choices.map((c) => (
                              <option key={String(c)} value={String(c)}>
                                {String(c)}
                              </option>
                            ))}
                          </select>
                        ) : o.type === "bool" ? (
                          <select className="field ml-6 w-fit font-mono text-xs" value={values[o.name] ?? "true"} onChange={(e) => setValues({ ...values, [o.name]: e.target.value })}>
                            <option value="true">true</option>
                            <option value="false">false</option>
                          </select>
                        ) : (
                          <input
                            className="field ml-6 font-mono text-xs"
                            placeholder={o.type === "list" ? "a, b, c" : o.type === "dict" ? "{ key: value }" : defaultText(o) || o.type}
                            value={values[o.name] ?? ""}
                            onChange={(e) => setValues({ ...values, [o.name]: e.target.value })}
                          />
                        ))}
                      {o.suboptions && on && <p className="pl-6 text-[11px] text-text-muted">Has its own options — see the examples.</p>}
                    </li>
                  );
                })}
              </ul>
            </div>

            <div className="flex flex-col gap-2">
              <span className="text-xs font-semibold text-text-secondary">The task</span>
              <pre className="overflow-x-auto rounded-xl bg-surface-2 p-3 font-mono text-xs text-text-primary">{yaml}</pre>
              <div className="flex flex-wrap gap-2">
                {onInsert && (
                  <>
                    <Button onClick={() => onInsert(yaml, "cursor", doc.name)}>Insert at cursor</Button>
                    <Button variant="secondary" onClick={() => onInsert(yaml, "file", doc.name)}>
                      New task file with it
                    </Button>
                  </>
                )}
                <Button variant="secondary" onClick={() => copy(yaml)}>
                  Copy
                </Button>
              </div>
            </div>

            {doc.examples && (
              <div className="flex flex-col gap-1">
                <button type="button" className="w-fit text-xs font-semibold text-accent hover:underline" onClick={() => setShowExamples(!showExamples)}>
                  {showExamples ? "Hide" : "Show"} the module&apos;s examples
                </button>
                {showExamples && (
                  <>
                    <pre className="max-h-80 overflow-auto rounded-xl bg-surface-2 p-3 font-mono text-[11px] text-text-secondary">{doc.examples}</pre>
                    <div className="flex gap-2">
                      {onInsert && (
                        <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => onInsert(doc.examples + "\n", "cursor", doc.name)}>
                          Insert the examples
                        </Button>
                      )}
                      <Button variant="secondary" className="px-3 py-1 text-xs" onClick={() => copy(doc.examples)}>
                        Copy the examples
                      </Button>
                    </div>
                  </>
                )}
              </div>
            )}
            {doc.notes && doc.notes.length > 0 && (
              <details className="text-[11px] text-text-muted">
                <summary className="cursor-pointer">Notes ({doc.notes.length})</summary>
                <ul className="mt-1 flex flex-col gap-1">
                  {doc.notes.map((n, i) => (
                    <li key={i}>• {n}</li>
                  ))}
                </ul>
              </details>
            )}
          </>
        )}
      </div>
    </div>
  );
}
