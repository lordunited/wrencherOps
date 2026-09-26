// Package deploy generates configuration for and runs remote installs of
// devops-wrapper's supported services, starting with GitLab on Docker.
package deploy

import (
	"fmt"
	"net/url"
	"strings"
)

// GitLabConfig holds everything needed to bring up a self-hosted GitLab via
// the official gitlab/gitlab-ce Docker image and initialize it.
//
// GitLab's own config surface (gitlab.rb / GITLAB_OMNIBUS_CONFIG) has
// hundreds of settings — SMTP, LDAP, registry, backups, and so on. Rather
// than model each one as a field here, ExtraOmnibusConfig is a raw escape
// hatch: any line valid in gitlab.rb can go there and it's appended
// verbatim into the generated config block. The alternative — a form field
// per GitLab setting — doesn't scale to "all of them."
type GitLabConfig struct {
	ExternalURL   string // e.g. http://gitlab.example.com — required, drives GitLab's own self-configuration
	HTTPPort      int    // host port -> container 80
	HTTPSPort     int    // host port -> container 443; 0 omits HTTPS entirely
	SSHPort       int    // host port -> container 22, for git-over-ssh
	RootPassword  string // sets the root account's password on first boot only
	ImageTag      string // gitlab/gitlab-ce image tag, e.g. "latest"
	DeployDir     string // host directory holding compose file + volumes, e.g. /opt/gitlab
	ContainerName string

	// ExtraOmnibusConfig is appended, line by line, into the
	// GITLAB_OMNIBUS_CONFIG block after the fields above — e.g.
	// registry_external_url 'https://registry.example.com'
	// gitlab_rails['smtp_enable'] = true
	ExtraOmnibusConfig string

	// Networks are extra docker networks the container joins, created
	// fresh in this compose file (not assumed to pre-exist).
	Networks []string

	// ExtraVolumes are additional volume mounts beyond the standard
	// config/logs/data ones, in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	// CPULimit and MemoryLimit cap the container's resource usage, e.g.
	// "2" and "4g". Empty means no limit (docker-compose default).
	CPULimit    string
	MemoryLimit string
}

// ComposeYAML renders the docker-compose.yml content for this configuration,
// following GitLab's documented Docker deployment layout.
func (c GitLabConfig) ComposeYAML() string {
	var b strings.Builder

	b.WriteString("services:\n")
	b.WriteString("  gitlab:\n")
	fmt.Fprintf(&b, "    image: gitlab/gitlab-ce:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", c.ContainerName)
	b.WriteString("    restart: always\n")
	fmt.Fprintf(&b, "    hostname: '%s'\n", hostnameFromURL(c.ExternalURL))
	b.WriteString("    environment:\n")
	b.WriteString("      GITLAB_OMNIBUS_CONFIG: |\n")
	fmt.Fprintf(&b, "        external_url '%s'\n", c.ExternalURL)
	fmt.Fprintf(&b, "        gitlab_rails['initial_root_password'] = '%s'\n", c.RootPassword)
	fmt.Fprintf(&b, "        gitlab_rails['gitlab_shell_ssh_port'] = %d\n", c.SSHPort)
	for _, line := range nonEmptyLines(c.ExtraOmnibusConfig) {
		fmt.Fprintf(&b, "        %s\n", line)
	}

	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:80'\n", c.HTTPPort)
	if c.HTTPSPort > 0 {
		fmt.Fprintf(&b, "      - '%d:443'\n", c.HTTPSPort)
	}
	fmt.Fprintf(&b, "      - '%d:22'\n", c.SSHPort)

	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/config:/etc/gitlab'\n", c.DeployDir)
	fmt.Fprintf(&b, "      - '%s/logs:/var/log/gitlab'\n", c.DeployDir)
	fmt.Fprintf(&b, "      - '%s/data:/var/opt/gitlab'\n", c.DeployDir)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}

	b.WriteString("    shm_size: '256m'\n")

	if c.CPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.CPULimit)
	}
	if c.MemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.MemoryLimit)
	}

	// GitLab's own documented readiness endpoint; curl ships in the full
	// omnibus image. start_period is generous because first boot can take
	// several minutes.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"curl\", \"-f\", \"http://localhost/-/readiness\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 5m\n")

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

func hostnameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return rawURL
	}
	return u.Hostname()
}

// nonEmptyLines splits s on newlines, trims trailing \r, and drops blank
// lines — used for user-pasted multi-line text fields (extra config,
// volumes) so stray blank lines don't produce broken output.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
