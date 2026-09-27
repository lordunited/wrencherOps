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
    <div className="flex min-h-dvh items-center justify-center px-4 py-10">
      <form onSubmit={handleSubmit} className="card flex w-full max-w-sm flex-col gap-5 p-7 sm:p-8">
        <div className="flex flex-col gap-4">
          <span className="grid h-11 w-11 place-items-center rounded-2xl bg-accent text-on-accent shadow-card" aria-hidden>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round">
              <path d="M4 17l6-6 4 4 6-8" />
            </svg>
          </span>
          <div>
            <h1 className="page-title">Welcome back</h1>
            <p className="mt-1 text-sm text-text-secondary">Sign in to your DevOps Panel.</p>
          </div>
        </div>
        <label className="flex flex-col gap-1.5 text-sm font-medium text-text-secondary">
          Username
          <input
            id="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            className="field"
            autoComplete="username"
            autoCapitalize="none"
            required
          />
        </label>
        <label className="flex flex-col gap-1.5 text-sm font-medium text-text-secondary">
          Password
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="field"
            autoComplete="current-password"
            required
          />
        </label>
        <Button type="submit" disabled={pending} className="w-full py-2.5">
          {pending ? "Signing in…" : "Sign in"}
        </Button>
      </form>
    </div>
  );
}
