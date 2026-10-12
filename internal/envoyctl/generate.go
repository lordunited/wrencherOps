// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package envoyctl

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// RouteSpec is a route to add: requests for Domains under PathPrefix go to
// a new cluster of Upstreams.
type RouteSpec struct {
	Domains     []string `json:"domains"`     // e.g. ["shop.example.com"], or ["*"]
	PathPrefix  string   `json:"pathPrefix"`  // "/" for everything
	Upstreams   []string `json:"upstreams"`   // "host:port", one or more
	ClusterName string   `json:"clusterName"` // the new cluster's name
	HealthPath  string   `json:"healthPath"`  // optional HTTP health check path
	Listener    string   `json:"listener"`    // optional: which HTTP listener (by name)
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// Validate checks a route spec's fields on their own.
func (s *RouteSpec) Validate() error {
	var domains []string
	for _, d := range s.Domains {
		if d = strings.TrimSpace(d); d != "" {
			domains = append(domains, d)
		}
	}
	if len(domains) == 0 {
		return fmt.Errorf("enter at least one domain (or * for any)")
	}
	for _, d := range domains {
		if strings.ContainsAny(d, " /\"'") {
			return fmt.Errorf("%q isn't a valid domain", d)
		}
	}
	s.Domains = domains
	if s.PathPrefix = strings.TrimSpace(s.PathPrefix); s.PathPrefix == "" {
		s.PathPrefix = "/"
	}
	if !strings.HasPrefix(s.PathPrefix, "/") {
		return fmt.Errorf("the path must start with /")
	}
	var ups []string
	for _, u := range s.Upstreams {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		host, port, err := net.SplitHostPort(u)
		if p, perr := strconv.Atoi(port); err != nil || host == "" || perr != nil || p < 1 || p > 65535 {
			return fmt.Errorf("%q isn't host:port", u)
		}
		ups = append(ups, u)
	}
	if len(ups) == 0 {
		return fmt.Errorf("enter at least one backend as host:port")
	}
	s.Upstreams = ups
	s.ClusterName = strings.TrimSpace(s.ClusterName)
	if s.ClusterName == "" {
		s.ClusterName = clusterNameFor(s.Domains[0], s.PathPrefix)
	}
	if !nameRe.MatchString(s.ClusterName) {
		return fmt.Errorf("a cluster name can only have letters, digits, . _ and -")
	}
	if s.HealthPath = strings.TrimSpace(s.HealthPath); s.HealthPath != "" && !strings.HasPrefix(s.HealthPath, "/") {
		return fmt.Errorf("the health check path must start with /")
	}
	return nil
}

func clusterNameFor(domain, prefix string) string {
	n := strings.NewReplacer("*", "any", ".", "_").Replace(domain)
	if p := strings.Trim(prefix, "/"); p != "" {
		n += "_" + strings.NewReplacer("/", "_").Replace(p)
	}
	n = regexp.MustCompile(`[^A-Za-z0-9_.-]`).ReplaceAllString(n, "_")
	return n
}

// mapGet returns the value node for key in a mapping node, or nil.
func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// mapEnsure returns key's value in m, adding it (as an empty node of kind)
// when it's missing.
func mapEnsure(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	if v := mapGet(m, key); v != nil {
		return v
	}
	v := &yaml.Node{Kind: kind}
	if kind == yaml.MappingNode {
		v.Tag = "!!map"
	} else if kind == yaml.SequenceNode {
		v.Tag = "!!seq"
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	return v
}

// fragment parses a YAML snippet into a node.
func fragment(src string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return nil, err
	}
	return doc.Content[0], nil
}

func quote(s string) string { return strconv.Quote(s) }

// Render adds spec to a bootstrap file and returns the new file and a
// one-line summary. It edits the document's structure (so the rest of the
// file and its comments stay), then re-encodes it with 2-space indents.
// An empty sequence written inline ("clusters: []") becomes a block list.
func (spec RouteSpec) Render(content string) (string, string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", "", fmt.Errorf("the config isn't valid YAML: %v", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", "", fmt.Errorf("the config is empty or not a mapping")
	}
	root := doc.Content[0]
	static := mapEnsure(root, "static_resources", yaml.MappingNode)
	listeners := mapGet(static, "listeners")
	if listeners == nil || listeners.Kind != yaml.SequenceNode {
		return "", "", fmt.Errorf("the config has no static listeners to add a route to")
	}

	// The listener: the one named, else the first HTTP one with inline routes.
	var routeCfg *yaml.Node
	var listenerName string
	for _, l := range listeners.Content {
		name := ""
		if n := mapGet(l, "name"); n != nil {
			name = n.Value
		}
		if spec.Listener != "" && name != spec.Listener {
			continue
		}
		chains := mapGet(l, "filter_chains")
		if chains == nil {
			continue
		}
		for _, fc := range chains.Content {
			filters := mapGet(fc, "filters")
			if filters == nil {
				continue
			}
			for _, f := range filters.Content {
				tc := mapGet(f, "typed_config")
				if rc := mapGet(tc, "route_config"); rc != nil && routeCfg == nil {
					routeCfg, listenerName = rc, name
				}
			}
		}
	}
	if routeCfg == nil {
		if spec.Listener != "" {
			return "", "", fmt.Errorf("listener %q has no HTTP routes in this file to add to", spec.Listener)
		}
		return "", "", fmt.Errorf("no HTTP listener with routes written in this file (routes from RDS are managed by its control plane)")
	}

	clusters := mapEnsure(static, "clusters", yaml.SequenceNode)
	clusters.Style = 0 // block style, even if it was "[]"
	for _, c := range clusters.Content {
		if n := mapGet(c, "name"); n != nil && n.Value == spec.ClusterName {
			return "", "", fmt.Errorf("a cluster named %q already exists; choose another name", spec.ClusterName)
		}
	}

	// The cluster: STATIC when every backend is an IP, else STRICT_DNS.
	clusterType := "STATIC"
	var eps strings.Builder
	for _, u := range spec.Upstreams {
		host, port, _ := net.SplitHostPort(u)
		if net.ParseIP(host) == nil {
			clusterType = "STRICT_DNS"
		}
		fmt.Fprintf(&eps, "      - endpoint:\n          address:\n            socket_address:\n              address: %s\n              port_value: %s\n", quote(host), port)
	}
	// Cluster names are letters, digits, . _ - only (Validate), so they're
	// written plain; domains, hosts and paths are quoted ("*" must be).
	src := fmt.Sprintf("name: %s\ntype: %s\nconnect_timeout: 5s\nlb_policy: ROUND_ROBIN\nload_assignment:\n  cluster_name: %s\n  endpoints:\n  - lb_endpoints:\n%s",
		spec.ClusterName, clusterType, spec.ClusterName, eps.String())
	if spec.HealthPath != "" {
		src += fmt.Sprintf("health_checks:\n- timeout: 2s\n  interval: 10s\n  unhealthy_threshold: 3\n  healthy_threshold: 2\n  http_health_check:\n    path: %s\n", quote(spec.HealthPath))
	}
	clusterNode, err := fragment(src)
	if err != nil {
		return "", "", err
	}
	clusters.Content = append(clusters.Content, clusterNode)

	routeNode, err := fragment(fmt.Sprintf("match:\n  prefix: %s\nroute:\n  cluster: %s\n", quote(spec.PathPrefix), spec.ClusterName))
	if err != nil {
		return "", "", err
	}

	// The virtual host: one with exactly these domains gets the route (ahead
	// of any catch-all prefix "/" route, which would otherwise take it);
	// otherwise a new one. A domain already served elsewhere is refused:
	// Envoy rejects a domain in two virtual hosts.
	vhosts := mapEnsure(routeCfg, "virtual_hosts", yaml.SequenceNode)
	want := strings.Join(spec.Domains, ",")
	var target *yaml.Node
	for _, vh := range vhosts.Content {
		var ds []string
		if d := mapGet(vh, "domains"); d != nil {
			for _, x := range d.Content {
				ds = append(ds, x.Value)
			}
		}
		if strings.Join(ds, ",") == want {
			target = vh
			break
		}
		for _, a := range ds {
			for _, b := range spec.Domains {
				if a == b {
					name := ""
					if n := mapGet(vh, "name"); n != nil {
						name = n.Value
					}
					return "", "", fmt.Errorf("%s is already served by virtual host %q together with other domains; add the route there in the file", b, name)
				}
			}
		}
	}
	summary := ""
	if target != nil {
		routes := mapEnsure(target, "routes", yaml.SequenceNode)
		at := len(routes.Content)
		for i, r := range routes.Content {
			m := mapGet(r, "match")
			if p := mapGet(m, "prefix"); p != nil && (p.Value == "/" || len(p.Value) < len(spec.PathPrefix) && strings.HasPrefix(spec.PathPrefix, p.Value)) {
				at = i
				break
			}
		}
		routes.Content = append(routes.Content[:at], append([]*yaml.Node{routeNode}, routes.Content[at:]...)...)
		summary = fmt.Sprintf("Route %s%s → cluster %s (%s) on listener %s", want, spec.PathPrefix, spec.ClusterName, strings.Join(spec.Upstreams, ", "), listenerName)
	} else {
		var domains []string
		for _, d := range spec.Domains {
			domains = append(domains, quote(d))
		}
		vh, err := fragment(fmt.Sprintf("name: %s\ndomains: [%s]\nroutes: []\n", spec.ClusterName, strings.Join(domains, ", ")))
		if err != nil {
			return "", "", err
		}
		r := mapGet(vh, "routes")
		r.Style = 0
		r.Content = []*yaml.Node{routeNode}
		vhosts.Content = append(vhosts.Content, vh)
		summary = fmt.Sprintf("New virtual host for %s: %s → cluster %s (%s) on listener %s", want, spec.PathPrefix, spec.ClusterName, strings.Join(spec.Upstreams, ", "), listenerName)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", "", err
	}
	enc.Close()
	return buf.String(), summary, nil
}
