// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { AnsibleModulePicker } from "@/components/AnsibleModulePicker";

export default function AnsibleModulesPage() {
  return (
    <div className="flex flex-col gap-5">
      <div>
        <h1 className="page-title">Modules</h1>
        <p className="mt-1 text-sm text-text-secondary">
          Every module the panel&apos;s Ansible ships (ansible.builtin), with its options and examples. Build a task here and
          copy it, or insert modules straight into a playbook from its editor (+ Insert module).
        </p>
      </div>
      <section className="card p-4 sm:p-5">
        <AnsibleModulePicker />
      </section>
    </div>
  );
}
