// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"net/http"
	"strings"
	"testing"
)

type recordingMux struct{ patterns []string }

func (m *recordingMux) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	m.patterns = append(m.patterns, pattern)
}

// Registering on the real router catches patterns that conflict, which
// would otherwise only surface as a panic when the server starts.
func TestRoutesRegisterWithoutConflicts(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering routes panicked: %v", r)
		}
	}()
	(&Handler{}).RegisterRoutes(http.NewServeMux())
}

// Every route must sit under a prefix the server mounts this handler at.
// /api/apps once didn't: its routes were registered and tested against
// the handler directly, but the server never sent those paths to it, so
// every request 404'd.
func TestRoutesStayUnderPathPrefixes(t *testing.T) {
	h := &Handler{}
	mux := &recordingMux{}
	h.RegisterRoutes(mux)
	if len(mux.patterns) == 0 {
		t.Fatal("no routes registered")
	}
	for _, p := range mux.patterns {
		path := p
		if _, after, ok := strings.Cut(p, " "); ok {
			path = after // drop the method
		}
		mounted := false
		for _, prefix := range h.PathPrefixes() {
			if path == prefix || strings.HasSuffix(prefix, "/") && strings.HasPrefix(path, prefix) {
				mounted = true
			}
		}
		if !mounted {
			t.Errorf("route %q isn't under any of %v, so the server would never reach it", p, h.PathPrefixes())
		}
	}
}
