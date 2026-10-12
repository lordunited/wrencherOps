// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { AccessMatrix } from "@/components/AccessMatrix";
import { useMe } from "@/components/Access";
import { Button } from "@/components/Button";
import type { PanelGroup, PanelTab, PanelUserDetail, Role } from "@/lib/api";
import type { TabAccess } from "@/lib/nav";

const LABEL = "flex flex-col gap-1 text-sm text-text-secondary";

export const ROLE_INFO: Record<Role, string> = {
  admin: "Everything, in every tab, and manages users and access.",
  writer: "Exactly the access given below: view or write, per tab.",
  viewer: "Can look, never change: at most view, whatever is given.",
};

async function send(method: string, path: string, body?: unknown): Promise<Response | null> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    toast.error((await res.text()) || `${res.status}`, { duration: 10000 });
    return null;
  }
  return res;
}

function RolePicker({ value, onChange, disabled }: { value: Role; onChange: (r: Role) => void; disabled?: boolean }) {
  return (
    <div role="radiogroup" aria-label="Role" className="grid gap-2 sm:grid-cols-3">
      {(Object.keys(ROLE_INFO) as Role[]).map((r) => (
        <button
          key={r}
          type="button"
          role="radio"
          aria-checked={value === r}
          disabled={disabled}
          onClick={() => onChange(r)}
          className={`flex flex-col items-start gap-0.5 rounded-2xl p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
            value === r ? "bg-accent-soft ring-2 ring-accent" : "bg-surface-2 hover:bg-surface-hover"
          }`}
        >
          <span className="text-sm font-bold capitalize text-text-primary">{r}</span>
          <span className="text-xs text-text-secondary">{ROLE_INFO[r]}</span>
        </button>
      ))}
    </div>
  );
}

function GroupPicker({ groups, value, onChange }: { groups: PanelGroup[]; value: string[]; onChange: (ids: string[]) => void }) {
  if (groups.length === 0) return <p className="text-xs text-text-muted">No groups yet.</p>;
  return (
    <div className="flex flex-wrap gap-2">
      {groups.map((g) => {
        const on = value.includes(g.id);
        return (
          <label key={g.id} className={`flex cursor-pointer items-center gap-2 rounded-full px-3 py-1.5 text-sm ${on ? "bg-accent-soft text-accent" : "bg-surface-2 text-text-secondary"}`}>
            <input type="checkbox" checked={on} onChange={() => onChange(on ? value.filter((x) => x !== g.id) : [...value, g.id])} />
            {g.name}
          </label>
        );
      })}
    </div>
  );
}

