// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package nginxctl

import (
	"strings"
	"testing"
)

const mainConf = `user  nginx;
worker_processes  auto;
error_log  /var/log/nginx/error.log notice;

events {
    worker_connections  1024;
}

http {
    include       /etc/nginx/mime.types;
    log_format  main  '$remote_addr - $remote_user [$time_local] "$request" '
                      '$status $body_bytes_sent';
    sendfile        on;
    keepalive_timeout  65;
    client_max_body_size 50m; # uploads
    include /etc/nginx/conf.d/*.conf;
}

stream {
    server {
        listen 5432;
        proxy_pass db.internal:5432;
    }
}
`

const siteConf = `# API behind a pool
upstream api_pool {
    least_conn;
    server 10.0.0.5:8080 weight=3;
    server 10.0.0.6:8080 backup;
    keepalive 32;
}

server {
    listen 80;
    server_name api.example.com www.api.example.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;
    ssl_certificate /etc/nginx/certs/api.crt;
    ssl_certificate_key /etc/nginx/certs/api.key;
    set $upstream "api_pool";
    location / {
        proxy_pass http://api_pool;
        proxy_set_header Host ${host};
    }
    location = /healthz { return 200 "ok"; }
    location /static/ {
        alias /srv/static/;
        location ~* \.(css|js)$ { expires 7d; }
    }
}
`

func TestParseAndBuild(t *testing.T) {
	ov := Build(map[string]string{
		"/etc/nginx/nginx.conf":         mainConf,
		"/etc/nginx/conf.d/api.conf":    siteConf,
		"/etc/nginx/conf.d/broken.conf": "server { listen 80;",
	}, "/etc/nginx/nginx.conf")

	g := ov.Globals
	if g.WorkerProcesses != "auto" || g.WorkerConnections != "1024" || g.ClientMaxBodySize != "50m" || g.KeepaliveTimeout != "65" {
		t.Errorf("globals = %+v", g)
	}
	if len(g.LogFormats) != 1 || g.LogFormats[0] != "main" || len(g.Includes) != 2 {
		t.Errorf("log formats / includes = %v / %v", g.LogFormats, g.Includes)
	}

	if len(ov.Servers) != 3 {
		t.Fatalf("servers = %d, want 3 (2 http + 1 stream): %+v", len(ov.Servers), ov.Servers)
	}
	redirect, secure, stream := ov.Servers[0], ov.Servers[1], ov.Servers[2]
	if redirect.Return != "301 https://$host$request_uri" || len(redirect.Names) != 2 || redirect.SSL {
		t.Errorf("redirect server = %+v", redirect)
	}
	if !secure.SSL || secure.Certificate != "/etc/nginx/certs/api.crt" || len(secure.Locations) != 4 {
		t.Errorf("https server = %+v", secure)
	}
	if secure.Locations[0].ProxyPass != "http://api_pool" || secure.Locations[1].Return != "200 ok" {
		t.Errorf("locations = %+v", secure.Locations)
	}
	if secure.Locations[3].Match != `/static/ › ~* \.(css|js)$` {
		t.Errorf("nested location = %q", secure.Locations[3].Match)
	}
	if stream.Context != "stream" || stream.ProxyPass != "db.internal:5432" {
		t.Errorf("stream server = %+v", stream)
	}

	if len(ov.Upstreams) != 1 {
		t.Fatalf("upstreams = %+v", ov.Upstreams)
	}
	u := ov.Upstreams[0]
	if u.Name != "api_pool" || u.Method != "least_conn" || len(u.Servers) != 2 || u.Servers[0] != "10.0.0.5:8080 weight=3" || u.Keepalive != "32" {
		t.Errorf("upstream = %+v", u)
	}
	if len(ov.ParseErrors) != 1 || !strings.Contains(ov.ParseErrors[0], "broken.conf") {
		t.Errorf("parse errors = %v", ov.ParseErrors)
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"server {", "}", "listen 80", `return "unterminated;`} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted invalid config", bad)
		}
	}
}

