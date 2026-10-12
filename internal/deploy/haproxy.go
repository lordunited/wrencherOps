// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"strings"
)

// HAProxyConfig holds what's needed to run the official haproxy image with
// its configuration on the host, so the panel can edit it and reload.
type HAProxyConfig struct {
	HTTPPort      int    // host port -> container 80
	HTTPSPort     int    // host port -> container 443; 0 omits it
	ImageTag      string // haproxy image tag, e.g. "lts-alpine"
	DeployDir     string
	ContainerName string

	// Networks are extra docker networks the container joins, created
	// fresh in this compose file, e.g. to reach the apps it balances.
	Networks []string

	// ExtraVolumes are additional mounts in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	CPULimit    string
	MemoryLimit string
}

// HAProxyConfigFolder is the folder under the deploy directory mounted into
// the container. The panel manages HAProxy's configuration, not what it
// balances to. The image runs as a fixed non-root user from the start (not
// root-then-drop, like Nginx's), but a host directory made by "mkdir -p"
// is world-readable by default, so — verified against a real container —
// that user can read it with no ownership changes needed.
const HAProxyConfigFolder = "conf" // -> /usr/local/etc/haproxy

func (c HAProxyConfig) ComposeYAML() string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  haproxy:\n")
	fmt.Fprintf(&b, "    image: haproxy:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", c.ContainerName)
	b.WriteString("    restart: always\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:80'\n", c.HTTPPort)
	if c.HTTPSPort > 0 {
		fmt.Fprintf(&b, "      - '%d:443'\n", c.HTTPSPort)
	}
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/%s:/usr/local/etc/haproxy'\n", c.DeployDir, HAProxyConfigFolder)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}
	if c.CPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.CPULimit)
	}
	if c.MemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.MemoryLimit)
	}
	// Verified against a real container: haproxy:lts-alpine has wget (no
	// curl), and the seed config's "monitor-uri" answers /healthz with a
	// plain 200 directly — no backend needed for the check to pass. Uses
	// 127.0.0.1 rather than localhost: this image resolves "localhost" to
	// ::1 first and the check fails with "connection refused" if the
	// listener isn't also reachable over IPv6.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"wget\", \"--spider\", \"-q\", \"http://127.0.0.1:80/healthz\"]\n")
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

// HAProxyStatsPort is the stats listener's bind port in the seed config.
// It's bound to 127.0.0.1 and never published in ComposeYAML's "ports:",
// since the panel only ever reaches it from inside the container (docker
// exec + wget to 127.0.0.1) — so it isn't reachable from outside the
// container at all, even if the stack is later switched to host
// networking. Its "mode http" is explicit because HAProxy silently ignores
// "stats" in TCP mode (verified), and a user may change the defaults.
const HAProxyStatsPort = 8404

// HAProxyDefaultConfig is the starting config of a new HAProxy: it answers
// its own health check and refuses everything else, so no request is
// served by accident until real frontends/backends are added. It also
// enables a stats page from the start (container-internal only, see
// HAProxyStatsPort) so the panel's Backends tab can show live per-server
// UP/DOWN status the moment real backends are added — without it, that
// status is structurally unavailable, since HAProxy's stats page is the
// only place it exists. Verified against a real container: config passes
// "haproxy -c", /healthz answers 200 via monitor-uri with no backend
// involved, everything else gets a plain 404, and the stats page answers
// its CSV correctly from inside the container on HAProxyStatsPort.
var HAProxyDefaultConfig = fmt.Sprintf(`global
    log stdout format raw local0
    maxconn 4096

defaults
    mode http
    timeout connect 5s
    timeout client  30s
    timeout server  30s
    log global
    option httplog

frontend main
    bind *:80
    monitor-uri /healthz
    default_backend no_backend

backend no_backend
    http-request deny deny_status 404

listen stats
    bind 127.0.0.1:%d
    mode http
    stats enable
    stats uri /haproxy?stats
`, HAProxyStatsPort)

// HAProxySeedFiles returns the starting config for a new HAProxy.
func HAProxySeedFiles(deployDir string) []SeedFile {
	return []SeedFile{{Path: strings.TrimRight(deployDir, "/") + "/" + HAProxyConfigFolder + "/haproxy.cfg", Content: HAProxyDefaultConfig}}
}