// NewUserForm adds a panel user: a name, a first password and a role.
export function NewUserForm({ groups }: { groups: PanelGroup[] }) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ username: "", password: "", role: "viewer" as Role, groups: [] as string[] });
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await send("POST", "/api/admin/users", form);
      if (!res) return;
      toast.success(`${form.username} can sign in now. Next: give them access to tabs.`);
      router.push(`/admin/users/${encodeURIComponent(form.username.trim().toLowerCase())}`);
    } finally {
      setBusy(false);
    }
  }

  if (!open) return <Button onClick={() => setOpen(true)}>+ New user</Button>;
  return (
    <form onSubmit={submit} className="card flex w-full flex-col gap-4 p-4 sm:p-5">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className={LABEL}>
          Username
          <input
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.target.value })}
            className="field font-mono"
            placeholder="e.g. sara.ops"
            autoComplete="off"
            required
            autoFocus
          />
        </label>
        <label className={LABEL}>
          First password
          <input
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            className="field"
            autoComplete="new-password"
            minLength={10}
            required
          />
          <span className="text-xs text-text-muted">At least 10 characters. They can change it in their account.</span>
        </label>
      </div>
      <RolePicker value={form.role} onChange={(role) => setForm({ ...form, role })} />
      <div className="flex flex-col gap-1.5">
        <span className="text-sm text-text-secondary">Groups</span>
        <GroupPicker groups={groups} value={form.groups} onChange={(ids) => setForm({ ...form, groups: ids })} />
      </div>
      <div className="flex gap-2">
        <Button type="submit" disabled={busy || !form.username.trim() || form.password.length < 10}>
          {busy ? "Creating…" : "Create user"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

// NewGroupForm adds a group.
export function NewGroupForm() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState({ name: "", description: "" });
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await send("POST", "/api/admin/groups", form);
      if (!res) return;
      const g: PanelGroup = await res.json();
      toast.success(`Group ${g.name} created. Next: its members and access.`);
      router.push(`/admin/groups/${encodeURIComponent(g.id)}`);
    } finally {
      setBusy(false);
    }
  }

  if (!open)
    return (
      <Button variant="secondary" onClick={() => setOpen(true)}>
        + New group
      </Button>
    );
  return (
    <form onSubmit={submit} className="card flex w-full flex-col gap-3 p-4 sm:p-5">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className={LABEL}>
          Name
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="field" placeholder="e.g. Infra team" required autoFocus />
        </label>
        <label className={LABEL}>
          Description (optional)
          <input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="field" />
        </label>
      </div>
      <div className="flex gap-2">
        <Button type="submit" disabled={busy || !form.name.trim()}>
          {busy ? "Creating…" : "Create group"}
        </Button>
        <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

// UserEditor is one user: role, groups, enabled, password, access per tab.
export function UserEditor({ initial, tabs, groups }: { initial: PanelUserDetail; tabs: PanelTab[]; groups: PanelGroup[] }) {
  const router = useRouter();
  const me = useMe();
  const [user, setUser] = useState(initial);
  const [role, setRole] = useState<Role>(initial.role);
  const [disabled, setDisabled] = useState(initial.disabled);
  const [memberOf, setMemberOf] = useState(initial.groups);
  const [grants, setGrants] = useState<TabAccess>(initial.grants);
  const [password, setPassword] = useState("");
  const [confirmDelete, setConfirmDelete] = useState("");
  const [busy, setBusy] = useState(false);
  const self = me?.username === user.username;

  const accountDirty = role !== user.role || disabled !== user.disabled || memberOf.slice().sort().join() !== user.groups.slice().sort().join();
  const grantsDirty = JSON.stringify(Object.entries(grants).sort()) !== JSON.stringify(Object.entries(user.grants).sort());

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    try {
      await fn();
    } finally {
      setBusy(false);
    }
  }

  const saveAccount = () =>
    run(async () => {
      const res = await send("PUT", `/api/admin/users/${encodeURIComponent(user.username)}`, { role, disabled, groups: memberOf });
      if (!res) return;
      const u: PanelUserDetail = await res.json();
      setUser(u);
      toast.success(disabled && !user.disabled ? `${u.username} is disabled and signed out.` : "Saved. It applies on their next click.");
      router.refresh();
    });

  const saveGrants = () =>
    run(async () => {
      const res = await send("PUT", `/api/admin/users/${encodeURIComponent(user.username)}/grants`, { grants });
      if (!res) return;
      const u: PanelUserDetail = await res.json();
      setUser(u);
      setGrants(u.grants);
      toast.success("Access saved. It applies on their next click.");
    });

  const resetPassword = () =>
    run(async () => {
      const res = await send("PUT", `/api/admin/users/${encodeURIComponent(user.username)}/password`, { password });
      if (!res) return;
      setPassword("");
      toast.success(`${user.username}'s password is changed, and they're signed out everywhere.`);
    });

  const remove = () =>
    run(async () => {
      const res = await send("DELETE", `/api/admin/users/${encodeURIComponent(user.username)}`);
      if (!res) return;
      toast.success(`${user.username} is deleted.`);
      router.push("/admin");
      router.refresh();
    });

  if (user.builtin) {
    return (
      <div className="card flex flex-col gap-2 p-5 text-sm text-text-secondary">
        <p>
          <b className="text-text-primary">{user.username}</b> is a built-in admin: it comes from the panel&apos;s{" "}
          <code className="font-mono">AUTH_USERS</code> setting, has every access, and can&apos;t be changed or deleted here — so the
          panel always has an admin who can get in.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-5">
      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <h2 className="text-base font-bold text-text-primary">Account</h2>
        <RolePicker value={role} onChange={setRole} disabled={self} />
        {self && <p className="text-xs text-text-muted">You can&apos;t change your own role; ask another admin.</p>}
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-text-secondary">Groups</span>
          <GroupPicker groups={groups} value={memberOf} onChange={setMemberOf} />
        </div>
        <label className="flex items-center gap-2 text-sm text-text-primary">
          <input type="checkbox" checked={disabled} disabled={self} onChange={(e) => setDisabled(e.target.checked)} />
          Disabled — can&apos;t sign in, and is signed out at once
        </label>
        <div>
          <Button onClick={saveAccount} disabled={busy || !accountDirty}>
            Save account
          </Button>
        </div>
      </section>

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <div>
          <h2 className="text-base font-bold text-text-primary">Access to tabs</h2>
          <p className="mt-1 text-sm text-text-secondary">
            {user.role === "admin"
              ? "An admin has write access to every tab; what's set here applies if the role changes."
              : user.role === "viewer"
                ? "A viewer gets at most view, whatever is set: make them a writer to give write access."
                : "What this user can do in each tab. Their groups' access adds to it: the highest wins."}
          </p>
        </div>
        <AccessMatrix tabs={tabs} value={grants} onChange={setGrants} cap={user.role === "viewer" ? "view" : "write"} effective={user.effective} />
        <div className="sticky bottom-24 flex justify-end md:bottom-4">
          <Button onClick={saveGrants} disabled={busy || !grantsDirty} className="shadow-lg">
            Save access
          </Button>
        </div>
      </section>

      <section className="card flex flex-col gap-3 p-4 sm:p-5">
        <h2 className="text-base font-bold text-text-primary">Password</h2>
        <div className="flex flex-wrap items-end gap-2">
          <label className={`${LABEL} min-w-0 flex-1`}>
            New password
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} className="field" autoComplete="new-password" minLength={10} />
          </label>
          <Button variant="secondary" onClick={resetPassword} disabled={busy || password.length < 10}>
            Set password
          </Button>
        </div>
        <p className="text-xs text-text-muted">At least 10 characters. They&apos;re signed out everywhere, so they sign in again with it.</p>
      </section>

      {!self && (
        <section className="card flex flex-col gap-3 p-4 sm:p-5">
          <h2 className="text-base font-bold text-status-critical">Delete user</h2>
          <p className="text-sm text-text-secondary">
            Removes the account, its access and group memberships, and signs it out. What they did in the panel stays. To
            confirm, type <b className="font-mono text-text-primary">{user.username}</b>:
          </p>
          <div className="flex flex-wrap gap-2">
            <input className="field max-w-xs font-mono" value={confirmDelete} onChange={(e) => setConfirmDelete(e.target.value)} autoComplete="off" />
            <Button variant="danger" onClick={remove} disabled={busy || confirmDelete !== user.username}>
              Delete {user.username}
            </Button>
          </div>
        </section>
      )}
    </div>
  );
}

