// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package haproxyctl

import (
	"reflect"
	"strings"
	"testing"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/dockerx"
)

func TestParseFileRoutesTLSAndSettings(t *testing.T) {
	cfg := `global
    maxconn 4096

defaults
    mode http
    timeout client 30s
    option httplog

frontend main
    bind *:80
    bind *:443 ssl crt /certs/site.pem
    acl host_app req.hdr(host),field(1,:) -i App.Example.com www.example.com
    acl is_api path_beg /api
    use_backend site_app if host_app
    use_backend api if is_api
    use_backend inline if { hdr(host) -i inline.example.com }
    use_backend others unless host_app
    default_backend fallback

backend site_app
    option httpchk GET /health
    http-request redirect scheme https code 301 unless { ssl_fc }
    server srv1 10.0.0.1:80 check
`
	secs := parseFile("/etc/haproxy/haproxy.cfg", cfg)
	if len(secs) != 4 {
		t.Fatalf("len(sections) = %d, want 4", len(secs))
	}
	if want := []Setting{{"maxconn", "4096"}}; !reflect.DeepEqual(secs[0].settings, want) {
		t.Errorf("global settings = %+v", secs[0].settings)
	}
	if want := []Setting{{"mode", "http"}, {"timeout client", "30s"}, {"option", "httplog"}}; !reflect.DeepEqual(secs[1].settings, want) {
		t.Errorf("defaults settings = %+v", secs[1].settings)
	}

	fe := secs[2]
	if fe.file != "/etc/haproxy/haproxy.cfg" || fe.line != 9 {
		t.Errorf("frontend at %s:%d, want /etc/haproxy/haproxy.cfg:9", fe.file, fe.line)
	}
	if !reflect.DeepEqual(fe.binds, []string{"*:80", "*:443"}) || !reflect.DeepEqual(fe.tlsBinds, []string{"*:443"}) ||
		!reflect.DeepEqual(fe.certs, []string{"/certs/site.pem"}) {
		t.Errorf("binds = %v, tls = %v, certs = %v", fe.binds, fe.tlsBinds, fe.certs)
	}
	want := []Route{
		{Backend: "site_app", Hosts: []string{"app.example.com", "www.example.com"}, Condition: "if host_app"},
		{Backend: "api", Condition: "if is_api"},
		{Backend: "inline", Hosts: []string{"inline.example.com"}, Condition: "if { hdr(host) -i inline.example.com }"},
		{Backend: "others", Condition: "unless host_app"},
		{Backend: "fallback", Default: true},
	}
	if got := fe.routes(); !reflect.DeepEqual(got, want) {
		t.Errorf("routes =\n%+v\nwant\n%+v", got, want)
	}

	be := secs[3]
	if be.healthCheck != "httpchk GET /health" || be.redirect != "scheme https code 301 unless { ssl_fc }" {
		t.Errorf("backend healthCheck = %q, redirect = %q", be.healthCheck, be.redirect)
	}
}

const mainCfg = "/usr/local/etc/haproxy/haproxy.cfg"

func seedFiles() []dockerx.File {
	return []dockerx.File{{Path: mainCfg, Content: deploy.HAProxyDefaultConfig, SHA: "seed"}}
}

