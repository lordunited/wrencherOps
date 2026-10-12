// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { LinkButton } from "@/components/Button";

export default function NotFound() {
  return (
    <div className="card flex flex-col items-start gap-3 p-6 sm:p-8">
      <span className="grid h-11 w-11 place-items-center rounded-2xl bg-accent-soft text-lg font-extrabold text-accent" aria-hidden>
        ?
      </span>
      <h1 className="page-title">This page no longer exists</h1>
      <p className="max-w-md text-sm text-text-secondary">
        It may have been deleted, for example a service that was removed. Pick it again from the list.
      </p>
      <div className="flex flex-wrap gap-2">
        <LinkButton href="/">Back to overview</LinkButton>
        <LinkButton href="/deploy/gitlab" variant="secondary">
          Go to Deploy
        </LinkButton>
      </div>
    </div>
  );
}
