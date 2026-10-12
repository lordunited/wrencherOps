// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package envoyctl

import (
	"os"
	"strings"
	"testing"

	"devops-wrapper/internal/deploy"
)

// Real /clusters?format=json from envoyproxy/envoy:v1.39 with its backend
// up, then stopped (its active health check failing).
func TestParseClusters(t *testing.T) {
	for file, want := range map[string]struct {
		healthy bool
		status  string
	}{
		"testdata/clusters_healthy.json": {true, "healthy"},
		"testdata/clusters_down.json":    {false, "failed active health check"},
	} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := parseClusters(b)
		if err != nil || len(cs) != 1 || cs[0].Name != "web" || len(cs[0].Endpoints) != 1 {
			t.Fatalf("%s: %+v %v", file, cs, err)
		}
		e := cs[0].Endpoints[0]
		if e.Healthy != want.healthy || e.Status != want.status || !strings.HasSuffix(e.Address, ":80") {
			t.Errorf("%s: endpoint %+v, want healthy=%v %q", file, e, want.healthy, want.status)
		}
	}
	ok, detail := summarize(true, "127.0.0.1:9901", "LIVE", "", mustClusters(t, "testdata/clusters_down.json"))
	if ok || !strings.Contains(detail, "0/1 endpoints healthy") || !strings.Contains(detail, "web 172.20.0.2:80 (failed active health check)") {
		t.Errorf("summary with a down endpoint: %v %q", ok, detail)
	}
	if ok, detail := summarize(true, "127.0.0.1:9901", "LIVE", "", mustClusters(t, "testdata/clusters_healthy.json")); !ok || detail != "Envoy live, 1/1 endpoints healthy" {
		t.Errorf("healthy summary: %v %q", ok, detail)
	}
}

