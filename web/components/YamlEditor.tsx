"use client";

import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { StreamLanguage } from "@codemirror/language";
import { nginx } from "@codemirror/legacy-modes/mode/nginx";
import { EditorView } from "@codemirror/view";

const LANGUAGES = {
  yaml: () => yaml(),
  nginx: () => StreamLanguage.define(nginx),
};

// A code editor for config files: YAML (compose files, Prometheus) or
// Nginx configuration.
export function YamlEditor({
  value,
  onChange,
  readOnly,
  language = "yaml",
}: {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  language?: keyof typeof LANGUAGES;
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
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}
