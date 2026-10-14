// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{"": None, "none": None, "view": View, "write": Write} {
		got, ok := ParseLevel(in)
		if !ok || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, ok, want)
		}
		if in != "" && got.String() != in {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), in)
		}
	}
	if _, ok := ParseLevel("admin"); ok {
		t.Error(`ParseLevel("admin") should fail`)
	}
}

func TestTabsAreUniqueAndComplete(t *testing.T) {
	keys, hrefs := map[string]bool{}, map[string]bool{}
	for _, tab := range Tabs {
		if keys[tab.Key] || hrefs[tab.Href] {
			t.Errorf("tab %s (%s) is listed twice", tab.Key, tab.Href)
		}
		keys[tab.Key], hrefs[tab.Href] = true, true
		if tab.Label == "" || tab.Category == "" || !strings.HasPrefix(tab.Href, "/") {
			t.Errorf("tab %+v is incomplete", tab)
		}
	}
}

func TestTabForHref(t *testing.T) {
	cases := map[string]string{
		"/":                          "overview",
		"/servers":                   "servers",
		"/servers/abc":               "servers",
		"/servers/notifications":     "servers.notifications",
		"/ansible/runs/12":           "ansible.runs",
		"/ansible/xyz":               "ansible.playbooks",
		"/deploy/compose/detect":     "deploy.compose",
		"/deploy/kubernetes/new":     "deploy.other",
		"/deploy/docker/1":           "deploy.other",
		"/admin":                     "admin.users",
		"/admin/users/bob":           "admin.users",
		"/terraform/runs/r1":         "terraform.runs",
		"/merge-requests":            "gitlab.merge-requests",
		"/deploy/prometheus/connect": "deploy.prometheus",
	}
	for path, want := range cases {
		tab, ok := TabForHref(path)
		if !ok || tab.Key != want {
			t.Errorf("TabForHref(%q) = %q, %v; want %q", path, tab.Key, ok, want)
		}
	}
	if tab, ok := TabForHref("/nowhere"); ok && tab.Key != "overview" {
		t.Errorf("TabForHref(/nowhere) = %q", tab.Key)
	}
}

func TestServiceTab(t *testing.T) {
	cases := []struct{ typ, app, tpl, origin, want string }{
		{"nginx", "", "", "panel", "deploy.nginx"},
		{"prometheus", "", "", "connected", "deploy.prometheus"},
		{"signoz", "", "", "panel", "deploy.signoz"},
		{"docker", "a1", "signoz", "panel", "deploy.signoz"},
		{"docker", "a1", "", "panel", "deploy.compose"},
		{"docker", "", "", "connected", "deploy.compose"},
		{"docker", "", "", "panel", "deploy.other"},
		{"kubernetes", "", "", "panel", "deploy.other"},
	}
	for _, c := range cases {
		if got := ServiceTab(c.typ, c.app, c.tpl, c.origin); got != c.want {
			t.Errorf("ServiceTab(%q,%q,%q,%q) = %q, want %q", c.typ, c.app, c.tpl, c.origin, got, c.want)
		}
		if _, ok := TabByKey(ServiceTab(c.typ, c.app, c.tpl, c.origin)); !ok {
			t.Errorf("ServiceTab for %v isn't a tab", c)
		}
	}
	for _, typ := range []string{"gitlab", "nexus3", "prometheus", "nginx", "haproxy", "envoy", "signoz"} {
		if _, ok := TabByKey(ServiceTab(typ, "", "", "panel")); !ok {
			t.Errorf("service type %s has no tab", typ)
		}
	}
}

