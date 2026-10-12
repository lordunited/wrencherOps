// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { StreamLanguage } from "@codemirror/language";
import { nginx } from "@codemirror/legacy-modes/mode/nginx";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { EditorView } from "@codemirror/view";
import { hcl } from "@/lib/hclMode";

const LANGUAGES = {
  yaml: () => yaml(),
  nginx: () => StreamLanguage.define(nginx),
  // No dedicated HAProxy mode exists; "properties" gives # comments their
  // own color and leaves keywords alone, which reads fine for its
  // flat "keyword value..." config lines.
  haproxy: () => StreamLanguage.define(properties),
  // KEY=VALUE with # comments: exactly what "properties" highlights.
  env: () => StreamLanguage.define(properties),
  // Terraform/OpenTofu (.tf, .tfvars, .hcl).
  hcl: () => StreamLanguage.define(hcl),
  text: () => [],
};

// languageFor picks a highlighting mode from a file's name.
export function languageFor(path: string): keyof typeof LANGUAGES {
  const name = path.split("/").pop() ?? "";
  if (/\.ya?ml$/i.test(name)) return "yaml";
  if (/\.(tf|tfvars|hcl)$/i.test(name)) return "hcl";
  if (name === ".env" || name.startsWith(".env.") || /\.env$/i.test(name)) return "env";
  if (/\.(conf|cfg|ini|properties|toml)$/i.test(name)) return "env";
  return "text";
}

// A code editor for config files: YAML (compose files, Prometheus),
// Nginx or HAProxy configuration.
export function YamlEditor({
  value,
  onChange,
  readOnly,
  language = "yaml",
  onEditor,
}: {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  language?: keyof typeof LANGUAGES;
  // onEditor hands over the editor once it's created, for inserting text at
  // the cursor (the Ansible module picker does).
  onEditor?: (view: EditorView) => void;
}) {
  return (
    <div className="overflow-hidden rounded-2xl">
      <CodeMirror
        value={value}
        onChange={onChange}
        readOnly={readOnly}
        theme="dark"
        extensions={[LANGUAGES[language](), EditorView.lineWrapping]}
        basicSetup={{ foldGutter: true, highlightActiveLine: !readOnly }}
        maxHeight="480px"
        onCreateEditor={onEditor}
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}
