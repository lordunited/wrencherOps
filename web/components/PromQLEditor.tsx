"use client";

import { useEffect, useMemo, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView, placeholder as placeholderExt } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { PromQLExtension } from "@prometheus-io/codemirror-promql";

// PromQL colors from the panel's own tokens, so the editor reads well in
// both light and dark mode.
const promqlHighlight = HighlightStyle.define([
  { tag: tags.function(tags.variableName), color: "var(--accent)", fontWeight: "600" },
  { tag: tags.operatorKeyword, color: "var(--accent)", fontWeight: "600" },
  { tag: [tags.modifier, tags.logicOperator], color: "var(--status-serious)", fontWeight: "600" },
  { tag: tags.labelName, color: "var(--status-warning)" },
  { tag: tags.string, color: "var(--status-good)" },
  { tag: tags.number, color: "var(--status-serious)" },
  { tag: [tags.operator, tags.arithmeticOperator], color: "var(--text-secondary)" },
  { tag: tags.comment, color: "var(--text-muted)", fontStyle: "italic" },
  { tag: tags.invalid, color: "var(--status-critical)" },
]);

// Layout only; colors come from CSS variables in globals.css
// (.promql-editor), which follow the light/dark theme.
const layout = EditorView.theme({
  "&": { fontSize: "13px" },
  ".cm-content": { padding: "10px 4px", minHeight: "64px" },
  ".cm-line": { padding: "0 10px" },
  "&.cm-focused": { outline: "none" },
});

type Check =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "invalid"; error: string }
  | { state: "unavailable"; error: string }
  | { state: "signed-out" }
  | { state: "valid"; kind: string; count: number; sample: { labels: string; value: string }[] };

interface QueryResponse {
  status: "success" | "error";
  error?: string;
  data?: {
    resultType: "vector" | "matrix" | "scalar" | "string";
    result: unknown;
  };
}

// A readable one-line reason from a non-JSON error response: never a page
// of HTML.
function shortError(text: string, status: number) {
  const t = text.trim();
  if (!t || t.startsWith("<")) {
    return status === 404
      ? "the panel's backend doesn't have this feature yet. Restart it with the latest build."
      : `the panel answered with HTTP ${status}.`;
  }
  return t.length > 200 ? `${t.slice(0, 200)}…` : t;
}

function formatLabels(metric: Record<string, string>) {
  const name = metric.__name__ ?? "";
  const rest = Object.entries(metric)
    .filter(([k]) => k !== "__name__")
    .map(([k, v]) => `${k}="${v}"`)
    .join(", ");
  return `${name}{${rest}}`;
}

