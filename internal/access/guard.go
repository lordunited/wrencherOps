// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// CookieName is the session cookie (auth.CookieName's value).
const CookieName = "session"

// Sessions validates a session token; *auth.SessionStore is one.
type Sessions interface {
	Validate(ctx context.Context, token string) (username string, ok bool)
}

// Guard is the API's gatekeeper: it signs the request's user in from its
// session, finds the route's Rule and refuses the request unless the user
// has the access it needs. It replaces a plain "signed in?" check, on
// every API route.
type Guard struct {
	sessions Sessions
	resolver *Resolver
	rules    map[string]Rule
	match    *http.ServeMux
}

type matchKey struct{}

// matched is filled in by match: which pattern a request is, with its
// path values.
type matched struct {
	pattern string
	req     *http.Request
}

func NewGuard(sessions Sessions, resolver *Resolver, rules map[string]Rule) *Guard {
	g := &Guard{sessions: sessions, resolver: resolver, rules: rules, match: http.NewServeMux()}
	for pattern := range rules {
		g.match.HandleFunc(pattern, func(_ http.ResponseWriter, r *http.Request) {
			m := r.Context().Value(matchKey{}).(*matched)
			m.pattern, m.req = pattern, r
		})
	}
	return g
}

// discard is a ResponseWriter that drops what match writes for a request
// it has no pattern for.
type discard struct{ h http.Header }

func (d *discard) Header() http.Header {
	if d.h == nil {
		d.h = http.Header{}
	}
	return d.h
}
func (*discard) Write(b []byte) (int, error) { return len(b), nil }
func (*discard) WriteHeader(int)             {}

// Check finds the rule for r and whether a may follow it. It returns the
// request to pass on (its body intact if a rule peeked at it), and the
// HTTP status and message to refuse with, or 0.
func (g *Guard) Check(r *http.Request, a *Access) (*http.Request, int, string) {
	m := &matched{}
	g.match.ServeHTTP(&discard{}, r.WithContext(context.WithValue(r.Context(), matchKey{}, m)))
	if m.pattern == "" {
		return r, http.StatusNotFound, "not found"
	}
	rule := g.rules[m.pattern]
	tabs := rule.Tabs
	if rule.Resolve != nil {
		var err error
		tabs, err = rule.Resolve(r.Context(), m.req)
		r.Body = m.req.Body // a peek replaced it
		if errors.Is(err, ErrNotFound) {
			return r, http.StatusNotFound, "not found"
		}
		if err != nil {
			return r, http.StatusInternalServerError, "checking access: " + err.Error()
		}
	}
	if len(tabs) == 0 && rule.Need == None {
		return r, 0, ""
	}
	if a.CanAny(rule.Need, tabs...) {
		return r, 0, ""
	}
	return r, http.StatusForbidden, denied(rule.Need, tabs)
}

func denied(need Level, tabs []string) string {
	names := make([]string, 0, len(tabs))
	for _, k := range tabs {
		if t, ok := TabByKey(k); ok {
			names = append(names, t.Category+" › "+t.Label)
		} else {
			names = append(names, k)
		}
	}
	what := "view"
	if need == Write {
		what = "change anything in"
	}
	return fmt.Sprintf("you don't have access to %s %s; an admin can give it to you in Admin › Users & access", what, strings.Join(names, " or "))
}

// Wrap guards next.
func (g *Guard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		username, ok := g.sessions.Validate(r.Context(), cookie.Value)
		if !ok {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		a, err := g.resolver.Resolve(r.Context(), username)
		if errors.Is(err, ErrNoAccount) {
			http.Error(w, "not authenticated", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "checking access: "+err.Error(), http.StatusInternalServerError)
			return
		}
		r, status, msg := g.Check(r, a)
		if status != 0 {
			if status == http.StatusForbidden {
				log.Printf("access: refused %s %s to %s", r.Method, r.URL.Path, username)
			}
			http.Error(w, msg, status)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithAccess(r.Context(), a)))
	})
}
