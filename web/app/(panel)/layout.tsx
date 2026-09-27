import { Brand } from "@/components/Brand";
import { LogoutButton } from "@/components/LogoutButton";
import { MobileNav } from "@/components/MobileNav";
import { Sidebar } from "@/components/Sidebar";
import { getMe } from "@/lib/api";

export default async function PanelLayout({ children }: { children: React.ReactNode }) {
  const me = await getMe();

  return (
    <div className="flex min-h-dvh">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex items-center justify-between gap-3 bg-background/85 px-4 py-3 pt-[calc(0.75rem+env(safe-area-inset-top,0px))] backdrop-blur-md sm:px-8 md:justify-end">
          <div className="md:hidden">
            <Brand />
          </div>
          <div className="flex items-center gap-1">
            <span className="flex items-center gap-2 rounded-full bg-surface-1 py-1 pl-1 pr-3 text-sm font-semibold text-text-primary shadow-card">
              <span className="grid h-7 w-7 place-items-center rounded-full bg-accent-soft text-xs font-bold uppercase text-accent" aria-hidden>
                {me.username.slice(0, 1)}
              </span>
              {me.username}
            </span>
            <LogoutButton />
          </div>
        </header>
        <main className="min-w-0 flex-1 px-4 pb-32 pt-2 sm:px-8 md:pb-12">
          <div className="mx-auto max-w-5xl">{children}</div>
        </main>
      </div>
      <MobileNav />
    </div>
  );
}