func mustClusters(t *testing.T, f string) []LiveCluster {
	b, _ := os.ReadFile(f)
	cs, err := parseClusters(b)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

const sample = `# edge proxy
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 9901 }
static_resources:
  listeners:
  - name: main
    address:
      socket_address: { address: 0.0.0.0, port_value: 10000 }
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          route_config:
            virtual_hosts:
            - name: default
              domains: ["*"]
              routes:
              - match: { path: "/healthz" }
                direct_response: { status: 200 }
              - match: { prefix: "/" }
                route: { cluster: web }
  - name: db
    address:
      socket_address: { address: 0.0.0.0, port_value: 5432 }
    filter_chains:
    - filters:
      - name: envoy.filters.network.tcp_proxy
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
          stat_prefix: db
          cluster: pg
  clusters:
  - name: web
    type: STRICT_DNS
    load_assignment:
      cluster_name: web
      endpoints:
      - lb_endpoints:
        - endpoint: { address: { socket_address: { address: web-1, port_value: 8080 } } }
        - endpoint: { address: { socket_address: { address: web-2, port_value: 8080 } } }
    health_checks:
    - interval: 5s
      http_health_check: { path: /ping }
  - name: pg
    load_assignment:
      cluster_name: pg
      endpoints:
      - lb_endpoints:
        - endpoint: { address: { socket_address: { address: 10.0.0.9, port_value: 5432 } } }
`

func TestParseConfig(t *testing.T) {
	c := ParseConfig(sample)
	if c.ParseError != "" || c.Admin != "127.0.0.1:9901" || len(c.Listeners) != 2 || len(c.Clusters) != 2 {
		t.Fatalf("%+v", c)
	}
	http := c.Listeners[0]
	if http.Kind != "http" || http.Port != 10000 || len(http.Routes) != 2 || http.Routes[0].Target != "answers 200 itself" || http.Routes[1].Cluster != "web" || http.Routes[1].Match != "prefix /" {
		t.Errorf("http listener: %+v", http)
	}
	if tcp := c.Listeners[1]; tcp.Kind != "tcp" || tcp.TCPCluster != "pg" {
		t.Errorf("tcp listener: %+v", tcp)
	}
	if w := c.Clusters[0]; w.Type != "STRICT_DNS" || len(w.Endpoints) != 2 || w.Endpoints[1] != "web-2:8080" || w.HealthCheck != "HTTP /ping every 5s" {
		t.Errorf("web cluster: %+v", w)
	}
	if pg := c.Clusters[1]; pg.Type != "STATIC" || pg.LBPolicy != "ROUND_ROBIN" {
		t.Errorf("defaults: %+v", pg)
	}
	if c := ParseConfig("static_resources: [unclosed"); c.ParseError == "" {
		t.Error("broken YAML should report a parse error")
	}
	if c := ParseConfig("dynamic_resources:\n  cds_config: {}\n  lds_config: {}\n"); !strings.Contains(c.Dynamic, "clusters (CDS), listeners (LDS)") {
		t.Errorf("dynamic: %q", c.Dynamic)
	}
}

func TestRenderRoute(t *testing.T) {
	// A new domain: a new virtual host, and a STRICT_DNS cluster with a check.
	spec := RouteSpec{Domains: []string{"shop.example.com"}, PathPrefix: "/", Upstreams: []string{"shop:3000", "10.0.0.5:3000"}, HealthPath: "/health"}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	out, summary, err := spec.Render(sample)
	if err != nil {
		t.Fatal(err)
	}
	c := ParseConfig(out)
	if len(c.Clusters) != 3 || c.Clusters[2].Name != "shop_example_com" || c.Clusters[2].Type != "STRICT_DNS" || len(c.Clusters[2].Endpoints) != 2 || c.Clusters[2].HealthCheck == "" {
		t.Errorf("cluster: %+v", c.Clusters)
	}
	var shop *Route
	for i, r := range c.Listeners[0].Routes {
		if r.Cluster == "shop_example_com" {
			shop = &c.Listeners[0].Routes[i]
		}
	}
	if shop == nil || shop.Domains[0] != "shop.example.com" || shop.Match != "prefix /" {
		t.Errorf("route: %+v", c.Listeners[0].Routes)
	}
	if !strings.Contains(out, "cluster: shop_example_com\n") {
		t.Errorf("the route's cluster should be written plain:\n%s", out)
	}
	if !strings.Contains(out, "# edge proxy") || !strings.Contains(summary, "New virtual host for shop.example.com") {
		t.Errorf("comments kept / summary: %q\n%s", summary, out)
	}

	// Same domains as an existing virtual host ("*"): the route goes into it,
	// ahead of its catch-all prefix "/".
	api := RouteSpec{Domains: []string{"*"}, PathPrefix: "/api", Upstreams: []string{"10.0.0.7:8000"}}
	api.Validate()
	out, _, err = api.Render(sample)
	if err != nil {
		t.Fatal(err)
	}
	routes := ParseConfig(out).Listeners[0].Routes
	if len(routes) != 3 || routes[1].Match != "prefix /api" || routes[1].Cluster != "any_api" || routes[2].Match != "prefix /" {
		t.Errorf("routes in *: %+v", routes)
	}
	if ParseConfig(out).Clusters[2].Type != "STATIC" {
		t.Error("an IP backend should make a STATIC cluster")
	}

	// Refused: a taken cluster name, and a domain served with others.
	dup := RouteSpec{Domains: []string{"x.example.com"}, Upstreams: []string{"a:1"}, ClusterName: "web"}
	dup.Validate()
	if _, _, err := dup.Render(sample); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate cluster: %v", err)
	}
	mixed := RouteSpec{Domains: []string{"*", "other.com"}, Upstreams: []string{"a:1"}}
	mixed.Validate()
	if _, _, err := mixed.Render(sample); err == nil {
		t.Error("a domain already in another virtual host should be refused")
	}

	// The panel's own starting config ("clusters: []") takes a route too.
	first := RouteSpec{Domains: []string{"*"}, PathPrefix: "/", Upstreams: []string{"web:80"}, ClusterName: "web"}
	first.Validate()
	out, _, err = first.Render(deploy.EnvoyDefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	c = ParseConfig(out)
	if len(c.Clusters) != 1 || c.Listeners[0].Routes[1].Cluster != "web" || c.Listeners[0].Routes[2].Target != "answers 404 itself" {
		t.Errorf("on the default config: %+v\n%s", c.Listeners[0].Routes, out)
	}
}

func TestRouteSpecValidate(t *testing.T) {
	for _, bad := range []RouteSpec{
		{Upstreams: []string{"a:1"}},
		{Domains: []string{"a.com"}},
		{Domains: []string{"a.com"}, Upstreams: []string{"a"}},
		{Domains: []string{"a.com"}, Upstreams: []string{"a:99999"}},
		{Domains: []string{"a.com"}, Upstreams: []string{"a:1"}, PathPrefix: "api"},
		{Domains: []string{"a.com"}, Upstreams: []string{"a:1"}, ClusterName: "bad name"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

func TestHelpers(t *testing.T) {
	if p := ConfigPath([]string{"envoy", "-c", "/etc/envoy/envoy.yaml"}); p != "/etc/envoy/envoy.yaml" {
		t.Error(p)
	}
	if p := ConfigPath([]string{"--config-path=/cfg/x.yaml"}); p != "/cfg/x.yaml" {
		t.Error(p)
	}
	if p := ConfigPath(nil); p != DefaultConfigPath {
		t.Error(p)
	}
	for img, want := range map[string]bool{"envoyproxy/envoy:v1.39-latest": true, "envoyproxy/envoy-distroless:v1.39": true, "registry.local/envoy:1": true, "nginx:alpine": false, "haproxy:lts": false} {
		if IsEnvoyImage(img) != want {
			t.Errorf("IsEnvoyImage(%q) != %v", img, want)
		}
	}
	// Envoy's real validation failure output.
	out := "[2026-10-03 09:04:50.008][1][info][config] loading 1 listener(s)\n[2026-10-03 09:04:50.009][1][critical][main] [source/server/config_validation/server.cc:76] error initializing configuration '/panel-check/envoy.yaml': route: unknown cluster 'nope'"
	if v := versionOf("50d48c6c6c964b06f1365cb54ed05a7e03ac631f/1.39.0/Clean/RELEASE/BoringSSL"); v != "1.39.0 (RELEASE, BoringSSL)" {
		t.Errorf("version: %q", v)
	}
	if got := lastLine(out); got != "error initializing configuration '/panel-check/envoy.yaml': route: unknown cluster 'nope'" {
		t.Errorf("lastLine = %q", got)
	}
}
