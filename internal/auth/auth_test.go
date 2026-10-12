// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/db"
)

func TestParseUsers(t *testing.T) {
	u, err := ParseUsers(" alice:$2a$10$abc , bob:$2a$10$def,")
	if err != nil || len(u) != 2 || u["alice"] != "$2a$10$abc" || u["bob"] != "$2a$10$def" {
		t.Fatalf("ParseUsers = %v, %v", u, err)
	}
	for _, bad := range []string{"", " , ", "alice", "alice:", ":hash"} {
		if _, err := ParseUsers(bad); err == nil {
			t.Errorf("ParseUsers(%q) should fail", bad)
		}
	}
}

type fakeVerifier map[string]string

func (f fakeVerifier) Verify(_ context.Context, user, pw string) bool {
	return pw != "" && f[user] == pw
}

// TestLoginLogoutE2E drives login and logout against a throwaway Postgres
// (TEST_DATABASE_URL).
func TestLoginLogoutE2E(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("needs TEST_DATABASE_URL to a throwaway Postgres")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	sessions := NewSessionStore(pool, time.Hour)
	mux := http.NewServeMux()
	NewHandler(fakeVerifier{"alice": "right"}, sessions, true).RegisterRoutes(mux)
	post := func(path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}

	if w := post("/api/auth/login", `{"username":"alice","password":"wrong"}`, nil); w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Errorf("a wrong password: %d, cookies %v", w.Code, w.Result().Cookies())
	}
	if w := post("/api/auth/login", `not json`, nil); w.Code != http.StatusBadRequest {
		t.Errorf("a bad body: %d", w.Code)
	}
	w := post("/api/auth/login", `{"username":"alice","password":"right"}`, nil)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %v", w.Code, w.Result().Cookies())
	}
	c := w.Result().Cookies()[0]
	if c.Name != CookieName || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || len(c.Value) < 40 {
		t.Errorf("the session cookie: %+v", c)
	}
	if user, ok := sessions.Validate(ctx, c.Value); !ok || user != "alice" {
		t.Fatalf("the new session doesn't validate: %q %v", user, ok)
	}
	if w := post("/api/auth/logout", "", c); w.Code != 200 || w.Result().Cookies()[0].MaxAge >= 0 {
		t.Errorf("logout: %d %v", w.Code, w.Result().Cookies())
	}
	if _, ok := sessions.Validate(ctx, c.Value); ok {
		t.Error("the session still validates after logout")
	}
	if _, ok := sessions.Validate(ctx, "no-such-token"); ok {
		t.Error("an unknown token validated")
	}

	// An expired session is refused, and removed.
	short := NewSessionStore(pool, -time.Second)
	token, _, err := short.Create(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := short.Validate(ctx, token); ok {
		t.Error("an expired session validated")
	}
	var n int
	pool.QueryRow(ctx, "SELECT count(*) FROM sessions WHERE token = $1", token).Scan(&n)
	if n != 0 {
		t.Error("an expired session wasn't removed")
	}
}