func TestEffective(t *testing.T) {
	user := map[string]Level{"servers": Write, "gitlab.health": Write, "admin.users": Write, "deploy.nginx": View}
	group := map[string]Level{"deploy.nginx": Write, "ansible.playbooks": View}

	admin := Effective(RoleAdmin)
	for _, tab := range Tabs {
		want := Write
		if tab.ViewOnly {
			want = View
		}
		if admin[tab.Key] != want {
			t.Errorf("admin has %v in %s, want %v", admin[tab.Key], tab.Key, want)
		}
	}

	w := Effective(RoleWriter, user, group)
	expect := map[string]Level{
		"servers":           Write,
		"gitlab.health":     View,  // view-only tab
		"admin.users":       None,  // admins only
		"deploy.nginx":      Write, // the group's grant is higher
		"ansible.playbooks": View,
		"overview":          View, // everyone's
		"terraform.runs":    None, // not granted
	}
	for tab, want := range expect {
		if w[tab] != want {
			t.Errorf("writer has %v in %s, want %v", w[tab], tab, want)
		}
	}

	v := Effective(RoleViewer, user, group)
	for tab, l := range v {
		if l > View {
			t.Errorf("viewer has %v in %s", l, tab)
		}
	}
	if v["servers"] != View || v["deploy.nginx"] != View || v["admin.users"] != None {
		t.Errorf("viewer access = %v", v)
	}

	if u := Effective("bogus", user); u["servers"] != View {
		t.Errorf("an unknown role must be capped at view, got %v", u["servers"])
	}
	if n := Effective(RoleWriter); len(n) != 1 || n["overview"] != View {
		t.Errorf("a writer with no grants sees only the overview, got %v", n)
	}
}

func TestAccessCan(t *testing.T) {
	a := &Access{Role: RoleWriter, Tabs: map[string]Level{"servers": View, "deploy.nginx": Write}}
	if !a.Can("servers", View) || a.Can("servers", Write) || !a.Can("deploy.nginx", Write) || a.Can("ansible.runs", View) {
		t.Error("Can is wrong")
	}
	if !a.CanAny(Write, "servers", "deploy.nginx") || a.CanAny(Write, "servers") {
		t.Error("CanAny is wrong")
	}
	var nilAccess *Access
	if nilAccess.Can("overview", View) || nilAccess.IsAdmin() {
		t.Error("a nil Access can do nothing")
	}
	if got := a.TabLevels(); got["servers"] != "view" || got["deploy.nginx"] != "write" || len(got) != 2 {
		t.Errorf("TabLevels = %v", got)
	}
}

func TestFilterAndAllowed(t *testing.T) {
	items := []string{"servers", "deploy.nginx", "deploy.envoy"}
	if got := Filter(context.Background(), items, func(s string) string { return s }); len(got) != 3 {
		t.Errorf("outside a guarded request Filter keeps everything, got %v", got)
	}
	ctx := WithAccess(context.Background(), &Access{Tabs: map[string]Level{"servers": View, "deploy.nginx": Write}})
	got := Filter(ctx, items, func(s string) string { return s })
	if strings.Join(got, ",") != "servers,deploy.nginx" {
		t.Errorf("Filter = %v", got)
	}
	if !Allowed(ctx, Write, "deploy.envoy", "deploy.nginx") || Allowed(ctx, Write, "servers") {
		t.Error("Allowed is wrong")
	}
}