// GroupEditor is one group: name, members, access per tab.
export function GroupEditor({ initial, tabs, users }: { initial: PanelGroup; tabs: PanelTab[]; users: { username: string; role: Role }[] }) {
  const router = useRouter();
  const [group, setGroup] = useState(initial);
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [members, setMembers] = useState(initial.members);
  const [grants, setGrants] = useState<TabAccess>(initial.grants);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [busy, setBusy] = useState(false);

  const detailsDirty = name !== group.name || description !== group.description || members.slice().sort().join() !== group.members.slice().sort().join();
  const grantsDirty = JSON.stringify(Object.entries(grants).sort()) !== JSON.stringify(Object.entries(group.grants).sort());

  async function run(fn: () => Promise<void>) {
    setBusy(true);
    try {
      await fn();
    } finally {
      setBusy(false);
    }
  }

  const saveDetails = () =>
    run(async () => {
      const res = await send("PUT", `/api/admin/groups/${encodeURIComponent(group.id)}`, { name, description, members });
      if (!res) return;
      setGroup(await res.json());
      toast.success("Group saved.");
      router.refresh();
    });

  const saveGrants = () =>
    run(async () => {
      const res = await send("PUT", `/api/admin/groups/${encodeURIComponent(group.id)}/grants`, { grants });
      if (!res) return;
      const g: PanelGroup = await res.json();
      setGroup(g);
      setGrants(g.grants);
      toast.success("Access saved. It applies to every member on their next click.");
    });

  const remove = () =>
    run(async () => {
      const res = await send("DELETE", `/api/admin/groups/${encodeURIComponent(group.id)}`);
      if (!res) return;
      toast.success(`Group ${group.name} is deleted; its members keep their own access.`);
      router.push("/admin");
      router.refresh();
    });

  const dbUsers = users; // built-in admins can't be in groups, nor need to

  return (
    <div className="flex flex-col gap-5">
      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <div className="grid gap-3 sm:grid-cols-2">
          <label className={LABEL}>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} className="field" />
          </label>
          <label className={LABEL}>
            Description
            <input value={description} onChange={(e) => setDescription(e.target.value)} className="field" />
          </label>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-text-secondary">Members</span>
          {dbUsers.length === 0 ? (
            <p className="text-xs text-text-muted">No users to add yet: create them first.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {dbUsers.map((u) => {
                const on = members.includes(u.username);
                return (
                  <label key={u.username} className={`flex cursor-pointer items-center gap-2 rounded-full px-3 py-1.5 text-sm ${on ? "bg-accent-soft text-accent" : "bg-surface-2 text-text-secondary"}`}>
                    <input
                      type="checkbox"
                      checked={on}
                      onChange={() => setMembers(on ? members.filter((x) => x !== u.username) : [...members, u.username])}
                    />
                    <span className="font-mono">{u.username}</span>
                    <span className="text-[11px] text-text-muted">{u.role}</span>
                  </label>
                );
              })}
            </div>
          )}
          <p className="text-xs text-text-muted">A viewer in this group still gets at most view.</p>
        </div>
        <div>
          <Button onClick={saveDetails} disabled={busy || !detailsDirty || !name.trim()}>
            Save group
          </Button>
        </div>
      </section>

      <section className="card flex flex-col gap-4 p-4 sm:p-5">
        <div>
          <h2 className="text-base font-bold text-text-primary">Access to tabs</h2>
          <p className="mt-1 text-sm text-text-secondary">What every member of {group.name} can do in each tab, on top of their own access.</p>
        </div>
        <AccessMatrix tabs={tabs} value={grants} onChange={setGrants} />
        <div className="sticky bottom-24 flex justify-end md:bottom-4">
          <Button onClick={saveGrants} disabled={busy || !grantsDirty} className="shadow-lg">
            Save access
          </Button>
        </div>
      </section>

      <section className="card flex flex-wrap items-center gap-3 p-4 sm:p-5">
        <p className="min-w-0 flex-1 text-sm text-text-secondary">Deleting the group removes the access it gives; its members stay, with their own.</p>
        {confirmDelete ? (
          <>
            <Button variant="danger" onClick={remove} disabled={busy}>
              Yes, delete {group.name}
            </Button>
            <Button variant="secondary" onClick={() => setConfirmDelete(false)} disabled={busy}>
              Cancel
            </Button>
          </>
        ) : (
          <Button variant="danger" onClick={() => setConfirmDelete(true)}>
            Delete group
          </Button>
        )}
      </section>
    </div>
  );
}

