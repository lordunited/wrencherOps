// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package envoyctl

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is a readable view of an Envoy bootstrap file: what it listens on,
// how requests are routed, and where they go.
type Config struct {
	// Admin is the admin API's address ("127.0.0.1:9901"), "" without one.
	Admin     string     `json:"admin"`
	Listeners []Listener `json:"listeners"`
	Clusters  []Cluster  `json:"clusters"`
	// Dynamic describes what comes from a control plane (xDS) rather than
	// this file, "" when everything is in the file.
	Dynamic    string `json:"dynamic,omitempty"`
	ParseError string `json:"parseError,omitempty"`
}

type Listener struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Port    int     `json:"port"`
	Kind    string  `json:"kind"` // "http", "tcp" or "other"
	Routes  []Route `json:"routes"`
	// TCPCluster is where a TCP proxy listener sends connections.
	TCPCluster string `json:"tcpCluster,omitempty"`
	// RDS names the route config a control plane sends, when it isn't inline.
	RDS string `json:"rds,omitempty"`
	TLS bool   `json:"tls"`
}

// Route is one route: requests for Domains matching Match go to Target.
type Route struct {
	VirtualHost string   `json:"virtualHost"`
	Domains     []string `json:"domains"`
	Match       string   `json:"match"`  // "prefix /api", "path /healthz", "regex ^/v[0-9]"
	Target      string   `json:"target"` // a cluster name, or what it does instead
	Cluster     string   `json:"cluster,omitempty"`
}

type Cluster struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	LBPolicy    string   `json:"lbPolicy"`
	Endpoints   []string `json:"endpoints"`
	HealthCheck string   `json:"healthCheck,omitempty"`
	TLS         bool     `json:"tls"`
}

type socketAddress struct {
	SocketAddress struct {
		Address   string `yaml:"address"`
		PortValue int    `yaml:"port_value"`
	} `yaml:"socket_address"`
	Pipe struct {
		Path string `yaml:"path"`
	} `yaml:"pipe"`
}

func (a socketAddress) String() string {
	if a.Pipe.Path != "" {
		return "unix:" + a.Pipe.Path
	}
	if a.SocketAddress.Address == "" && a.SocketAddress.PortValue == 0 {
		return ""
	}
	return net.JoinHostPort(a.SocketAddress.Address, strconv.Itoa(a.SocketAddress.PortValue))
}

type routeConfig struct {
	VirtualHosts []struct {
		Name    string   `yaml:"name"`
		Domains []string `yaml:"domains"`
		Routes  []struct {
			Match struct {
				Prefix         string                 `yaml:"prefix"`
				Path           string                 `yaml:"path"`
				PathSeparated  string                 `yaml:"path_separated_prefix"`
				SafeRegex      struct{ Regex string } `yaml:"safe_regex"`
				ConnectMatcher any                    `yaml:"connect_matcher"`
			} `yaml:"match"`
			Route *struct {
				Cluster          string `yaml:"cluster"`
				ClusterHeader    string `yaml:"cluster_header"`
				WeightedClusters *struct {
					Clusters []struct {
						Name   string `yaml:"name"`
						Weight int    `yaml:"weight"`
					} `yaml:"clusters"`
				} `yaml:"weighted_clusters"`
			} `yaml:"route"`
			Redirect       map[string]any `yaml:"redirect"`
			DirectResponse *struct {
				Status int `yaml:"status"`
			} `yaml:"direct_response"`
		} `yaml:"routes"`
	} `yaml:"virtual_hosts"`
}

type bootstrap struct {
	Admin struct {
		Address socketAddress `yaml:"address"`
	} `yaml:"admin"`
	StaticResources struct {
		Listeners []struct {
			Name         string        `yaml:"name"`
			Address      socketAddress `yaml:"address"`
			FilterChains []struct {
				TransportSocket map[string]any `yaml:"transport_socket"`
				Filters         []struct {
					Name        string         `yaml:"name"`
					TypedConfig map[string]any `yaml:"typed_config"`
				} `yaml:"filters"`
			} `yaml:"filter_chains"`
		} `yaml:"listeners"`
		Clusters []struct {
			Name           string         `yaml:"name"`
			Type           string         `yaml:"type"`
			LBPolicy       string         `yaml:"lb_policy"`
			TransportSock  map[string]any `yaml:"transport_socket"`
			LoadAssignment struct {
				Endpoints []struct {
					LBEndpoints []struct {
						Endpoint struct {
							Address socketAddress `yaml:"address"`
						} `yaml:"endpoint"`
					} `yaml:"lb_endpoints"`
				} `yaml:"endpoints"`
			} `yaml:"load_assignment"`
			HealthChecks []struct {
				Interval        string         `yaml:"interval"`
				HTTPHealthCheck map[string]any `yaml:"http_health_check"`
				TCPHealthCheck  map[string]any `yaml:"tcp_health_check"`
			} `yaml:"health_checks"`
		} `yaml:"clusters"`
	} `yaml:"static_resources"`
	DynamicResources map[string]any `yaml:"dynamic_resources"`
}

