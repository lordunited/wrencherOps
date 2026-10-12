// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"strings"
)

// EnvoyConfig holds what's needed to run the official envoyproxy/envoy
// image with its bootstrap config on the host, so the panel can edit it.
type EnvoyConfig struct {
	HTTPPort      int    // host port -> the listener on container port 10000
	ImageTag      string // envoyproxy/envoy tag, e.g. "v1.39-latest"
	DeployDir     string
	ContainerName string

	// Networks are extra docker networks the container joins, created
	// fresh in this compose file, e.g. to reach the services it routes to.
	Networks []string

	// ExtraVolumes are additional mounts in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	CPULimit    string
	MemoryLimit string
}

// EnvoyConfigFolder is the folder under the deploy directory mounted at
// /etc/envoy. Envoy runs as its own user (uid 101, set up by the image's
// entrypoint); a folder made by "mkdir -p" is world-readable, so it can
// read the config with no ownership changes (verified).
const EnvoyConfigFolder = "conf" // -> /etc/envoy

// EnvoyListenerPort is the listener port in the starting config.
const EnvoyListenerPort = 10000

// EnvoyAdminPort is the admin API's port, bound to 127.0.0.1 inside the
// container and never published: the panel reaches it with docker exec.
const EnvoyAdminPort = 9901

func (c EnvoyConfig) ComposeYAML() string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  envoy:\n")
	fmt.Fprintf(&b, "    image: envoyproxy/envoy:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", c.ContainerName)
	b.WriteString("    restart: always\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:%d'\n", c.HTTPPort, EnvoyListenerPort)
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/%s:/etc/envoy'\n", c.DeployDir, EnvoyConfigFolder)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}
	if c.CPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.CPULimit)
	}
	if c.MemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.MemoryLimit)
	}
	// The image has bash but no curl or wget (verified), so the check asks
	// the admin API's /ready over bash's /dev/tcp — in HTTP/1.1, as the
	// admin API answers HTTP/1.0 with "426 Upgrade Required".
	b.WriteString("    healthcheck:\n")
	fmt.Fprintf(&b, "      test: [\"CMD\", \"bash\", \"-c\", 'exec 3<>/dev/tcp/127.0.0.1/%d && printf \"GET /ready HTTP/1.1\\r\\nHost: localhost\\r\\nConnection: close\\r\\n\\r\\n\" >&3 && grep -q LIVE <&3']\n", EnvoyAdminPort)
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 3\n")
	b.WriteString("      start_period: 10s\n")
	if len(c.Networks) > 0 {
		b.WriteString("    networks:\n")
		for _, n := range c.Networks {
			fmt.Fprintf(&b, "      - %s\n", n)
		}
		b.WriteString("\nnetworks:\n")
		for _, n := range c.Networks {
			fmt.Fprintf(&b, "  %s:\n", n)
		}
	}
	return b.String()
}

// EnvoyDefaultConfig is the starting config of a new Envoy: one HTTP
// listener that answers /healthz itself and 404s everything else, so
// nothing is proxied by accident until routes are added — from the panel's
// "Add a route", or by editing the file. Its admin API is on 127.0.0.1
// only. Verified: "envoy --mode validate" accepts it, /healthz answers 200
// and anything else 404.
var EnvoyDefaultConfig = fmt.Sprintf(`admin:
  address:
    socket_address:
      address: 127.0.0.1
      port_value: %d
static_resources:
  listeners:
  - name: main
    address:
      socket_address:
        address: 0.0.0.0
        port_value: %d
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          access_log:
          - name: envoy.access_loggers.stdout
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog
          http_filters:
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
          route_config:
            name: local_route
            virtual_hosts:
            - name: default
              domains: ["*"]
              routes:
              - match:
                  path: /healthz
                direct_response:
                  status: 200
                  body:
                    inline_string: "ok\n"
              - match:
                  prefix: /
                direct_response:
                  status: 404
                  body:
                    inline_string: "no route\n"
  clusters: []
`, EnvoyAdminPort, EnvoyListenerPort)

// EnvoySeedFiles returns the starting config for a new Envoy.
func EnvoySeedFiles(deployDir string) []SeedFile {
	return []SeedFile{{Path: strings.TrimRight(deployDir, "/") + "/" + EnvoyConfigFolder + "/envoy.yaml", Content: EnvoyDefaultConfig}}
}