// ChangePasswordForm is the signed-in user changing their own password.
export function ChangePasswordForm() {
  const [form, setForm] = useState({ current: "", next: "", again: "" });
  const [busy, setBusy] = useState(false);
  const mismatch = form.again !== "" && form.again !== form.next;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const res = await send("POST", "/api/account/password", { current: form.current, new: form.next });
      if (!res) return;
      setForm({ current: "", next: "", again: "" });
      toast.success("Password changed. Your other sessions are signed out.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-3">
      <div className="grid gap-3 sm:grid-cols-3">
        <label className={LABEL}>
          Current password
          <input type="password" value={form.current} onChange={(e) => setForm({ ...form, current: e.target.value })} className="field" autoComplete="current-password" required />
        </label>
        <label className={LABEL}>
          New password
          <input type="password" value={form.next} onChange={(e) => setForm({ ...form, next: e.target.value })} className="field" autoComplete="new-password" minLength={10} required />
        </label>
        <label className={LABEL}>
          New password, again
          <input type="password" value={form.again} onChange={(e) => setForm({ ...form, again: e.target.value })} className="field" autoComplete="new-password" required />
          {mismatch && <span className="text-xs text-status-critical">Doesn&apos;t match.</span>}
        </label>
      </div>
      <div>
        <Button type="submit" disabled={busy || !form.current || form.next.length < 10 || form.next !== form.again}>
          {busy ? "Changing…" : "Change password"}
        </Button>
      </div>
    </form>
  );
}