func TestRenderedConfigsParse(t *testing.T) {
	specs := []ServerSpec{
		{Names: []string{"app.example.com"}, Kind: "proxy", Upstream: "http://app:3000", Websocket: true, MaxBodySize: "20m", Timeout: "120s"},
		{Names: []string{"shop.example.com www.shop.example.com"}, Kind: "loadbalance", Backends: []string{"10.0.0.1:8080 weight=2", "10.0.0.2:8080", ""}, Method: "least_conn",
			CertPath: "/etc/nginx/certs/shop.crt", KeyPath: "/etc/nginx/certs/shop.key", ForceHTTPS: true},
		{Names: []string{"old.example.com"}, Kind: "redirect", RedirectTo: "https://new.example.com/"},
	}
	for _, sp := range specs {
		if err := sp.Validate(); err != nil {
			t.Fatalf("Validate(%+v): %v", sp, err)
		}
		out := sp.Render()
		ov := Build(map[string]string{"/etc/nginx/conf.d/x.conf": out}, "/etc/nginx/nginx.conf")
		if len(ov.ParseErrors) > 0 {
			t.Fatalf("rendered config doesn't parse: %v\n%s", ov.ParseErrors, out)
		}
		switch sp.Kind {
		case "proxy":
			if !strings.Contains(out, "proxy_pass http://app:3000;") || !strings.Contains(out, "Upgrade $http_upgrade") || !strings.Contains(out, "client_max_body_size 20m;") {
				t.Errorf("proxy config:\n%s", out)
			}
		case "loadbalance":
			if len(ov.Upstreams) != 1 || ov.Upstreams[0].Name != "shop_example_com_backend" || len(ov.Upstreams[0].Servers) != 2 || ov.Upstreams[0].Method != "least_conn" {
				t.Errorf("upstream = %+v\n%s", ov.Upstreams, out)
			}
			if len(ov.Servers) != 2 || ov.Servers[0].Return != "301 https://$host$request_uri" || !ov.Servers[1].SSL {
				t.Errorf("https servers = %+v", ov.Servers)
			}
			if len(sp.Names) != 2 {
				t.Errorf("names not split: %v", sp.Names)
			}
		case "redirect":
			if !strings.Contains(out, "return 301 https://new.example.com$request_uri;") {
				t.Errorf("redirect config:\n%s", out)
			}
		}
	}
}

