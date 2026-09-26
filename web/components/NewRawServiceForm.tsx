"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";
import { YamlEditor } from "@/components/YamlEditor";

const FIELD_CLASS =
  "rounded-md border border-border bg-background px-3 py-2 text-sm text-text-primary";
const LABEL_CLASS = "flex flex-col gap-1 text-sm text-text-secondary";

export function NewRawServiceForm({
  title,
  description,
  endpoint,
  basePath,
  backLabel,
  manifestLabel,
  manifestPlaceholder,
  defaultDeployDir,
}: {
  title: string;
  description: string;
  endpoint: string;
  basePath: string;
  backLabel: string;
  manifestLabel: string;
  manifestPlaceholder: string;
  defaultDeployDir: string;
}) {
  const router = useRouter();

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [sshPort, setSshPort] = useState("22");
  const [username, setUsername] = useState("root");
  const [deployDir, setDeployDir] = useState(defaultDeployDir);
  const [manifest, setManifest] = useState(manifestPlaceholder);

  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);

    try {
      const res = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          host,
          port: Number(sshPort) || undefined,
          username,
          deployDir,
          manifest,
        }),
      });

      if (!res.ok) {
        toast.error(await res.text());
        return;
      }

      const service = await res.json();
      router.push(`${basePath}/${service.id}`);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex flex-col gap-8">
      <Link href={basePath} className="text-sm text-text-secondary hover:text-text-primary">
        ← {backLabel}
      </Link>
      <div>
        <h1 className="text-xl font-semibold text-text-primary">{title}</h1>
        <p className="mt-1 text-sm text-text-secondary">{description}</p>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-8">
        <fieldset className="flex flex-col gap-4" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">Target server</legend>
          <div className="grid grid-cols-2 gap-4">
            <label className={LABEL_CLASS}>
              Name
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={LABEL_CLASS}>
              Host
              <input
                value={host}
                onChange={(e) => setHost(e.target.value)}
                placeholder="203.0.113.10"
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={LABEL_CLASS}>
              SSH port
              <input
                value={sshPort}
                onChange={(e) => setSshPort(e.target.value)}
                inputMode="numeric"
                className={FIELD_CLASS}
              />
            </label>
            <label className={LABEL_CLASS}>
              Username
              <input
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                className={FIELD_CLASS}
                required
              />
            </label>
            <label className={`${LABEL_CLASS} col-span-2`}>
              Working directory (where the file is written on the target)
              <input
                value={deployDir}
                onChange={(e) => setDeployDir(e.target.value)}
                className={FIELD_CLASS}
              />
            </label>
          </div>
          <p className="text-xs text-text-muted">
            No password here — you&apos;ll enter it when you actually deploy.
          </p>
        </fieldset>

        <fieldset className="flex flex-col gap-3" disabled={submitting}>
          <legend className="text-sm font-semibold text-text-primary">{manifestLabel}</legend>
          <YamlEditor value={manifest} onChange={setManifest} />
        </fieldset>

        <Button type="submit" disabled={submitting} className="w-fit">
          {submitting ? "Creating…" : "Create service"}
        </Button>
      </form>
    </div>
  );
}
