import Link from "next/link";

export function Brand() {
  return (
    <Link href="/" className="flex items-center gap-2.5 rounded-xl">
      <span className="grid h-8 w-8 place-items-center rounded-xl bg-accent text-on-accent" aria-hidden>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.25" strokeLinecap="round" strokeLinejoin="round">
          <path d="M4 17l6-6 4 4 6-8" />
        </svg>
      </span>
      <span className="text-[15px] font-extrabold tracking-tight text-text-primary">DevOps Panel</span>
    </Link>
  );
}
