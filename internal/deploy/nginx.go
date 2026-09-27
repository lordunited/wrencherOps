package deploy

import (
	"fmt"
	"strings"
)

// NginxConfig holds what's needed to run the official nginx image with its
// configuration on the host, so the panel can edit sites and reload.
type NginxConfig struct {
	HTTPPort      int    // host port -> container 80
	HTTPSPort     int    // host port -> container 443; 0 omits it
	ImageTag      string // nginx image tag, e.g. "stable-alpine"
	DeployDir     string
	ContainerName string

	// Networks are extra docker networks the container joins, created
	// fresh in this compose file, e.g. to reach the apps it proxies to.
	Networks []string

	// ExtraVolumes are additional mounts in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	CPULimit    string
	MemoryLimit string
}

// Nginx folders under the deploy directory, mounted into the container.
// The panel manages Nginx configuration, not website content.
const (
	NginxConfFolder  = "conf.d" // -> /etc/nginx/conf.d, one file per server block
	NginxCertsFolder = "certs"  // -> /etc/nginx/certs, read-only
)

func (c NginxConfig) ComposeYAML() string {
	var b strings.Builder
	b.WriteString("services:\n")
	b.WriteString("  nginx:\n")
	fmt.Fprintf(&b, "    image: nginx:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", c.ContainerName)
	b.WriteString("    restart: always\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:80'\n", c.HTTPPort)
	if c.HTTPSPort > 0 {
		fmt.Fprintf(&b, "      - '%d:443'\n", c.HTTPSPort)
	}
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/%s:/etc/nginx/conf.d'\n", c.DeployDir, NginxConfFolder)
	fmt.Fprintf(&b, "      - '%s/%s:/etc/nginx/certs:ro'\n", c.DeployDir, NginxCertsFolder)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}
	if c.CPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.CPULimit)
	}
	if c.MemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.MemoryLimit)
	}
	// Healthy while the master process is alive and the config on disk is
	// valid. Both the Debian and Alpine images ship nginx and a shell.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD-SHELL\", \"nginx -t -q && kill -0 $$(cat /run/nginx.pid 2>/dev/null || cat /var/run/nginx.pid)\"]\n")
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

// NginxDefaultServer is the starting config of a new Nginx: a catch-all
// server that answers /healthz and refuses everything else, so no request
// is served by accident until real server blocks are added.
const NginxDefaultServer = `server {
    listen 80 default_server;
    server_name _;

    location = /healthz {
        access_log off;
        default_type text/plain;
        return 200 "ok\n";
    }

    location / {
        return 404;
    }
}
`

// NginxSeedFiles returns the starting config for a new Nginx.
func NginxSeedFiles(deployDir string) []SeedFile {
	return []SeedFile{{Path: strings.TrimRight(deployDir, "/") + "/" + NginxConfFolder + "/default.conf", Content: NginxDefaultServer}}
}