func TestRenderSecurityHeaders(t *testing.T) {
	sp := ServerSpec{
		Names: []string{"app.example.com"}, Kind: "proxy", Upstream: "http://app:3000",
		CertPath: "/etc/nginx/certs/app.crt", KeyPath: "/etc/nginx/certs/app.key", ForceHTTPS: true,
		HSTS: true, XFrameOptions: "SAMEORIGIN", NoSniff: true, ReferrerPolicy: "strict-origin-when-cross-origin",
		ContentSecurityPolicy: "default-src 'self'", PermissionsPolicy: "geolocation=(), camera=()",
		CORS: true, CORSOrigin: "https://app.example.com", CORSCredentials: true,
	}
	if err := sp.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	out := sp.Render()
	ov := Build(map[string]string{"/etc/nginx/conf.d/x.conf": out}, "/etc/nginx/nginx.conf")
	if len(ov.ParseErrors) > 0 {
		t.Fatalf("rendered config doesn't parse: %v\n%s", ov.ParseErrors, out)
	}

	// The point of detection: what Render() writes must come back out of
	// Build() too, the same way it would for a server block someone wrote
	// by hand — not just parse without error.
	if len(ov.Servers) != 2 {
		t.Fatalf("expected 2 servers (HTTP redirect + HTTPS), got %d", len(ov.Servers))
	}
	httpSrv, httpsSrv := ov.Servers[0], ov.Servers[1]
	if len(httpSrv.Headers) != 0 {
		t.Errorf("plain-HTTP server should have no detected headers, got %+v", httpSrv.Headers)
	}
	// Non-CORS headers apply to the whole server, so they're declared (and
	// so detected) at server level.
	want := map[string]string{
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
		"X-Frame-Options":           "SAMEORIGIN",
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Content-Security-Policy":   "default-src 'self'",
		"Permissions-Policy":        "geolocation=(), camera=()",
	}
	got := map[string]string{}
	for _, h := range httpsSrv.Headers {
		got[h.Name] = h.Value
		if !h.Always {
			t.Errorf("detected header %s should have Always=true", h.Name)
		}
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("detected server-level header %s = %q, want %q (all detected: %+v)", name, got[name], value, httpsSrv.Headers)
		}
	}
	// CORS headers only make sense per-request-path, so they're declared
	// (and so detected) on the location, not the server.
	if len(httpsSrv.Locations) != 1 {
		t.Fatalf("expected 1 location, got %+v", httpsSrv.Locations)
	}
	locHeaders := map[string]string{}
	for _, h := range httpsSrv.Locations[0].Headers {
		locHeaders[h.Name] = h.Value
	}
	if locHeaders["Access-Control-Allow-Origin"] != "https://app.example.com" {
		t.Errorf("detected location header Access-Control-Allow-Origin = %q, want the configured origin (all: %+v)",
			locHeaders["Access-Control-Allow-Origin"], httpsSrv.Locations[0].Headers)
	}
	if locHeaders["Access-Control-Allow-Credentials"] != "true" {
		t.Errorf("detected location header Access-Control-Allow-Credentials = %q, want \"true\" (all: %+v)",
			locHeaders["Access-Control-Allow-Credentials"], httpsSrv.Locations[0].Headers)
	}

	httpBlock, httpsBlock, ok := strings.Cut(out, "listen 443")
	if !ok {
		t.Fatalf("expected an HTTPS server block:\n%s", out)
	}
	if strings.Contains(httpBlock, "Strict-Transport-Security") {
		t.Errorf("HSTS must not appear on the plain-HTTP (redirect-only) block:\n%s", httpBlock)
	}
	for _, want := range []string{
		`Strict-Transport-Security "max-age=63072000; includeSubDomains" always;`,
		"X-Frame-Options SAMEORIGIN always;",
		"X-Content-Type-Options nosniff always;",
		"Referrer-Policy strict-origin-when-cross-origin always;",
		`Content-Security-Policy "default-src 'self'" always;`,
		`Permissions-Policy "geolocation=(), camera=()" always;`,
		`Access-Control-Allow-Origin "https://app.example.com" always;`,
		`Access-Control-Allow-Methods "GET, POST, PUT, PATCH, DELETE, OPTIONS" always;`,
		`Access-Control-Allow-Credentials "true" always;`,
		"if ($request_method = OPTIONS) {",
		"return 204;",
	} {
		if !strings.Contains(httpsBlock, want) {
			t.Errorf("missing %q in HTTPS block:\n%s", want, httpsBlock)
		}
	}

	// The regression this guards against: nginx's "if" does not inherit
	// add_header from a PARENT server/location — verified against a real
	// nginx container, where CORS headers declared at server level simply
	// never reached a "return" inside a nested "if". Declaring them in the
	// *same* location block as the "if" (rather than the parent server
	// block, and rather than duplicating them inside the "if" itself) does
	// work — also verified against a real container — so that's the
	// layout to protect: the CORS lines must come before the "if", in the
	// same location.
	corsIdx := strings.Index(httpsBlock, `Access-Control-Allow-Origin "https://app.example.com" always;`)
	locationIdx := strings.Index(httpsBlock, "location / {")
	ifIdx := strings.Index(httpsBlock, "if ($request_method")
	if corsIdx < 0 || locationIdx < 0 || ifIdx < 0 || !(locationIdx < corsIdx && corsIdx < ifIdx) {
		t.Errorf("Access-Control-Allow-Origin must be declared inside \"location /\", before the OPTIONS \"if\":\n%s", httpsBlock)
	}
}

func TestMultiOriginCORSMap(t *testing.T) {
	sp := ServerSpec{
		Names: []string{"api.example.com"}, Kind: "proxy", Upstream: "http://app:3000",
		CORS: true, CORSOrigin: "https://app.example.com, https://staging.example.com", CORSCredentials: true,
	}
	if err := sp.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	out := sp.Render()
	ov := Build(map[string]string{"/etc/nginx/conf.d/x.conf": out}, "/etc/nginx/nginx.conf")
	if len(ov.ParseErrors) > 0 {
		t.Fatalf("rendered config doesn't parse: %v\n%s", ov.ParseErrors, out)
	}

	if !strings.Contains(out, "map $http_origin $cors_origin_api_example_com {") {
		t.Errorf("expected a map block for the multi-origin case:\n%s", out)
	}
	if !strings.Contains(out, `default "";`) {
		t.Errorf("map must default to empty (deny) for an unlisted origin:\n%s", out)
	}
	if !strings.Contains(out, "https://app.example.com $http_origin;") || !strings.Contains(out, "https://staging.example.com $http_origin;") {
		t.Errorf("map must list both configured origins:\n%s", out)
	}
	if !strings.Contains(out, `Access-Control-Allow-Origin "$cors_origin_api_example_com" always;`) {
		t.Errorf("Access-Control-Allow-Origin must reference the mapped variable:\n%s", out)
	}
	if !strings.Contains(out, "add_header Vary Origin always;") {
		t.Errorf("a dynamic origin needs Vary: Origin so caches don't mix up clients:\n%s", out)
	}
	// The map must be defined before it's used.
	if strings.Index(out, "map $http_origin") > strings.Index(out, "Access-Control-Allow-Origin") {
		t.Errorf("map block must come before its use:\n%s", out)
	}
}

