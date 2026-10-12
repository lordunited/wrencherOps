// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import type { WatchedServer } from "@/lib/api";

const LABEL = "flex flex-col gap-1 text-sm text-text-secondary";

// Adds a server to watch by its SSH login. The password is checked by
// logging in before it's saved; it can be left empty when the panel already
// has one saved for that login.
export function AddServerForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", host: "", port: "22", username: "root", password: "" });
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      const res = await fetch("/api/servers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...form, port: Number(form.port) || 22 }),
      });
      if (!res.ok) {
        toast.error(await res.text(), { duration: 10000 });
        return;
      }
      const s: WatchedServer = await res.json();
      toast.success(`Watching ${s.name}.`);
      router.push(`/servers/${s.id}`);
    } finally {
      setBusy(false);
    }
  }

  if (!open) {
    return <Button onClick={() => setOpen(true)}>+ Add server</Button>;
  }
  return (
    <form onSubmit={submit} className="card flex w-full flex-col gap-3 p-4 sm:p-5">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-5">
        <label className={`${LABEL} sm:col-span-2`}>
          Host
          <input value={form.host} onChange={(e) => setForm({ ...form, host: e.target.value })} placeholder="203.0.113.10" className="field font-mono" required autoFocus />
        </label>
        <label className={LABEL}>
          SSH port
          <input value={form.port} onChange={(e) => setForm({ ...form, port: e.target.value })} inputMode="numeric" className="field" />
        </label>
        <label className={LABEL}>
          SSH user
          <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} className="field" required />
        </label>
        <label className={LABEL}>
          Password
          <input
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            className="field"
            autoComplete="off"
            placeholder="empty if already saved"
          />
        </label>
        <label className={`${LABEL} sm:col-span-2`}>
          Name (optional)
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="defaults to the host" className="field" />
        </label>
      </div>
      <p className="text-xs text-text-muted">
        The panel logs in to check the password before saving it, then reads the server&apos;s state every minute. Reading
        failed logins and the kernel log needs root: log in as root, or as a user that can sudo (with this password).
      </p>
      <div className="flex gap-2">
        <Button type="submit" disabled={busy || !form.host.trim()}>
          {busy ? "Connecting…" : "Add and check"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
