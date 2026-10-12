// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { EditorView } from "@codemirror/view";

export interface ModuleSummary {
  name: string;
  short: string;
  checkMode: string;
}

export interface ModuleOption {
  name: string;
  description: string;
  type: string;
  elements?: string;
  required: boolean;
  default?: unknown;
  choices?: unknown[];
  aliases?: string[];
  suboptions?: boolean;
}

export interface ModuleDoc extends ModuleSummary {
  description: string;
  options: ModuleOption[];
  notes?: string[];
  examples: string;
  diffMode: string;
}

// A plain YAML scalar unless it would read as something else (a number,
// a bool, null, a mapping, a comment, an alias...): then JSON-quoted, which
// is valid YAML.
export function yamlScalar(v: string): string {
  if (v === "") return '""';
  const special = /^[\s\-?:,\[\]{}#&*!|>'"%@`]|[:#]\s|\s$|^(true|false|yes|no|on|off|null|~)$/i.test(v) || /^[-+]?(\d|\.\d)/.test(v);
  return special ? JSON.stringify(v) : v;
}

// optionValue formats a value typed for an option, by its type.
export function optionValue(o: ModuleOption, raw: string): string {
  const v = raw.trim();
  switch (o.type) {
    case "bool":
      return v === "" ? "true" : /^(true|yes|on|1)$/i.test(v) ? "true" : "false";
    case "int":
    case "float":
      return v === "" ? "0" : /^-?\d+(\.\d+)?$/.test(v) ? v : yamlScalar(v);
    case "list":
      return v === "" ? "[]" : "[" + v.split(",").map((x) => yamlScalar(x.trim())).filter((x) => x !== '""').join(", ") + "]";
    case "dict":
      return v === "" ? "{}" : v;
    default:
      // A template or variable stays quoted, as Ansible needs.
      return v.startsWith("{{") ? JSON.stringify(v) : yamlScalar(v);
  }
}

// taskYAML is a task using module with the chosen options, at indent 0.
export function taskYAML(module: string, name: string, options: { option: ModuleOption; value: string }[]): string {
  const lines = [`- name: ${yamlScalar(name || "Describe what this does")}`, `  ${module}:`];
  for (const { option, value } of options) {
    const v = optionValue(option, value);
    // An empty value is left for you to fill in, and marked so it stands out.
    const empty = value.trim() === "" && !["bool", "int", "float", "list", "dict"].includes(option.type);
    lines.push(`    ${option.name}: ${v}${empty ? (option.required ? "  # required — fill in" : "  # fill in") : ""}`);
  }
  if (options.length === 0) lines[1] += " {}";
  return lines.join("\n") + "\n";
}

function indentOf(line: string) {
  return line.length - line.trimStart().length;
}

const LIST_KEYS = /^\s*(tasks|handlers|pre_tasks|post_tasks|block|rescue|always):\s*(#.*)?$/;

// insertTask puts a task into the editor where it belongs: at the
// indentation of the task list the cursor is in, after the task it's in
// (or right there on a blank line) — then selects it.
export function insertTask(view: EditorView, task: string) {
  const doc = view.state.doc;
  const cur = doc.lineAt(view.state.selection.main.head);
  // The list the cursor is in: under the nearest "tasks:"-like key above,
  // else the nearest list item above.
  let indent = 0;
  for (let n = cur.number; n >= 1; n--) {
    const t = doc.line(n).text;
    if (!t.trim() || t.trim().startsWith("#")) continue;
    if (LIST_KEYS.test(t)) {
      indent = indentOf(t) + 2;
      break;
    }
    if (/^\s*- /.test(t)) {
      // A play's own line ("- hosts:"): its tasks are two levels in.
      indent = /^\s*- (hosts|import_playbook|name:.*\bhosts\b)\s*:?/.test(t) ? indentOf(t) + 4 : indentOf(t);
      break;
    }
  }
  const pad = " ".repeat(indent);
  const text = task.trimEnd().split("\n").map((l) => (l ? pad + l : l)).join("\n") + "\n";

  // Where: on a blank cursor line, there; otherwise after the item the
  // cursor is in — before the next line indented at or above the list.
  let at: number;
  if (!cur.text.trim()) {
    at = cur.from;
  } else {
    at = doc.length;
    for (let n = cur.number + 1; n <= doc.lines; n++) {
      const t = doc.line(n).text;
      if (t.trim() && indentOf(t) <= indent) {
        at = doc.line(n).from;
        break;
      }
    }
  }
  const before = at > 0 && doc.sliceString(at - 1, at) !== "\n" ? "\n" : "";
  const insert = before + text;
  view.dispatch({ changes: { from: at, to: cur.text.trim() || at !== cur.from ? at : cur.to, insert }, selection: { anchor: at + before.length, head: at + insert.length }, scrollIntoView: true });
  view.focus();
}
