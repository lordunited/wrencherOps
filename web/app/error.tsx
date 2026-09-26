"use client";

import { useEffect } from "react";
import { Button } from "@/components/Button";

// Defensive fallback for errors above the panel layout itself (e.g. the
// layout's own session check failing outright, rather than redirecting).
// Rare in practice — most auth failures redirect to /login instead of
// throwing — but this keeps that case calm rather than showing Next's
// default crash screen.
export default function RootError({
  error,
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background px-4 text-center">
      <h2 className="text-lg font-semibold text-text-primary">Something went wrong</h2>
      <p className="max-w-md text-sm text-text-secondary">
        {error.message || "An unexpected error occurred."}
      </p>
      {error.digest && <p className="text-xs text-text-muted">Reference: {error.digest}</p>}
      <Button variant="secondary" onClick={() => retry()}>
        Try again
      </Button>
    </div>
  );
}
