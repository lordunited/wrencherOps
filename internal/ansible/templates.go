// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

// Template is a starter playbook offered when creating one.
type Template struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Files       []File `json:"files"`
}

// Templates use only ansible.builtin modules: the panel ships ansible-core,
// without extra collections.
var Templates = []Template{
	{
		Key: "blank", Name: "Blank playbook", Description: "A minimal playbook to start from.",
		Files: []File{{Path: EntryPoint, Content: `- name: My playbook
  hosts: all
  become: true
  tasks:
    - name: Say hello
      ansible.builtin.debug:
        msg: "Hello from {{ inventory_hostname }}"
`}},
	},
	{
		Key: "facts", Name: "Server facts", Description: "Reads each server's OS, kernel, CPUs, memory and uptime. Changes nothing.",
		Files: []File{{Path: EntryPoint, Content: `- name: Server facts
  hosts: all
  gather_facts: true
  tasks:
    - name: Show what each server runs
      ansible.builtin.debug:
        msg: >-
          {{ ansible_distribution }} {{ ansible_distribution_version }},
          kernel {{ ansible_kernel }}, {{ ansible_processor_vcpus }} CPUs,
          {{ (ansible_memtotal_mb / 1024) | round(1) }} GiB RAM,
          up {{ (ansible_uptime_seconds / 86400) | round(1) }} days
`}},
	},
	{
		Key: "upgrade", Name: "Upgrade packages", Description: "Updates the package lists and upgrades every package (apt on Debian/Ubuntu, dnf on RHEL-family), and says whether a reboot is needed. Doesn't reboot.",
		Files: []File{{Path: EntryPoint, Content: `- name: Upgrade packages
  hosts: all
  become: true
  tasks:
    - name: Upgrade with apt
      when: ansible_os_family == "Debian"
      ansible.builtin.apt:
        update_cache: true
        upgrade: dist
        cache_valid_time: 3600

    - name: Upgrade with dnf
      when: ansible_os_family == "RedHat"
      ansible.builtin.dnf:
        name: "*"
        state: latest

    - name: Check whether a reboot is needed
      ansible.builtin.stat:
        path: /var/run/reboot-required
      register: reboot_required

    - name: Report it
      ansible.builtin.debug:
        msg: "{{ 'A reboot is needed' if reboot_required.stat.exists else 'No reboot needed' }}"
`}},
	},
	{
		Key: "user", Name: "Create a user with sudo", Description: "Creates a user (set user_name in extra vars) with a home folder and adds it to the sudo/wheel group.",
		Files: []File{{Path: EntryPoint, Content: `- name: Create a user with sudo
  hosts: all
  become: true
  vars:
    user_name: deploy
  tasks:
    - name: Create {{ user_name }}
      ansible.builtin.user:
        name: "{{ user_name }}"
        shell: /bin/bash
        create_home: true
        groups: "{{ 'sudo' if ansible_os_family == 'Debian' else 'wheel' }}"
        append: true
`}},
	},
}
