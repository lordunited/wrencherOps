// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

export const AUTHOR = {
  name: "Mohammadreza Masoumi",
  title: "DevOps Engineer",
  email: "s.masoumimohammadreza@gmail.com",
};

// AuthorCredit is the panel's byline: who built it, with a mail link.
export function AuthorCredit({ className = "" }: { className?: string }) {
  return (
    <p className={`text-[11px] leading-relaxed text-text-muted ${className}`}>
      Built by <span className="font-semibold text-text-secondary">{AUTHOR.name}</span>
      <br />
      {AUTHOR.title} ·{" "}
      <a href={`mailto:${AUTHOR.email}`} className="hover:text-accent hover:underline">
        {AUTHOR.email}
      </a>
    </p>
  );
}
