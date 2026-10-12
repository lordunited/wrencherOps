// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import type { TfFile, TfTemplate, TfWorkspace } from "@/lib/api";

const ARCHIVE = /\.(zip|tar|tar\.gz|tgz)$/i;

function toBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

// TfNewWorkspace starts a workspace: from a template, or by uploading
// .tf files, a folder or an archive of one (a terraform.tfstate in it is
// imported as its state).
export function TfNewWorkspace({ templates }: { templates: TfTemplate[] }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const filesInput = useRef<HTMLInputElement | null>(null);
  const folderInput = useRef<HTMLInputElement | null>(null);

  async function create(t: TfTemplate) {
    setBusy(t.key);
    try {
      const res = await fetch("/api/terraform/workspaces", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ template: t.key }) });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const w: TfWorkspace = await res.json();
      router.push(`/terraform/${w.id}`);
    } finally {
      setBusy(null);
    }
  }

  async function upload(list: FileList | null, fromFolder: boolean) {
    if (!list || list.length === 0) return;
    const all = Array.from(list);
    let body: object;
    if (!fromFolder && all.length === 1 && ARCHIVE.test(all[0].name)) {
      if (all[0].size > 10 << 20) {
        toast.error("The archive is over 10 MiB.");
        return;
      }
      body = { filename: all[0].name, archive: toBase64(await all[0].arrayBuffer()) };
    } else {
      const files: TfFile[] = [];
      for (const f of all) {
        const p = fromFolder ? f.webkitRelativePath.split("/").slice(1).join("/") : f.name;
        if (!p || f.size > 1 << 20) continue;
        const text = await f.text();
        if (!text.includes("\u0000")) files.push({ path: p, content: text });
      }
      body = { files, filename: fromFolder ? (all[0].webkitRelativePath.split("/")[0] ?? "") : all[0].name };
    }
    setBusy("upload");
    try {
      const res = await fetch("/api/terraform/workspaces/upload", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
      const text = await res.text();
      let r: { workspace?: TfWorkspace; notes?: string[]; error?: string };
      try {
        r = JSON.parse(text);
      } catch {
        toast.error(text || `Upload failed (HTTP ${res.status}).`, { duration: 10000 });
        return;
      }
      for (const n of r.notes ?? []) toast.message(n, { duration: 12000 });
      if (!res.ok || !r.workspace) {
        toast.error(r.error ?? "Upload failed.", { duration: 10000 });
        return;
      }
      toast.success(`Uploaded ${r.workspace.name}.`);
      router.push(`/terraform/${r.workspace.id}`);
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" onClick={() => filesInput.current?.click()} disabled={busy !== null}>
          {busy === "upload" ? "Uploading…" : "↑ Upload .tf files or archive"}
        </Button>
        <Button variant="secondary" onClick={() => folderInput.current?.click()} disabled={busy !== null}>
          ↑ Upload folder
        </Button>
        <Button onClick={() => setOpen(!open)}>+ New workspace</Button>
      </div>
      <input ref={filesInput} type="file" multiple hidden accept=".tf,.tfvars,.hcl,.json,.zip,.tar,.gz,.tgz,.tpl,.sh,.yaml,.yml" onChange={(e) => (upload(e.target.files, false), (e.target.value = ""))} />
      <input
        ref={(el) => {
          folderInput.current = el;
          el?.setAttribute("webkitdirectory", "");
        }}
        type="file"
        multiple
        hidden
        onChange={(e) => (upload(e.target.files, true), (e.target.value = ""))}
      />
      {open && (
        <div className="card flex w-full flex-col gap-2 p-4 sm:p-5">
          <p className="text-sm font-semibold text-text-primary">Start from</p>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            {templates.map((t) => (
              <button key={t.key} type="button" onClick={() => create(t)} disabled={busy !== null} className="flex flex-col gap-1 rounded-2xl bg-surface-2 p-3 text-left transition-colors hover:bg-surface-hover disabled:opacity-60">
                <span className="text-sm font-bold text-text-primary">{busy === t.key ? "Creating…" : t.name}</span>
                <span className="text-xs text-text-secondary">{t.description}</span>
              </button>
            ))}
          </div>
        </div>
      )}
    </>
  );
}
