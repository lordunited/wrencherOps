import { LogoutButton } from "@/components/LogoutButton";
import { Sidebar } from "@/components/Sidebar";
import { getMe } from "@/lib/api";

export default async function PanelLayout({ children }: { children: React.ReactNode }) {
  const me = await getMe();

  return (
    <div className="flex min-h-full">
      <Sidebar />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-end gap-3 border-b border-border px-8 py-3 text-sm text-text-secondary">
          <span>{me.username}</span>
          <LogoutButton />
        </header>
        <main className="min-w-0 flex-1 px-8 py-8">
          <div className="mx-auto max-w-4xl">{children}</div>
        </main>
      </div>
    </div>
  );
}