// A PromQL editor wired to the service's own Prometheus: suggestions for
// metric names, labels, label values and functions as you type, inline
// syntax errors, and a live check that runs the query and says whether it
// is valid and what it matches right now.
export function PromQLEditor({
  serviceId,
  value,
  onChange,
  id,
}: {
  serviceId: string;
  value: string;
  onChange: (value: string) => void;
  id?: string;
}) {
  const extensions = useMemo(
    () => [
      new PromQLExtension()
        .setComplete({
          remote: {
            url: `/api/services/${serviceId}/prometheus`,
            httpMethod: "GET",
            cache: { maxAge: 5 * 60 * 1000 },
          },
          maxMetricsMetadata: 20000,
        })
        .asExtension(),
      syntaxHighlighting(promqlHighlight),
      layout,
      EditorView.lineWrapping,
      placeholderExt("up == 0"),
      EditorView.contentAttributes.of(id ? { id, "aria-label": "PromQL condition" } : { "aria-label": "PromQL condition" }),
    ],
    [serviceId, id],
  );

  const [check, setCheck] = useState<Check>({ state: "idle" });

  // Run the query against Prometheus shortly after typing stops.
  useEffect(() => {
    const query = value.trim();
    const controller = new AbortController();
    const timer = setTimeout(async () => {
      if (!query) {
        setCheck({ state: "idle" });
        return;
      }
      setCheck({ state: "checking" });
      try {
        const res = await fetch(
          `/api/services/${serviceId}/prometheus/api/v1/query?query=${encodeURIComponent(query)}`,
          { signal: controller.signal, cache: "no-store" },
        );
        if (res.status === 401) {
          setCheck({ state: "signed-out" });
          return;
        }
        const text = await res.text();
        let body: QueryResponse | null = null;
        try {
          body = JSON.parse(text);
        } catch {
          body = null;
        }
        if (body?.status === "error" && (res.status === 400 || res.status === 422)) {
          setCheck({ state: "invalid", error: body.error ?? "Prometheus rejected the query." });
          return;
        }
        if (!res.ok || !body || body.status !== "success" || !body.data) {
          setCheck({ state: "unavailable", error: body?.error ?? shortError(text, res.status) });
          return;
        }
        const { resultType, result } = body.data;
        if (resultType === "vector" || resultType === "matrix") {
          const series = result as { metric: Record<string, string>; value?: [number, string] }[];
          setCheck({
            state: "valid",
            kind: resultType,
            count: series.length,
            sample: series.slice(0, 5).map((s) => ({ labels: formatLabels(s.metric), value: s.value?.[1] ?? "" })),
          });
        } else {
          const [, v] = result as [number, string];
          setCheck({ state: "valid", kind: resultType, count: -1, sample: [{ labels: resultType, value: String(v) }] });
        }
      } catch (e) {
        if ((e as Error).name === "AbortError") return;
        setCheck({ state: "unavailable", error: (e as Error).message });
      }
    }, 700);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [value, serviceId]);

  return (
    <div className="flex flex-col gap-2">
      <div className="promql-editor">
        <CodeMirror
          value={value}
          onChange={onChange}
          theme="none"
          extensions={extensions}
          basicSetup={{
            lineNumbers: false,
            foldGutter: false,
            highlightActiveLine: false,
            highlightActiveLineGutter: false,
            autocompletion: true,
            closeBrackets: true,
            bracketMatching: true,
            syntaxHighlighting: false,
          }}
        />
      </div>
      <CheckLine check={check} />
    </div>
  );
}

function CheckLine({ check }: { check: Check }) {
  if (check.state === "idle") {
    return (
      <p className="text-xs text-text-muted">
        Start typing for suggestions: metric names, labels, their values and functions. Press Ctrl+Space to open them.
      </p>
    );
  }
  if (check.state === "checking") {
    return <p className="text-xs text-text-muted">Checking against Prometheus…</p>;
  }
  if (check.state === "invalid") {
    return (
      <p className="rounded-xl bg-[color-mix(in_srgb,var(--status-critical)_12%,transparent)] px-3 py-2 text-xs text-status-critical">
        <span className="font-bold">Not valid PromQL.</span> {check.error}
      </p>
    );
  }
  if (check.state === "signed-out") {
    return (
      <p className="rounded-xl bg-[color-mix(in_srgb,var(--status-warning)_14%,transparent)] px-3 py-2 text-xs text-text-primary">
        <span className="font-bold">Your session has expired.</span>{" "}
        <a href="/login" className="font-semibold text-accent underline">
          Sign in again
        </a>{" "}
        to get suggestions and query checks.
      </p>
    );
  }
  if (check.state === "unavailable") {
    return (
      <p className="text-xs text-text-muted">
        Couldn&apos;t reach Prometheus to check this query: {check.error}
      </p>
    );
  }
  if (check.count === -1) {
    return (
      <p className="rounded-xl bg-[color-mix(in_srgb,var(--status-warning)_14%,transparent)] px-3 py-2 text-xs text-text-primary">
        <span className="font-bold">Valid, but it returns a single {check.kind} ({check.sample[0]?.value}).</span> An alert
        condition should return series, for example <code className="font-mono">up == 0</code>.
      </p>
    );
  }
  if (check.count === 0) {
    return (
      <p className="rounded-xl bg-[color-mix(in_srgb,var(--status-good)_12%,transparent)] px-3 py-2 text-xs text-text-primary">
        <span className="font-bold text-status-good">Valid.</span> It matches nothing right now, so this alert would not
        fire at the moment.
      </p>
    );
  }
  return (
    <div className="flex flex-col gap-1.5 rounded-xl bg-[color-mix(in_srgb,var(--status-warning)_14%,transparent)] px-3 py-2 text-xs text-text-primary">
      <p>
        <span className="font-bold text-status-good">Valid.</span> It matches{" "}
        <span className="font-bold">
          {check.count} series
        </span>{" "}
        right now, so this alert would fire for {check.count === 1 ? "it" : "each of them"} once the &ldquo;for&rdquo;
        time has passed.
      </p>
      <ul className="flex flex-col gap-0.5 font-mono text-[11px] text-text-secondary">
        {check.sample.map((s, i) => (
          <li key={i} className="break-all">
            {s.labels} <span className="text-text-muted">= {s.value}</span>
          </li>
        ))}
        {check.count > check.sample.length && <li className="text-text-muted">…and {check.count - check.sample.length} more</li>}
      </ul>
    </div>
  );
}
