// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import type { AnsiblePlaybook, AnsibleTemplate } from "@/lib/api";

// NewPlaybookForm creates a playbook from one of the starter templates.
export function NewPlaybookForm({ templates }: { templates: AnsibleTemplate[] }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  async function create(t: AnsibleTemplate) {
    setBusy(t.key);
    try {
      const res = await fetch("/api/ansible/playbooks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ template: t.key }),
      });
      if (!res.ok) {
        toast.error(await res.text());
        return;
      }
      const p: AnsiblePlaybook = await res.json();
      router.push(`/ansible/${p.id}`);
    } finally {
      setBusy(null);
    }
  }

  if (!open) return <Button onClick={() => setOpen(true)}>+ New playbook</Button>;
  return (
    <div className="card flex w-full flex-col gap-2 p-4 sm:p-5">
      <p className="text-sm font-semibold text-text-primary">Start from</p>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {templates.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => create(t)}
            disabled={busy !== null}
            className="flex flex-col gap-1 rounded-2xl bg-surface-2 p-3 text-left transition-colors hover:bg-surface-hover disabled:opacity-60"
          >
            <span className="text-sm font-bold text-text-primary">{busy === t.key ? "Creating…" : t.name}</span>
            <span className="text-xs text-text-secondary">{t.description}</span>
          </button>
        ))}
      </div>
      <Button variant="secondary" className="w-fit px-3 py-1 text-xs" onClick={() => setOpen(false)}>
        Cancel
      </Button>
    </div>
  );
}