func render(t *testing.T, spec SiteSpec, files []dockerx.File) ([]SiteChange, string) {
	t.Helper()
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	changes, summary, err := spec.Render(files, mainCfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return changes, summary
}

func routesOf(t *testing.T, content, frontend string) []Route {
	t.Helper()
	for _, s := range parseFile(mainCfg, content) {
		if s.kind == "frontend" && s.name == frontend {
			return s.routes()
		}
	}
	t.Fatalf("frontend %s not found in:\n%s", frontend, content)
	return nil
}

func TestRenderProxySiteAddsHostRuleBeforeDefault(t *testing.T) {
	changes, summary := render(t, SiteSpec{Kind: "proxy", Names: []string{"App.example.com www.app.example.com"}, Upstream: "http://app:3000/"}, seedFiles())
	if len(changes) != 1 || changes[0].Path != mainCfg || changes[0].BaseSHA != "seed" {
		t.Fatalf("changes = %+v", changes)
	}
	after := changes[0].After
	want := []Route{
		{Backend: "site_app_example_com", Hosts: []string{"app.example.com", "www.app.example.com"}, Condition: "if host_app_example_com"},
		{Backend: "no_backend", Default: true},
	}
	if got := routesOf(t, after, "main"); !reflect.DeepEqual(got, want) {
		t.Errorf("routes = %+v", got)
	}
	if !strings.Contains(after, "backend site_app_example_com\n    mode http\n    server srv1 app:3000 check\n") {
		t.Errorf("backend block missing or wrong:\n%s", after)
	}
	if !strings.HasSuffix(after, "\n") {
		t.Error("file must end with a newline")
	}
	if !strings.Contains(summary, "app.example.com") || !strings.Contains(summary, "frontend main") {
		t.Errorf("summary = %q", summary)
	}
}

// Sites the panel adds stay grouped, in the order they were added, each
// acl beside its use_backend — and ahead of hand-written rules.
func TestRenderKeepsSitesTogetherInOrder(t *testing.T) {
	cfg := deploy.HAProxyDefaultConfig
	for _, name := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		changes, _ := render(t, SiteSpec{Kind: "proxy", Names: []string{name}, Upstream: "app:3000"}, []dockerx.File{{Path: mainCfg, Content: cfg}})
		cfg = changes[0].After
	}
	want := "    monitor-uri /healthz\n" +
		"    acl host_a_example_com req.hdr(host),field(1,:) -i a.example.com\n" +
		"    use_backend site_a_example_com if host_a_example_com\n" +
		"    acl host_b_example_com req.hdr(host),field(1,:) -i b.example.com\n" +
		"    use_backend site_b_example_com if host_b_example_com\n" +
		"    acl host_c_example_com req.hdr(host),field(1,:) -i c.example.com\n" +
		"    use_backend site_c_example_com if host_c_example_com\n" +
		"    default_backend no_backend\n"
	if !strings.Contains(cfg, want) {
		t.Errorf("frontend not in added order:\n%s", cfg)
	}

	handWritten := "defaults\n    mode http\nfrontend main\n    bind *:80\n    acl is_api path_beg /api\n    use_backend api if is_api\n    default_backend x\n"
	changes, _ := render(t, SiteSpec{Kind: "proxy", Names: []string{"a.example.com"}, Upstream: "app:3000"}, []dockerx.File{{Path: mainCfg, Content: handWritten}})
	if got := routesOf(t, changes[0].After, "main"); got[0].Backend != "site_a_example_com" {
		t.Errorf("a new site must be checked before hand-written rules, routes = %+v", got)
	}
}

func TestRenderWithoutDomainsReplacesDefaultBackend(t *testing.T) {
	changes, summary := render(t, SiteSpec{Kind: "proxy", Upstream: "app:3000"}, seedFiles())
	got := routesOf(t, changes[0].After, "main")
	if len(got) != 1 || got[0].Backend != "site_default_80" || !got[0].Default {
		t.Errorf("routes = %+v", got)
	}
	if !strings.Contains(summary, "replaces default_backend no_backend") {
		t.Errorf("summary = %q", summary)
	}
}

func TestRenderLoadBalancerAndRedirect(t *testing.T) {
	changes, _ := render(t, SiteSpec{
		Kind: "loadbalance", Names: []string{"api.example.com"}, Balance: "leastconn", HealthCheckPath: "/health",
		Backends: []string{"10.0.0.5:8080 weight=2", "", "10.0.0.6:8080", "10.0.0.7:8080 backup"},
	}, seedFiles())
	for _, want := range []string{
		"    balance leastconn\n", "    option httpchk GET /health\n",
		"    server srv1 10.0.0.5:8080 check weight 2\n", "    server srv2 10.0.0.6:8080 check\n", "    server srv3 10.0.0.7:8080 check backup\n",
	} {
		if !strings.Contains(changes[0].After, want) {
			t.Errorf("missing %q in:\n%s", want, changes[0].After)
		}
	}

	changes, _ = render(t, SiteSpec{Kind: "redirect", Names: []string{"old.example.com"}, RedirectTo: "https://new.example.com/"}, seedFiles())
	if !strings.Contains(changes[0].After, "    http-request redirect prefix https://new.example.com code 301\n") {
		t.Errorf("redirect missing:\n%s", changes[0].After)
	}
}

func TestRenderHTTPSCreatesTLSFrontendOrAddsCert(t *testing.T) {
	spec := SiteSpec{Kind: "proxy", Names: []string{"app.example.com"}, Upstream: "app:3000", CertPath: "/certs/app.pem", ForceHTTPS: true, HSTS: true}
	changes, _ := render(t, spec, seedFiles())
	after := changes[0].After
	for _, want := range []string{
		"frontend https_443\n    bind *:443 ssl crt /certs/app.pem\n    mode http\n",
		"    http-request redirect scheme https code 301 unless { ssl_fc }\n",
		`    http-response set-header Strict-Transport-Security "max-age=31536000; includeSubDomains" if { ssl_fc }` + "\n",
	} {
		if !strings.Contains(after, want) {
			t.Errorf("missing %q in:\n%s", want, after)
		}
	}
	if got := routesOf(t, after, "https_443"); len(got) != 1 || got[0].Backend != "site_app_example_com" {
		t.Errorf("https routes = %+v", got)
	}

	// With a TLS frontend already there, the site joins it and its cert is
	// added to the existing bind.
	existing := deploy.HAProxyDefaultConfig + "\nfrontend tls\n    bind *:443 ssl crt /certs/other.pem\n    mode http\n    default_backend no_backend\n"
	changes, _ = render(t, spec, []dockerx.File{{Path: mainCfg, Content: existing}})
	after = changes[0].After
	if !strings.Contains(after, "    bind *:443 ssl crt /certs/other.pem crt /certs/app.pem\n") {
		t.Errorf("cert not added to existing bind:\n%s", after)
	}
	if strings.Contains(after, "frontend https_443") {
		t.Error("a second TLS frontend was created")
	}
	if got := routesOf(t, after, "tls"); len(got) != 2 || got[0].Backend != "site_app_example_com" {
		t.Errorf("tls routes = %+v", got)
	}
}

