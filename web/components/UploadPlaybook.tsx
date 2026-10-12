// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import type { AnsibleFile, AnsiblePlaybook } from "@/lib/api";

const MAX_ARCHIVE = 10 << 20;
const MAX_FILE = 1 << 20;
const ARCHIVE = /\.(zip|tar|tar\.gz|tgz)$/i;

interface Picked {
  label: string; // what was picked, for the name and the summary
  archive?: { filename: string; base64: string };
  files?: AnsibleFile[];
  skipped: string[];
}

interface UploadResult {
  playbook?: AnsiblePlaybook;
  notes: string[];
  candidates?: string[];
  error?: string;
}

function toBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s);
}

function baseName(label: string) {
  return label.replace(ARCHIVE, "").replace(/\.(ya?ml)$/i, "");
}

// UploadPlaybook creates a playbook from an upload: a playbook file (or a
// few), a project folder, or an archive of one (.zip, .tar.gz, .tgz, .tar).
// The server unpacks archives, leaves out what can't be part of a playbook
// (saying what), and picks the file to run — or asks, when it can't tell.
export function UploadPlaybook() {
  const router = useRouter();
  const filesInput = useRef<HTMLInputElement | null>(null);
  const folderInput = useRef<HTMLInputElement | null>(null);
  const [picked, setPicked] = useState<Picked | null>(null);
  const [name, setName] = useState("");
  const [candidates, setCandidates] = useState<string[] | null>(null);
  const [entry, setEntry] = useState("");
  const [busy, setBusy] = useState(false);
  const [notes, setNotes] = useState<string[]>([]);

  async function pick(list: FileList | null, fromFolder: boolean) {
    if (!list || list.length === 0) return;
    const all = Array.from(list);
    setCandidates(null);
    setEntry("");
    setNotes([]);
    if (!fromFolder && all.length === 1 && ARCHIVE.test(all[0].name)) {
      const f = all[0];
      if (f.size > MAX_ARCHIVE) {
        toast.error(`${f.name} is over 10 MiB.`);
        return;
      }
      setPicked({ label: f.name, archive: { filename: f.name, base64: toBase64(await f.arrayBuffer()) }, skipped: [] });
      setName(baseName(f.name));
      return;
    }
    const files: AnsibleFile[] = [];
    const skipped: string[] = [];
    for (const f of all) {
      const p = fromFolder ? f.webkitRelativePath.split("/").slice(1).join("/") : f.name;
      if (!p) continue;
      if (f.size > MAX_FILE) {
        skipped.push(`${p} (over 1 MiB)`);
        continue;
      }
      const text = await f.text();
      if (text.includes("\u0000") || text.includes("�")) {
        skipped.push(`${p} (not a text file)`);
        continue;
      }
      files.push({ path: p, content: text });
    }
    const label = fromFolder ? (all[0].webkitRelativePath.split("/")[0] ?? "project") : all.length === 1 ? all[0].name : `${all.length} files`;
    setPicked({ label, files, skipped });
    setName(fromFolder || all.length === 1 ? baseName(label) : "Uploaded playbook");
  }

  async function upload() {
    if (!picked) return;
    setBusy(true);
    try {
      const res = await fetch("/api/ansible/playbooks/upload", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          entry,
          ...(picked.archive ? { filename: picked.archive.filename, archive: picked.archive.base64 } : { files: picked.files }),
        }),
      });
      const text = await res.text();
      let body: UploadResult;
      try {
        body = JSON.parse(text);
      } catch {
        toast.error(text || `Upload failed (HTTP ${res.status}).`, { duration: 10000 });
        return;
      }
      setNotes(body.notes ?? []);
      if (res.status === 409 && body.candidates?.length) {
        setCandidates(body.candidates);
        setEntry(body.candidates[0]);
        toast.message("Pick the playbook to run, then upload again.");
        return;
      }
      if (!res.ok || !body.playbook) {
        toast.error(body.error ?? `Upload failed (HTTP ${res.status}).`, { duration: 10000 });
        return;
      }
      const left = (body.notes?.length ?? 0) + picked.skipped.length;
      toast.success(`Uploaded ${body.playbook.name}: runs ${body.playbook.entry}${left ? `; ${left} file${left === 1 ? "" : "s"} left out` : ""}.`, { duration: 8000 });
      router.push(`/ansible/${body.playbook.id}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" onClick={() => filesInput.current?.click()}>
          ↑ Upload playbook or archive
        </Button>
        <Button variant="secondary" onClick={() => folderInput.current?.click()}>
          ↑ Upload folder
        </Button>
      </div>
      <input
        ref={filesInput}
        type="file"
        multiple
        hidden
        accept=".yml,.yaml,.zip,.tar,.gz,.tgz,.j2,.cfg,.ini,.json,.txt,.sh,.conf"
        onChange={(e) => {
          pick(e.target.files, false);
          e.target.value = "";
        }}
      />
      <input
        ref={(el) => {
          folderInput.current = el;
          el?.setAttribute("webkitdirectory", "");
        }}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          pick(e.target.files, true);
          e.target.value = "";
        }}
      />

      {picked && (
        <div className="card flex w-full flex-col gap-3 p-4 sm:p-5">
          <p className="text-sm text-text-primary">
            Upload <b className="font-mono">{picked.label}</b>
            {picked.files ? ` (${picked.files.length} file${picked.files.length === 1 ? "" : "s"})` : " — unpacked on the server"}
          </p>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className="flex flex-col gap-1 text-sm text-text-secondary">
              Name
              <input className="field" value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            {candidates && (
              <label className="flex flex-col gap-1 text-sm text-text-secondary">
                Playbook to run (several could be)
                <select className="field font-mono" value={entry} onChange={(e) => setEntry(e.target.value)}>
                  {candidates.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </div>
          {(picked.skipped.length > 0 || notes.length > 0) && (
            <ul className="flex flex-col gap-0.5 text-xs text-text-muted">
              {[...picked.skipped.map((s) => `${s}: left out`), ...notes].map((n) => (
                <li key={n}>• {n}</li>
              ))}
            </ul>
          )}
          <p className="text-xs text-text-muted">
            The file to run is found on its own: playbook.yml, site.yml or main.yml, else the only top-level YAML file that is
            a list of plays. A project&apos;s own ansible.cfg and inventory are left out — the panel builds them for each run
            from the servers you pick. Only text files are kept (1 MiB each, 8 MiB in all).
          </p>
          <div className="flex gap-2">
            <Button onClick={upload} disabled={busy || !name.trim()}>
              {busy ? "Uploading…" : "Upload"}
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setPicked(null);
                setCandidates(null);
                setNotes([]);
              }}
              disabled={busy}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
