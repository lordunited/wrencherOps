// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestModuleCatalog reads the real catalog from ansible-doc (skipped where
// Ansible isn't installed).
func TestModuleCatalog(t *testing.T) {
	if _, err := exec.LookPath("ansible-doc"); err != nil {
		t.Skip("ansible-doc isn't installed here")
	}
	var c moduleCatalog
	if err := c.load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.list) < 60 {
		t.Errorf("only %d modules", len(c.list))
	}
	cp := c.modules["ansible.builtin.copy"]
	if cp == nil || cp.CheckMode != "full" || cp.DiffMode != "full" || !strings.Contains(cp.Examples, "ansible.builtin.copy:") {
		t.Fatalf("copy: %+v", cp)
	}
	if cp.Options[0].Name != "dest" || !cp.Options[0].Required || cp.Options[0].Type != "path" {
		t.Errorf("required options come first: %+v", cp.Options[0])
	}
	for _, o := range cp.Options {
		if strings.Contains(o.Description, "O(") || strings.Contains(o.Description, "C(") {
			t.Errorf("markup left in %s: %q", o.Name, o.Description)
		}
	}
	svc := c.modules["ansible.builtin.service"]
	var state *ModuleOption
	for i := range svc.Options {
		if svc.Options[i].Name == "state" {
			state = &svc.Options[i]
		}
	}
	if state == nil || len(state.Choices) < 3 {
		t.Errorf("service state choices: %+v", state)
	}
	t.Logf("%d modules; check mode none: %s", len(c.list), func() string {
		var n []string
		for _, m := range c.list {
			if m.CheckMode == "none" {
				n = append(n, strings.TrimPrefix(m.Name, "ansible.builtin."))
			}
		}
		return strings.Join(n, ", ")
	}())
}
