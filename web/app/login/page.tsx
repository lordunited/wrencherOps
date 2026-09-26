"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/Button";

export default function LoginPage() {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [pending, setPending] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setPending(true);
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      if (!res.ok) {
        toast.error("Invalid username or password.");
        return;
      }
      router.push("/");
      router.refresh();
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <form
        onSubmit={handleSubmit}
        className="flex w-full max-w-sm flex-col gap-4 rounded-xl border border-border bg-surface-1 p-8"
      >
        <div>
          <h1 className="text-lg font-semibold text-text-primary">DevOps Panel</h1>
          <p className="mt-1 text-sm text-text-secondary">Sign in to continue.</p>
        </div>
        <label className="flex flex-col gap-1 text-sm text-text-secondary">
          Username
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="rounded border border-border bg-background px-3 py-2 text-text-primary"
            autoComplete="username"
            required
          />
        </label>
        <label className="flex flex-col gap-1 text-sm text-text-secondary">
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="rounded border border-border bg-background px-3 py-2 text-text-primary"
            autoComplete="current-password"
            required
          />
        </label>
        <Button type="submit" disabled={pending} className="w-full">
          {pending ? "Signing in…" : "Sign in"}
        </Button>
      </form>
    </div>
  );
}
