// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"devops-wrapper/internal/access"
	"devops-wrapper/internal/auth"
	"devops-wrapper/internal/db"
)

// TestUserManagementE2E drives the real login, guard and admin API against
// a throwaway Postgres (TEST_DATABASE_URL): an admin makes a user and a
// group, grants access, and the user gets exactly that.
func TestUserManagementE2E(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("needs TEST_DATABASE_URL to a throwaway Postgres")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, table := range []string{"panel_grants", "panel_group_members", "panel_groups", "panel_users", "sessions"} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Fatal(err)
		}
	}

	rootHash, _ := bcrypt.GenerateFromPassword([]byte("root password!"), bcrypt.MinCost)
	store := access.NewStore(pool)
	resolver := access.NewResolver(map[string]string{"root": string(rootHash)}, store)
	sessions := auth.NewSessionStore(pool, time.Hour)

	mux := http.NewServeMux()
	auth.NewHandler(resolver, sessions, false).RegisterRoutes(mux)
	api := http.NewServeMux()
	access.NewHandler(store, resolver).RegisterRoutes(api)
	// A stand-in for the panel's own routes, under their real patterns.
	ok := func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"ok":true}`) }
	api.HandleFunc("GET /api/servers", ok)
	api.HandleFunc("DELETE /api/servers/{id}", ok)
	api.HandleFunc("POST /api/terraform/workspaces/{id}/runs", ok)
	api.HandleFunc("GET /api/gitlab/projects", ok)
	guard := access.NewGuard(sessions, resolver, access.Policy(access.Lookups{}))
	mux.Handle("/api/", guard.Wrap(api))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar}
	}
	call := func(c *http.Client, method, path string, body any) (int, string) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = strings.NewReader(string(b))
		}
		req, _ := http.NewRequest(method, srv.URL+path, rd)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, strings.TrimSpace(string(b))
	}
	expect := func(c *http.Client, method, path string, body any, want int) string {
		t.Helper()
		code, out := call(c, method, path, body)
		if code != want {
			t.Fatalf("%s %s: %d %s; want %d", method, path, code, out, want)
		}
		return out
	}
	login := func(user, pw string) (*http.Client, int) {
		c := client()
		code, _ := call(c, "POST", "/api/auth/login", map[string]string{"username": user, "password": pw})
		return c, code
	}

	root, code := login("root", "root password!")
	if code != 200 {
		t.Fatalf("the built-in admin can't sign in: %d", code)
	}
	me := expect(root, "GET", "/api/auth/me", nil, 200)
	if !strings.Contains(me, `"role":"admin"`) || !strings.Contains(me, `"admin.users":"write"`) {
		t.Fatalf("me = %s", me)
	}
	if !strings.Contains(expect(root, "GET", "/api/admin/tabs", nil, 200), `"deploy.envoy"`) {
		t.Fatal("tabs don't list every tab")
	}

	// Bad users are refused.
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "Bad Name", "password": "long enough pw", "role": "writer"}, 400)
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "alice", "password": "short", "role": "writer"}, 400)
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "alice", "password": "long enough pw", "role": "god"}, 400)
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "root", "password": "long enough pw", "role": "writer"}, 400)

	// A writer, in a group.
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "alice", "password": "alice's password", "role": "writer"}, 200)
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "alice", "password": "alice's password", "role": "writer"}, 409)
	var g struct{ ID string }
	json.Unmarshal([]byte(expect(root, "POST", "/api/admin/groups", map[string]any{"name": "infra", "members": []string{"alice"}}, 200)), &g)
	expect(root, "POST", "/api/admin/groups", map[string]any{"name": "infra"}, 409)
	expect(root, "PUT", "/api/admin/groups/"+g.ID+"/grants", map[string]any{"grants": map[string]string{"terraform.workspaces": "write"}}, 200)
	expect(root, "PUT", "/api/admin/users/alice/grants", map[string]any{"grants": map[string]string{"servers": "view"}}, 200)
	expect(root, "PUT", "/api/admin/users/alice/grants", map[string]any{"grants": map[string]string{"admin.users": "write"}}, 400)

	alice, code := login("alice", "alice's password")
	if code != 200 {
		t.Fatalf("alice can't sign in: %d", code)
	}
	if _, code := login("alice", "wrong password"); code != 401 {
		t.Fatalf("a wrong password signed in: %d", code)
	}
	expect(alice, "GET", "/api/servers", nil, 200)
	expect(alice, "DELETE", "/api/servers/x", nil, 403)
	expect(alice, "POST", "/api/terraform/workspaces/w/runs", nil, 200) // through infra
	expect(alice, "GET", "/api/gitlab/projects", nil, 403)
	expect(alice, "GET", "/api/admin/users", nil, 403)
	user := expect(root, "GET", "/api/admin/users/alice", nil, 200)
	if !strings.Contains(user, `"terraform.workspaces":"write"`) || !strings.Contains(user, `"servers":"view"`) {
		t.Fatalf("alice's effective access = %s", user)
	}

	// Made a viewer: never writes, even through a group — at once.
	expect(root, "PUT", "/api/admin/users/alice", map[string]any{"role": "viewer"}, 200)
	expect(alice, "POST", "/api/terraform/workspaces/w/runs", nil, 403)
	expect(alice, "GET", "/api/servers", nil, 200)

	// Her own password.
	expect(alice, "POST", "/api/account/password", map[string]string{"current": "wrong", "new": "a brand new password"}, 403)
	expect(alice, "POST", "/api/account/password", map[string]string{"current": "alice's password", "new": "tiny"}, 400)
	expect(alice, "POST", "/api/account/password", map[string]string{"current": "alice's password", "new": "a brand new password"}, 200)
	expect(alice, "GET", "/api/servers", nil, 200) // this session stays
	if _, code := login("alice", "a brand new password"); code != 200 {
		t.Fatal("the new password doesn't work")
	}
	expect(root, "POST", "/api/account/password", map[string]string{"current": "root password!", "new": "a brand new password"}, 400)

	// An admin can't lock themself out.
	expect(root, "POST", "/api/admin/users", map[string]any{"username": "boss", "password": "boss's password", "role": "admin"}, 200)
	boss, _ := login("boss", "boss's password")
	expect(boss, "PUT", "/api/admin/users/boss", map[string]any{"role": "writer"}, 400)
	expect(boss, "DELETE", "/api/admin/users/boss", nil, 400)
	expect(boss, "DELETE", "/api/admin/users/root", nil, 400) // built-in

	// Disabled: signed out at once, and can't sign in.
	expect(root, "PUT", "/api/admin/users/alice", map[string]any{"role": "viewer", "disabled": true}, 200)
	expect(alice, "GET", "/api/servers", nil, 401)
	if _, code := login("alice", "a brand new password"); code != 401 {
		t.Fatal("a disabled user signed in")
	}

	// Deleted: gone from its group too.
	expect(root, "DELETE", "/api/admin/users/alice", nil, 200)
	expect(root, "GET", "/api/admin/users/alice", nil, 404)
	if grp := expect(root, "GET", "/api/admin/groups/"+g.ID, nil, 200); strings.Contains(grp, "alice") {
		t.Fatalf("a deleted user is still in a group: %s", grp)
	}
	expect(root, "DELETE", "/api/admin/groups/"+g.ID, nil, 200)
	expect(root, "GET", "/api/admin/groups/"+g.ID, nil, 404)

	// Signed out: nothing.
	expect(root, "POST", "/api/auth/logout", nil, 200)
	expect(root, "GET", "/api/auth/me", nil, 401)
}