func TestUsernamesAndPasswords(t *testing.T) {
	for _, ok := range []string{"bob", "ops.team-1", "a_b", "x1"} {
		if !ValidUsername(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "a", "Bob", "-bob", "bob smith", "bob/../x", strings.Repeat("a", 64)} {
		if ValidUsername(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
	if CheckPassword("short", "bob") == nil {
		t.Error("a short password must be refused")
	}
	if CheckPassword("averylongname", "averylongname") == nil {
		t.Error("the username as password must be refused")
	}
	if CheckPassword(strings.Repeat("x", 73), "bob") == nil {
		t.Error("a password past bcrypt's 72 bytes must be refused")
	}
	if err := CheckPassword("correct horse battery", "bob"); err != nil {
		t.Error(err)
	}
}

func TestParseGrants(t *testing.T) {
	g, err := parseGrants(map[string]string{"servers": "write", "gitlab.health": "view", "deploy.nginx": "none"})
	if err != nil || g["servers"] != Write || g["gitlab.health"] != View || len(g) != 2 {
		t.Errorf("parseGrants = %v, %v", g, err)
	}
	for _, in := range []map[string]string{
		{"nope": "view"},
		{"servers": "admin"},
		{"admin.users": "view"},
		{"gitlab.health": "write"},
	} {
		if _, err := parseGrants(in); err == nil {
			t.Errorf("parseGrants(%v) should fail", in)
		}
	}
}

func TestPeekFieldKeepsTheBody(t *testing.T) {
	body := `{"kind":"nginx","host":"h","password":"pw"}`
	r := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	if got, err := peekField(r, "kind"); got != "nginx" || err != nil {
		t.Errorf("peekField = %q, %v", got, err)
	}
	rest, _ := io.ReadAll(r.Body)
	if string(rest) != body {
		t.Errorf("the body after a peek = %q", rest)
	}
}

// The peek must see what the handler's json decoding will: whatever it
// can't be sure of is refused, never guessed.
func TestPeekFieldReadsLikeTheHandler(t *testing.T) {
	cases := []struct {
		name, body string
		want       string
		err        error
	}{
		{"missing field", `{"host":"h"}`, "", nil},
		{"null field", `{"kind":null}`, "", nil},
		{"other case", `{"KIND":"gitlab"}`, "gitlab", nil},
		{"spelled twice", `{"kind":"prometheus","KIND":"gitlab"}`, "", ErrBadBody},
		{"spelled twice, Kelvin sign", `{"kind":"prometheus","Kind":"gitlab"}`, "", ErrBadBody},
		{"not a string", `{"kind":7}`, "", ErrBadBody},
		{"not json", `not json`, "", ErrBadBody},
		{"empty", ``, "", ErrBadBody},
		{"trailing value", `{"kind":"prometheus"} {"kind":"gitlab"}`, "", ErrBadBody},
		{"too large", `{"pad":"` + strings.Repeat("x", maxPeek) + `","kind":"gitlab"}`, "", ErrBodyTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/x", strings.NewReader(c.body))
			got, err := peekField(r, "kind")
			if got != c.want || !errors.Is(err, c.err) {
				t.Errorf("peekField = %q, %v; want %q, %v", got, err, c.want, c.err)
			}
			if rest, _ := io.ReadAll(r.Body); string(rest) != c.body {
				t.Errorf("the body after a peek is %d bytes, want %d", len(rest), len(c.body))
			}
		})
	}
}

// --- the guard

type fakeStore struct {
	users  map[string]*User
	grants map[string]map[string]Level // "user:bob" -> grants
	calls  int
}

func (f *fakeStore) User(_ context.Context, name string) (*User, error) {
	f.calls++
	if u, ok := f.users[name]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}

func (f *fakeStore) Grants(_ context.Context, kind, subject string) (map[string]Level, error) {
	return f.grants[kind+":"+subject], nil
}

type fakeSessions map[string]string

func (f fakeSessions) Validate(_ context.Context, token string) (string, bool) {
	u, ok := f[token]
	return u, ok
}

func hash(t *testing.T, pw string) string {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func testGuard(t *testing.T) (*Guard, *fakeStore, *Resolver) {
	store := &fakeStore{
		users: map[string]*User{
			"wendy":  {Username: "wendy", Role: RoleWriter, Groups: []string{"g1"}, passwordHash: hash(t, "wendy's password")},
			"victor": {Username: "victor", Role: RoleViewer, passwordHash: hash(t, "victor's password")},
			"gone":   {Username: "gone", Role: RoleWriter, Disabled: true, passwordHash: hash(t, "gone's password")},
		},
		grants: map[string]map[string]Level{
			"user:wendy":  {"deploy.nginx": Write, "servers": View},
			"group:g1":    {"terraform.workspaces": Write},
			"user:victor": {"deploy.nginx": Write, "deploy.compose": View},
		},
	}
	resolver := NewResolver(map[string]string{"root": hash(t, "root's password")}, store)
	services := map[string]string{"s-nginx": "deploy.nginx", "s-envoy": "deploy.envoy"}
	policy := Policy(Lookups{
		ServiceTab: func(_ context.Context, id string) (string, error) {
			if tab, ok := services[id]; ok {
				return tab, nil
			}
			return "", ErrNotFound
		},
		AppTab: func(context.Context, string) (string, error) { return "deploy.compose", nil },
	})
	sessions := fakeSessions{"t-root": "root", "t-wendy": "wendy", "t-victor": "victor", "t-gone": "gone", "t-ghost": "ghost"}
	return NewGuard(sessions, resolver, policy), store, resolver
}

func TestGuard(t *testing.T) {
	g, _, _ := testGuard(t)
	var reached *Access
	var reachedBody string
	h := g.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = FromContext(r.Context())
		b, _ := io.ReadAll(r.Body)
		reachedBody = string(b)
	}))
	cases := []struct {
		token, method, path, body string
		want                      int
	}{
		{"", "GET", "/api/servers", "", 401},
		{"t-bad", "GET", "/api/servers", "", 401},
		{"t-gone", "GET", "/api/auth/me", "", 401},  // disabled
		{"t-ghost", "GET", "/api/auth/me", "", 401}, // deleted
		{"t-wendy", "GET", "/api/auth/me", "", 200},
		{"t-wendy", "GET", "/api/servers", "", 200},
		{"t-wendy", "DELETE", "/api/servers/x", "", 403},
		{"t-wendy", "POST", "/api/servers/x/check", "", 200}, // a check only reads
		{"t-wendy", "GET", "/api/services/s-nginx", "", 200},
		{"t-wendy", "POST", "/api/services/s-nginx/deploy", "", 200},
		{"t-wendy", "GET", "/api/services/s-envoy", "", 403},
		{"t-wendy", "GET", "/api/services/s-missing", "", 404},
		{"t-wendy", "POST", "/api/services/connect", `{"kind":"nginx","container":"c"}`, 200},
		{"t-wendy", "POST", "/api/services/connect", `{"kind":"envoy"}`, 403},
		{"t-wendy", "POST", "/api/services/connect", `{}`, 403},               // prometheus by default
		{"t-wendy", "POST", "/api/services/connect", `{"KIND":"nginx"}`, 200}, // the handler matches any case
		{"t-wendy", "POST", "/api/services/connect", `{"kind":"nginx","KIND":"envoy"}`, 400},
		{"t-wendy", "POST", "/api/services/connect", `{"pad":"` + strings.Repeat("x", maxPeek) + `","kind":"nginx"}`, 413},
		{"t-victor", "POST", "/api/apps", `{"template":"x","TEMPLATE":"signoz"}`, 400},
		{"t-wendy", "POST", "/api/terraform/workspaces/w1/runs", "", 200}, // through the group
		{"t-wendy", "GET", "/api/terraform/runs", "", 200},
		{"t-wendy", "GET", "/api/admin/users", "", 403},
		{"t-victor", "GET", "/api/servers", "", 403}, // no servers, notifications or ansible tab
		{"t-wendy", "GET", "/api/gitlab/projects", "", 403},
		{"t-wendy", "GET", "/api/no/such/route", "", 404},
		{"t-victor", "GET", "/api/services/s-nginx", "", 200},
		{"t-victor", "POST", "/api/services/s-nginx/nginx/inspect", "", 200},
		{"t-victor", "POST", "/api/services/s-nginx/nginx/apply", "", 403}, // a viewer never writes
		{"t-victor", "POST", "/api/apps", `{"template":"signoz"}`, 403},
		{"t-victor", "GET", "/api/apps/a1", "", 200},
		{"t-victor", "POST", "/api/apps/envfile", "", 403}, // .env files are for writers
		{"t-root", "GET", "/api/admin/users", "", 200},
		{"t-root", "POST", "/api/services/envoy", "", 200},
		{"t-root", "DELETE", "/api/apps/a1", "", 200},
	}
	for _, c := range cases {
		reached, reachedBody = nil, ""
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		if c.token != "" {
			r.AddCookie(&http.Cookie{Name: CookieName, Value: c.token})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("%s %s %s as %s: %d %s; want %d", c.method, c.path, c.body, c.token, w.Code, strings.TrimSpace(w.Body.String()), c.want)
			continue
		}
		if c.want == 200 {
			if reached == nil {
				t.Errorf("%s %s: the handler didn't get the user's access", c.method, c.path)
			} else if reachedBody != c.body {
				t.Errorf("%s %s: the handler got body %q, want %q", c.method, c.path, reachedBody, c.body)
			}
		} else if reached != nil {
			t.Errorf("%s %s as %s: refused, yet the handler ran", c.method, c.path, c.token)
		}
	}
}

