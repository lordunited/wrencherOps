// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import "testing"

func TestValidateCasting(t *testing.T) {
	if name, err := ValidateCasting(SigNozDefaultCasting); err != nil || name != "signoz" {
		t.Errorf("default casting: %q, %v", name, err)
	}
	for label, casting := range map[string]string{
		"not YAML":     "spec: [",
		"no name":      "apiVersion: v1alpha1\nkind: Installation\nspec:\n  deployment:\n    flavor: compose\n    mode: docker\n",
		"swarm flavor": "kind: Installation\nmetadata:\n  name: s\nspec:\n  deployment:\n    flavor: swarm\n    mode: docker\n",
		"wrong kind":   "kind: Something\nmetadata:\n  name: s\nspec:\n  deployment:\n    flavor: compose\n    mode: docker\n",
	} {
		if _, err := ValidateCasting(casting); err == nil {
			t.Errorf("%s: accepted", label)
		}
	}
}
