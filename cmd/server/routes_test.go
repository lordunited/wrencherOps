// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package main

import (
	"net/http"
	"sort"
	"testing"

	"devops-wrapper/internal/access"
	"devops-wrapper/internal/ansible"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/gitlab"
	"devops-wrapper/internal/servers"
	"devops-wrapper/internal/service"
	"devops-wrapper/internal/terraform"
)

type recorder struct{ patterns []string }

func (r *recorder) HandleFunc(p string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, p)
}

func testAPI() apiHandlers {
	return apiHandlers{
		gitlab: new(gitlab.Client), deploy: new(deploy.Handler), service: new(service.Handler),
		servers: new(servers.Handler), ansible: new(ansible.Handler), terraform: new(terraform.Handler),
		access: new(access.Handler),
	}
}

// Every API route has an access rule — the guard refuses (404s) one
// without — and every rule is for a route that exists.
func TestEveryRouteHasARule(t *testing.T) {
	rec := &recorder{}
	testAPI().register(rec)
	policy := access.Policy(access.Lookups{})

	registered := map[string]bool{}
	for _, p := range rec.patterns {
		if registered[p] {
			t.Errorf("%s is registered twice", p)
		}
		registered[p] = true
		if _, ok := policy[p]; !ok {
			t.Errorf("route %s has no access rule in access.Policy", p)
		}
	}
	var stale []string
	for p := range policy {
		if !registered[p] {
			stale = append(stale, p)
		}
	}
	sort.Strings(stale)
	for _, p := range stale {
		t.Errorf("access.Policy has a rule for %s, which no handler serves", p)
	}
	if len(rec.patterns) < 100 {
		t.Errorf("only %d routes recorded; is a handler missing from apiHandlers.register?", len(rec.patterns))
	}
}

// All the API's routes go on one mux: none may conflict.
func TestRoutesShareOneMux(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering every route on one mux panicked: %v", r)
		}
	}()
	testAPI().register(http.NewServeMux())
}

// Every tab a rule names exists.
func TestRulesNameRealTabs(t *testing.T) {
	for p, r := range access.Policy(access.Lookups{}) {
		for _, tab := range r.Tabs {
			if _, ok := access.TabByKey(tab); !ok {
				t.Errorf("%s needs tab %q, which doesn't exist", p, tab)
			}
		}
	}
}
