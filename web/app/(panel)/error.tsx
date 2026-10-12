// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useEffect } from "react";
import { Button } from "@/components/Button";

// Catches a page's data-fetch failures without taking over the whole
// screen — this renders inside the panel layout, so the sidebar and header
// stay intact and usable while only the content area shows this.
export default function PanelError({
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
    <div className="card flex flex-col items-start gap-3 p-6">
      <h2 className="section-title">Couldn&apos;t load this page</h2>
      <p className="max-w-md text-sm text-text-secondary">
        {error.message || "Something went wrong loading this data."}
      </p>
      {error.digest && <p className="text-xs text-text-muted">Reference: {error.digest}</p>}
      <Button variant="secondary" onClick={() => retry()}>
        Try again
      </Button>
    </div>
  );
}
