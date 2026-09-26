"use client";

import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { EditorView } from "@codemirror/view";

export function YamlEditor({
  value,
  onChange,
  readOnly,
}: {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <CodeMirror
        value={value}
        onChange={onChange}
        readOnly={readOnly}
        theme="dark"
        extensions={[yaml(), EditorView.lineWrapping]}
        basicSetup={{ foldGutter: true, highlightActiveLine: !readOnly }}
        style={{ fontSize: "13px" }}
      />
    </div>
  );
}
