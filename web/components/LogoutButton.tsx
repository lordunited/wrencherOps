"use client";

import { useRouter } from "next/navigation";
import { Icon } from "@/components/icons";

export function LogoutButton() {
  const router = useRouter();

  async function handleLogout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.push("/login");
    router.refresh();
  }

  return (
    <button
      onClick={handleLogout}
      className="flex items-center gap-1.5 rounded-full px-3 py-1.5 text-sm font-semibold text-text-secondary transition-colors hover:bg-surface-2 hover:text-text-primary"
    >
      <Icon name="logout" />
      <span className="hidden sm:inline">Log out</span>
      <span className="sr-only sm:hidden">Log out</span>
    </button>
  );
}