func TestRenderNewPortCreatesFrontend(t *testing.T) {
	changes, _ := render(t, SiteSpec{Kind: "proxy", Names: []string{"app.example.com"}, Upstream: "app:3000", ListenPort: 8080}, seedFiles())
	if got := routesOf(t, changes[0].After, "http_8080"); len(got) != 1 {
		t.Errorf("routes = %+v", got)
	}
}

func TestRenderEditsTheFileHoldingTheFrontend(t *testing.T) {
	files := []dockerx.File{
		{Path: mainCfg, Content: "global\n    maxconn 100\n\ndefaults\n    mode http\n", SHA: "a"},
		{Path: "/usr/local/etc/haproxy/conf.d/web.cfg", Content: "frontend web\n    bind *:80\n    default_backend x\n\nbackend x\n    server s 1.2.3.4:80\n", SHA: "b"},
	}
	changes, _ := render(t, SiteSpec{Kind: "proxy", Names: []string{"app.example.com"}, Upstream: "app:3000"}, files)
	if len(changes) != 1 || changes[0].Path != files[1].Path || changes[0].BaseSHA != "b" {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestRenderRefusals(t *testing.T) {
	// The same site twice.
	changes, _ := render(t, SiteSpec{Kind: "proxy", Names: []string{"app.example.com"}, Upstream: "app:3000"}, seedFiles())
	spec := SiteSpec{Kind: "proxy", Names: []string{"app.example.com"}, Upstream: "app:3000"}
	spec.Validate()
	if _, _, err := spec.Render([]dockerx.File{{Path: mainCfg, Content: changes[0].After}}, mainCfg); err == nil {
		t.Error("adding the same site twice should fail")
	}

	// A TCP-mode frontend on the port can't route by domain.
	tcp := "defaults\n    mode tcp\nfrontend db\n    bind *:80\n    default_backend x\n"
	if _, _, err := spec.Render([]dockerx.File{{Path: mainCfg, Content: tcp}}, mainCfg); err == nil || !strings.Contains(err.Error(), "TCP mode") {
		t.Errorf("tcp frontend: err = %v", err)
	}

	// The stats listener isn't a site frontend.
	onStats := SiteSpec{Kind: "proxy", Names: []string{"a.example.com"}, Upstream: "app:3000", ListenPort: deploy.HAProxyStatsPort}
	onStats.Validate()
	if _, _, err := onStats.Render(seedFiles(), mainCfg); err == nil || !strings.Contains(err.Error(), "stats page") {
		t.Errorf("stats port: err = %v", err)
	}
}

func TestSiteSpecValidateRejects(t *testing.T) {
	for name, spec := range map[string]SiteSpec{
		"bad domain":       {Kind: "proxy", Names: []string{"exa mple!.com"}, Upstream: "app:3000"},
		"no port":          {Kind: "proxy", Upstream: "app"},
		"https upstream":   {Kind: "proxy", Upstream: "https://app:3000"},
		"no backends":      {Kind: "loadbalance", Backends: []string{" ", ""}},
		"bad backend opt":  {Kind: "loadbalance", Backends: []string{"a:1 max_fails=3"}},
		"bad redirect":     {Kind: "redirect", RedirectTo: "example.com"},
		"quote in header":  {Kind: "proxy", Upstream: "app:3000", ContentSecurityPolicy: `default-src "self"`},
		"cert on 443":      {Kind: "proxy", Upstream: "app:3000", CertPath: "/c.pem", ListenPort: 443},
		"relative cert":    {Kind: "proxy", Upstream: "app:3000", CertPath: "c.pem"},
		"bad timeout":      {Kind: "proxy", Upstream: "app:3000", Timeout: "soon"},
		"unknown kind":     {Kind: "tunnel"},
		"bad health check": {Kind: "proxy", Upstream: "app:3000", HealthCheckPath: "health"},
	} {
		if err := spec.Validate(); err == nil {
			t.Errorf("%s: Validate accepted %+v", name, spec)
		}
	}
}

func TestHeaderValuesEscapePercent(t *testing.T) {
	spec := SiteSpec{Kind: "proxy", Upstream: "app:3000", ContentSecurityPolicy: "img-src 100% data:"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(spec.headerLines(), "\n"); got != `    http-response set-header Content-Security-Policy "img-src 100%% data:"` {
		t.Errorf("headerLines = %s", got)
	}
}