func TestGuardRefusalSaysWhere(t *testing.T) {
	g, _, _ := testGuard(t)
	r := httptest.NewRequest("DELETE", "/api/servers/x", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: "t-wendy"})
	w := httptest.NewRecorder()
	g.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "change anything in Servers › Servers") {
		t.Errorf("refusal = %q", w.Body.String())
	}
}

func TestResolverCachesUntilForget(t *testing.T) {
	_, store, resolver := testGuard(t)
	ctx := context.Background()
	a, err := resolver.Resolve(ctx, "wendy")
	if err != nil || !a.Can("deploy.nginx", Write) {
		t.Fatalf("Resolve = %+v, %v", a, err)
	}
	calls := store.calls
	resolver.Resolve(ctx, "wendy")
	if store.calls != calls {
		t.Error("a second Resolve should come from the cache")
	}
	store.grants["user:wendy"] = map[string]Level{}
	resolver.Forget()
	a, _ = resolver.Resolve(ctx, "wendy")
	if a.Can("deploy.nginx", View) {
		t.Error("after Forget the new grants apply")
	}
	// A change made around the API applies once the cache expires.
	store.grants["user:wendy"] = map[string]Level{"deploy.nginx": Write}
	resolver.mu.Lock()
	c := resolver.cache["wendy"]
	c.expires = time.Now().Add(-time.Second)
	resolver.cache["wendy"] = c
	resolver.mu.Unlock()
	if a, _ = resolver.Resolve(ctx, "wendy"); !a.Can("deploy.nginx", Write) {
		t.Error("an expired cache entry should be read again")
	}
	root, _ := resolver.Resolve(ctx, "root")
	if !root.IsAdmin() || !root.Builtin || !root.Can("admin.users", Write) {
		t.Errorf("a built-in account is an admin: %+v", root)
	}
}

