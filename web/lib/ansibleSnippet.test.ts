// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { describe, expect, it } from "vitest";
import { optionValue, taskYAML, yamlScalar, type ModuleOption } from "@/lib/ansibleSnippet";

const opt = (name: string, type: string, required = false) => ({ name, type, required }) as ModuleOption;

describe("yamlScalar", () => {
  it("leaves plain words plain", () => {
    expect(yamlScalar("nginx")).toBe("nginx");
    expect(yamlScalar("/etc/nginx/nginx.conf")).toBe("/etc/nginx/nginx.conf");
  });
  it("quotes what YAML would read as something else", () => {
    for (const v of ["", "yes", "No", "null", "~", "123", "1.5", "-x", "a: b", "# c", "*alias", "trailing "]) {
      expect(yamlScalar(v)).toBe(JSON.stringify(v));
    }
  });
});

describe("optionValue", () => {
  it("formats by the option's type", () => {
    expect(optionValue(opt("enabled", "bool"), "")).toBe("true");
    expect(optionValue(opt("enabled", "bool"), "no")).toBe("false");
    expect(optionValue(opt("port", "int"), "8080")).toBe("8080");
    expect(optionValue(opt("port", "int"), "")).toBe("0");
    expect(optionValue(opt("pkgs", "list"), "nginx, curl,")).toBe("[nginx, curl]");
    expect(optionValue(opt("pkgs", "list"), "")).toBe("[]");
    expect(optionValue(opt("env", "dict"), "")).toBe("{}");
    expect(optionValue(opt("dest", "str"), "{{ dir }}/x")).toBe('"{{ dir }}/x"');
    expect(optionValue(opt("state", "str"), "present")).toBe("present");
  });
});

describe("taskYAML", () => {
  it("writes a task with its options, marking the empty ones", () => {
    const y = taskYAML("ansible.builtin.apt", "Install nginx", [
      { option: opt("name", "str", true), value: "" },
      { option: opt("state", "str"), value: "present" },
      { option: opt("update_cache", "bool"), value: "yes" },
    ]);
    expect(y).toBe(
      "- name: Install nginx\n  ansible.builtin.apt:\n    name:   # required — fill in\n    state: present\n    update_cache: true\n".replace(
        "name:   #",
        'name: ""  #',
      ),
    );
  });
  it("writes an empty mapping for a module with no options", () => {
    expect(taskYAML("ansible.builtin.ping", "", [])).toBe("- name: Describe what this does\n  ansible.builtin.ping: {}\n");
  });
});