func TestHeaderRedefinitionWarnings(t *testing.T) {
	config := `
server {
    listen 80;
    server_name app.example.com;

    add_header X-Frame-Options SAMEORIGIN always;
    add_header X-Content-Type-Options nosniff always;

    location / {
        try_files $uri $uri/ =404;
    }

    location /api/ {
        add_header Access-Control-Allow-Origin "*" always;
        proxy_pass http://backend;
    }
}
`
	ov := Build(map[string]string{"/etc/nginx/conf.d/app.conf": config}, "/etc/nginx/nginx.conf")
	if len(ov.ParseErrors) > 0 {
		t.Fatalf("config didn't parse: %v", ov.ParseErrors)
	}
	if len(ov.HeaderWarnings) != 1 {
		t.Fatalf("expected exactly 1 warning (only /api/ redefines headers; / defines none of its own so it inherits cleanly), got %+v", ov.HeaderWarnings)
	}
	w := ov.HeaderWarnings[0]
	for _, want := range []string{"/api/", "X-Content-Type-Options", "X-Frame-Options"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning missing %q: %s", want, w)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	bad := []ServerSpec{
		{Kind: "proxy", Upstream: "http://a:1"},                                  // no names
		{Names: []string{"a.com;"}, Kind: "proxy", Upstream: "http://a:1"},       // injection attempt
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "app:3000"},          // not a URL
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1; evil"},  // injection attempt
		{Names: []string{"a.com"}, Kind: "loadbalance", Backends: []string{"a"}}, // no port
		{Names: []string{"a.com"}, Kind: "redirect", RedirectTo: "https://b", RedirectCode: 200},
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", CertPath: "/c.crt"}, // key missing
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", MaxBodySize: "lots"},
		{Names: []string{"a.com"}, Kind: "teleport"},
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", XFrameOptions: "NOPE"},
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", ReferrerPolicy: "whenever-i-feel-like-it"},
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", ContentSecurityPolicy: `default-src "self"`}, // embedded quote
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", CORS: true, CORSOrigin: "not a url"},
		{Names: []string{"a.com"}, Kind: "proxy", Upstream: "http://a:1", CORS: true, CORSCredentials: true}, // wildcard origin + credentials
	}
	for _, sp := range bad {
		if err := sp.Validate(); err == nil {
			t.Errorf("accepted %+v", sp)
		}
	}
}

func TestFileNameAndNewFileDir(t *testing.T) {
	if got := FileName([]string{"*.Example.com"}); got != "example.com.conf" {
		t.Errorf("FileName = %s", got)
	}
	if got := FileName([]string{"_"}); got != "server.conf" {
		t.Errorf("FileName(_) = %s", got)
	}
	if got := newFileDir([]string{"/etc/nginx/mime.types", "/etc/nginx/conf.d/*.conf"}); got != "/etc/nginx/conf.d" {
		t.Errorf("newFileDir = %s", got)
	}
	if got := newFileDir([]string{"sites-enabled/*"}); got != "/etc/nginx/sites-enabled" {
		t.Errorf("relative sites-enabled = %s", got)
	}
	if got := newFileDir([]string{"/srv/nginx/vhosts/*.conf"}); got != "/srv/nginx/vhosts" {
		t.Errorf("custom include dir = %s", got)
	}
	if !IsNginxImage("nginxinc/nginx-unprivileged:1.27") || !IsNginxImage("openresty/openresty:alpine") || IsNginxImage("prom/prometheus") {
		t.Error("IsNginxImage")
	}
}

func TestLastLinePrefersEmerg(t *testing.T) {
	out := "nginx: [emerg] unknown directive \"proxy_passs\" in /etc/nginx/conf.d/x.conf:5\nnginx: configuration file /etc/nginx/nginx.conf test failed"
	if got := lastLine(out); got != `[emerg] unknown directive "proxy_passs" in /etc/nginx/conf.d/x.conf:5` {
		t.Errorf("lastLine = %q", got)
	}
}

func TestParseKeepsEscapes(t *testing.T) {
	ds, err := Parse(`return 200 "ok\n"; add_header X "a\"b";`)
	if err != nil {
		t.Fatal(err)
	}
	if ds[0].Args[1] != `ok\n` || ds[1].Args[1] != `a"b` {
		t.Errorf("args = %q / %q", ds[0].Args, ds[1].Args)
	}
}