func TestVerify(t *testing.T) {
	_, _, resolver := testGuard(t)
	ctx := context.Background()
	for _, c := range []struct {
		user, pw string
		want     bool
	}{
		{"root", "root's password", true},
		{"root", "wrong", false},
		{"wendy", "wendy's password", true},
		{"wendy", "root's password", false},
		{"gone", "gone's password", false}, // disabled
		{"nobody", "anything", false},
	} {
		if got := resolver.Verify(ctx, c.user, c.pw); got != c.want {
			t.Errorf("Verify(%s, %s) = %v", c.user, c.pw, got)
		}
	}
}

// web/lib/nav.ts names the same tabs, with the same pages, as Tabs.
func TestNavMatchesTabs(t *testing.T) {
	b, err := os.ReadFile("../../web/lib/nav.ts")
	if err != nil {
		t.Skip("web/lib/nav.ts isn't here:", err)
	}
	items := regexp.MustCompile(`\{ href: "([^"]+)", label: "[^"]+", tab: "([^"]+)" \}`).FindAllStringSubmatch(string(b), -1)
	if len(items) == 0 {
		t.Fatal("found no nav items in nav.ts")
	}
	inNav := map[string]bool{}
	for _, m := range items {
		href, key := m[1], m[2]
		tab, ok := TabByKey(key)
		if !ok {
			t.Errorf("nav.ts item %s names tab %q, which access.Tabs doesn't have", href, key)
			continue
		}
		if tab.Href != href {
			t.Errorf("tab %s is %s in access.Tabs but %s in nav.ts", key, tab.Href, href)
		}
		inNav[key] = true
	}
	for _, tab := range Tabs {
		if !inNav[tab.Key] && tab.Key != "overview" && tab.Key != "deploy.other" {
			t.Errorf("tab %s isn't in nav.ts", tab.Key)
		}
	}

	set := regexp.MustCompile(`(?s)VIEW_ONLY_TABS = new Set\(\[(.*?)\]\)`).FindStringSubmatch(string(b))
	if set == nil {
		t.Fatal("found no VIEW_ONLY_TABS in nav.ts")
	}
	viewOnly := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(set[1], -1) {
		viewOnly[m[1]] = true
	}
	for _, tab := range Tabs {
		if tab.ViewOnly != viewOnly[tab.Key] {
			t.Errorf("tab %s: view only is %v in access.Tabs, %v in nav.ts's VIEW_ONLY_TABS", tab.Key, tab.ViewOnly, viewOnly[tab.Key])
		}
	}
}