// ParseConfig reads a bootstrap file (YAML, or JSON, which is YAML too).
func ParseConfig(content string) Config {
	cfg := Config{Listeners: []Listener{}, Clusters: []Cluster{}}
	var b bootstrap
	if err := yaml.Unmarshal([]byte(content), &b); err != nil {
		cfg.ParseError = err.Error()
		return cfg
	}
	cfg.Admin = b.Admin.Address.String()

	for _, l := range b.StaticResources.Listeners {
		lv := Listener{Name: l.Name, Address: l.Address.SocketAddress.Address, Port: l.Address.SocketAddress.PortValue, Kind: "other", Routes: []Route{}}
		for _, fc := range l.FilterChains {
			if fc.TransportSocket != nil {
				lv.TLS = true
			}
			for _, f := range fc.Filters {
				typ, _ := f.TypedConfig["@type"].(string)
				switch {
				case strings.HasSuffix(typ, "HttpConnectionManager") || strings.HasSuffix(f.Name, "http_connection_manager"):
					lv.Kind = "http"
					if rds, ok := f.TypedConfig["rds"].(map[string]any); ok {
						lv.RDS, _ = rds["route_config_name"].(string)
					}
					if rc, ok := f.TypedConfig["route_config"]; ok {
						lv.Routes = append(lv.Routes, routesOf(rc)...)
					}
				case strings.HasSuffix(typ, "TcpProxy") || strings.HasSuffix(f.Name, "tcp_proxy"):
					lv.Kind = "tcp"
					lv.TCPCluster, _ = f.TypedConfig["cluster"].(string)
				}
			}
		}
		cfg.Listeners = append(cfg.Listeners, lv)
	}

	for _, c := range b.StaticResources.Clusters {
		cv := Cluster{Name: c.Name, Type: c.Type, LBPolicy: c.LBPolicy, Endpoints: []string{}, TLS: c.TransportSock != nil}
		if cv.Type == "" {
			cv.Type = "STATIC"
		}
		if cv.LBPolicy == "" {
			cv.LBPolicy = "ROUND_ROBIN"
		}
		for _, e := range c.LoadAssignment.Endpoints {
			for _, le := range e.LBEndpoints {
				if a := le.Endpoint.Address.String(); a != "" {
					cv.Endpoints = append(cv.Endpoints, a)
				}
			}
		}
		for _, hc := range c.HealthChecks {
			switch {
			case hc.HTTPHealthCheck != nil:
				p, _ := hc.HTTPHealthCheck["path"].(string)
				cv.HealthCheck = fmt.Sprintf("HTTP %s every %s", p, hc.Interval)
			case hc.TCPHealthCheck != nil:
				cv.HealthCheck = "TCP every " + hc.Interval
			}
		}
		cfg.Clusters = append(cfg.Clusters, cv)
	}

	if len(b.DynamicResources) > 0 {
		var parts []string
		for k := range b.DynamicResources {
			switch k {
			case "lds_config":
				parts = append(parts, "listeners (LDS)")
			case "cds_config":
				parts = append(parts, "clusters (CDS)")
			case "ads_config":
				parts = append(parts, "an aggregated stream (ADS)")
			}
		}
		sort.Strings(parts)
		if len(parts) > 0 {
			cfg.Dynamic = "Some configuration comes from a control plane (xDS): " + strings.Join(parts, ", ") + ". Edit that there; this file only has the bootstrap."
		}
	}
	return cfg
}

// routesOf reads an inline route_config (decoded generically, as it sits
// inside typed_config) by re-encoding it into the typed struct.
func routesOf(raw any) []Route {
	b, err := yaml.Marshal(raw)
	if err != nil {
		return nil
	}
	var rc routeConfig
	if yaml.Unmarshal(b, &rc) != nil {
		return nil
	}
	var out []Route
	for _, vh := range rc.VirtualHosts {
		for _, r := range vh.Routes {
			rv := Route{VirtualHost: vh.Name, Domains: vh.Domains}
			switch m := r.Match; {
			case m.Prefix != "":
				rv.Match = "prefix " + m.Prefix
			case m.Path != "":
				rv.Match = "path " + m.Path
			case m.PathSeparated != "":
				rv.Match = "prefix " + m.PathSeparated
			case m.SafeRegex.Regex != "":
				rv.Match = "regex " + m.SafeRegex.Regex
			case m.ConnectMatcher != nil:
				rv.Match = "CONNECT"
			default:
				rv.Match = "any"
			}
			switch {
			case r.Route != nil && r.Route.Cluster != "":
				rv.Target, rv.Cluster = r.Route.Cluster, r.Route.Cluster
			case r.Route != nil && r.Route.WeightedClusters != nil:
				var ws []string
				for _, c := range r.Route.WeightedClusters.Clusters {
					ws = append(ws, fmt.Sprintf("%s (%d)", c.Name, c.Weight))
				}
				rv.Target = "split: " + strings.Join(ws, ", ")
			case r.Route != nil && r.Route.ClusterHeader != "":
				rv.Target = "cluster from header " + r.Route.ClusterHeader
			case r.Redirect != nil:
				rv.Target = "redirect"
				if h, ok := r.Redirect["https_redirect"].(bool); ok && h {
					rv.Target = "redirect to HTTPS"
				}
			case r.DirectResponse != nil:
				rv.Target = fmt.Sprintf("answers %d itself", r.DirectResponse.Status)
			default:
				rv.Target = "—"
			}
			out = append(out, rv)
		}
	}
	return out
}
