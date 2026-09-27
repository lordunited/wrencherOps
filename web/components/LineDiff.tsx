// Before/after view of a text file: changed lines with a few lines of
// context around them, like a unified diff.

type Op = { kind: "same" | "add" | "del"; text: string; oldNo?: number; newNo?: number };

const CONTEXT = 3;
// Past this many cells, the LCS table would be too big for the browser;
// fall back to showing old and new in full.
const MAX_CELLS = 4_000_000;

function diffLines(before: string, after: string): Op[] | null {
  const a = before === "" ? [] : before.replace(/\n$/, "").split("\n");
  const b = after === "" ? [] : after.replace(/\n$/, "").split("\n");
  if (a.length * b.length > MAX_CELLS) return null;

  // Longest common subsequence, computed from the end.
  const lcs: Uint32Array[] = Array.from({ length: a.length + 1 }, () => new Uint32Array(b.length + 1));
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const ops: Op[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      ops.push({ kind: "same", text: a[i], oldNo: i + 1, newNo: j + 1 });
      i++;
      j++;
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      ops.push({ kind: "del", text: a[i], oldNo: i + 1 });
      i++;
    } else {
      ops.push({ kind: "add", text: b[j], newNo: j + 1 });
      j++;
    }
  }
  while (i < a.length) ops.push({ kind: "del", text: a[i], oldNo: ++i });
  while (j < b.length) ops.push({ kind: "add", text: b[j], newNo: ++j });
  return ops;
}

export function LineDiff({ before, after }: { before: string; after: string }) {
  const ops = diffLines(before, after);
  if (!ops) {
    return <pre className="log-panel max-h-96 overflow-auto p-4">{after}</pre>;
  }

  // Keep changed lines plus CONTEXT lines around each; collapse the rest.
  const keep = new Array(ops.length).fill(false);
  ops.forEach((op, idx) => {
    if (op.kind === "same") return;
    for (let k = Math.max(0, idx - CONTEXT); k <= Math.min(ops.length - 1, idx + CONTEXT); k++) keep[k] = true;
  });
  const added = ops.filter((o) => o.kind === "add").length;
  const removed = ops.filter((o) => o.kind === "del").length;

  const rows: React.ReactNode[] = [];
  let skipped = 0;
  const flush = (key: string) => {
    if (skipped > 0) {
      rows.push(
        <div key={key} className="px-3 py-0.5 text-[11px] text-log-ink/50">
          ⋯ {skipped} unchanged line{skipped === 1 ? "" : "s"}
        </div>,
      );
      skipped = 0;
    }
  };
  ops.forEach((op, idx) => {
    if (!keep[idx]) {
      skipped++;
      return;
    }
    flush(`skip-${idx}`);
    const tone =
      op.kind === "add"
        ? "bg-[color-mix(in_srgb,var(--log-good)_16%,transparent)] text-log-good"
        : op.kind === "del"
          ? "bg-[color-mix(in_srgb,var(--status-critical)_18%,transparent)] text-[#ff9aa5]"
          : "";
    rows.push(
      <div key={idx} className={`flex gap-3 px-3 ${tone}`}>
        <span className="w-8 shrink-0 select-none text-right text-log-ink/40">{op.newNo ?? op.oldNo}</span>
        <span className="w-3 shrink-0 select-none">{op.kind === "add" ? "+" : op.kind === "del" ? "−" : " "}</span>
        <span className="whitespace-pre-wrap break-all">{op.text || " "}</span>
      </div>,
    );
  });
  flush("skip-end");

  return (
    <div className="flex flex-col gap-2">
      <p className="text-xs font-semibold text-text-secondary">
        <span className="text-status-good">+{added}</span>{" "}
        <span className="text-status-critical">−{removed}</span> lines
      </p>
      <div className="log-panel max-h-[28rem] overflow-auto py-2">
        {added + removed === 0 ? <div className="px-3">No changes.</div> : rows}
      </div>
    </div>
  );
}
