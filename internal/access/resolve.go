// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Access is what one signed-in user can do: a level per tab.
type Access struct {
	Username string           `json:"username"`
	Role     string           `json:"role"`
	Builtin  bool             `json:"builtin"`
	Tabs     map[string]Level `json:"-"`
}

// IsAdmin reports whether the user is an admin.
func (a *Access) IsAdmin() bool { return a != nil && a.Role == RoleAdmin }

// Level is the user's level in a tab.
func (a *Access) Level(tab string) Level {
	if a == nil {
		return None
	}
	return a.Tabs[tab]
}

// Can reports whether the user has at least need in tab.
func (a *Access) Can(tab string, need Level) bool { return a.Level(tab) >= need }

// CanAny reports whether the user has at least need in any of tabs.
func (a *Access) CanAny(need Level, tabs ...string) bool {
	for _, t := range tabs {
		if a.Can(t, need) {
			return true
		}
	}
	return false
}

// TabLevels is the user's level per tab, by name, for the browser.
func (a *Access) TabLevels() map[string]string {
	out := map[string]string{}
	for _, t := range Tabs {
		if l := a.Level(t.Key); l > None {
			out[t.Key] = l.String()
		}
	}
	return out
}

// Effective works out a user's level in every tab from its role and what
// it and its groups were granted: the highest grant wins, then the role
// caps it — an admin has everything, a viewer at most view, and nobody
// but an admin gets an admin tab or writes in a view-only one.
func Effective(role string, grants ...map[string]Level) map[string]Level {
	out := map[string]Level{}
	for _, t := range Tabs {
		l := None
		if role == RoleAdmin {
			l = Write
		} else if !t.AdminOnly {
			for _, g := range grants {
				if g[t.Key] > l {
					l = g[t.Key]
				}
			}
			if t.Everyone && l < View {
				l = View
			}
			if role != RoleWriter && l > View {
				l = View // a viewer, or an unknown role: never more than view
			}
		}
		if t.ViewOnly && l > View {
			l = View
		}
		if l > None {
			out[t.Key] = l
		}
	}
	return out
}

// ErrNoAccount is a sign-in for a user that no longer exists, or is
// disabled.
var ErrNoAccount = errors.New("this account doesn't exist or is disabled")

// Resolver turns a signed-in username into its Access, and checks
// passwords. Built-in accounts (AUTH_USERS: name → bcrypt hash) are
// admins and can't be changed from the panel, so nobody can lock
// everyone out.
type Resolver struct {
	builtin map[string]string
	store   accessStore

	mu    sync.Mutex
	cache map[string]cached
}

// cacheFor is how long a user's Access is kept. Changes through the API
// apply at once (Forget); this only bounds how long one made around it —
// straight in the database — takes.
const cacheFor = 30 * time.Second

type cached struct {
	access  *Access
	expires time.Time
}

// accessStore is what the Resolver reads; *Store is one.
type accessStore interface {
	User(ctx context.Context, username string) (*User, error)
	Grants(ctx context.Context, kind, subject string) (map[string]Level, error)
}

func NewResolver(builtin map[string]string, store accessStore) *Resolver {
	return &Resolver{builtin: builtin, store: store, cache: map[string]cached{}}
}

// IsBuiltin reports whether username is one of AUTH_USERS.
func (r *Resolver) IsBuiltin(username string) bool {
	_, ok := r.builtin[username]
	return ok
}

// Forget drops every cached Access: call after any change to users,
// groups or grants. Changes apply on the user's next request.
func (r *Resolver) Forget() {
	r.mu.Lock()
	r.cache = map[string]cached{}
	r.mu.Unlock()
}

// Resolve is username's Access.
func (r *Resolver) Resolve(ctx context.Context, username string) (*Access, error) {
	r.mu.Lock()
	c, ok := r.cache[username]
	r.mu.Unlock()
	if ok && time.Now().Before(c.expires) {
		return c.access, nil
	}
	var a *Access
	if r.IsBuiltin(username) {
		a = &Access{Username: username, Role: RoleAdmin, Builtin: true, Tabs: Effective(RoleAdmin)}
	} else {
		u, err := r.store.User(ctx, username)
		if errors.Is(err, ErrNotFound) || (err == nil && u.Disabled) {
			return nil, ErrNoAccount
		}
		if err != nil {
			return nil, err
		}
		grants := []map[string]Level{}
		g, err := r.store.Grants(ctx, SubjectUser, username)
		if err != nil {
			return nil, err
		}
		grants = append(grants, g)
		for _, id := range u.Groups {
			g, err := r.store.Grants(ctx, SubjectGroup, id)
			if err != nil {
				return nil, err
			}
			grants = append(grants, g)
		}
		a = &Access{Username: username, Role: u.Role, Tabs: Effective(u.Role, grants...)}
	}
	r.mu.Lock()
	r.cache[username] = cached{access: a, expires: time.Now().Add(cacheFor)}
	r.mu.Unlock()
	return a, nil
}

// dummyHash is compared against for an unknown username, so a wrong
// username takes as long as a wrong password and doesn't give itself away.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not a real password, only for timing"), bcrypt.DefaultCost)

// Verify checks a username and password: a built-in account's, or an
// enabled database user's.
func (r *Resolver) Verify(ctx context.Context, username, password string) bool {
	hash, builtin := r.builtin[username]
	if !builtin {
		u, err := r.store.User(ctx, username)
		if err == nil && !u.Disabled {
			hash = u.passwordHash
		}
	}
	if hash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type ctxKey struct{}

// WithAccess is ctx carrying a.
func WithAccess(ctx context.Context, a *Access) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// FromContext is the signed-in user's Access the Guard put on a request,
// or nil outside one.
func FromContext(ctx context.Context) *Access {
	a, _ := ctx.Value(ctxKey{}).(*Access)
	return a
}

// Filter keeps the items whose tab the request's user can view. Outside a
// guarded request (no Access) it keeps everything: every API route is
// mounted behind the Guard, which always sets one.
func Filter[T any](ctx context.Context, items []T, tab func(T) string) []T {
	a := FromContext(ctx)
	if a == nil {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if a.Can(tab(it), View) {
			out = append(out, it)
		}
	}
	return out
}

// Allowed reports whether the request's user has need in any of tabs (true
// outside a guarded request, as with Filter).
func Allowed(ctx context.Context, need Level, tabs ...string) bool {
	a := FromContext(ctx)
	return a == nil || a.CanAny(need, tabs...)
}
