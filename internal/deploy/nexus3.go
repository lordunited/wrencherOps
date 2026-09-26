package deploy

import (
	"fmt"
	"strings"
)

// Nexus3Config holds what's needed to bring up Sonatype Nexus Repository 3
// via the official sonatype/nexus3 Docker image.
//
// Unlike GitLab, this image has no config knob to set the admin password up
// front — it generates one into /nexus-data/admin.password inside the
// container on first boot. There's nothing to configure around that; the
// deploy job's log says where to find it.
type Nexus3Config struct {
	HTTPPort      int    // host port -> container 8081 (web UI + API)
	ExtraPort     int    // optional second port, e.g. for a docker registry connector; 0 omits it
	ImageTag      string // sonatype/nexus3 image tag, e.g. "latest"
	DeployDir     string // host directory holding compose file + data volume
	ContainerName string

	// Networks are extra docker networks the container joins, created
	// fresh in this compose file (not assumed to pre-exist).
	Networks []string

	// ExtraVolumes are additional volume mounts beyond the standard
	// nexus-data one, in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	// CPULimit and MemoryLimit cap the container's resource usage, e.g.
	// "2" and "4g". Empty means no limit (docker-compose default).
	CPULimit    string
	MemoryLimit string

	// JavaHeap sets Nexus's JVM min/max heap and max direct memory to the
	// same value, e.g. "2703m" — matching Sonatype's own production
	// sizing guidance (heap ≈ RAM/3, direct memory ≈ heap). Left empty,
	// the image auto-sizes from container memory, which is fine for
	// evaluation but not what Sonatype recommends for production.
	JavaHeap string
}

func (c Nexus3Config) ComposeYAML() string {
	var b strings.Builder

	b.WriteString("services:\n")
	b.WriteString("  nexus3:\n")
	fmt.Fprintf(&b, "    image: sonatype/nexus3:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", c.ContainerName)
	b.WriteString("    restart: always\n")

	if c.JavaHeap != "" {
		b.WriteString("    environment:\n")
		fmt.Fprintf(&b, "      - INSTALL4J_ADD_VM_PARAMS=-Xms%s -Xmx%s -XX:MaxDirectMemorySize=%s\n",
			c.JavaHeap, c.JavaHeap, c.JavaHeap)
	}

	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:8081'\n", c.HTTPPort)
	if c.ExtraPort > 0 {
		fmt.Fprintf(&b, "      - '%d:%d'\n", c.ExtraPort, c.ExtraPort)
	}

	// nexus-data must be writable by uid 200 (the container's nexus user).
	// If it's a fresh host directory owned by root, Nexus fails to start
	// until you `chown -R 200:200` it on the host first.
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/nexus-data:/nexus-data'\n", c.DeployDir)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}

	if c.CPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.CPULimit)
	}
	if c.MemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.MemoryLimit)
	}

	// Verified against a real container: curl is present in this image
	// (Alpine-based) and /service/rest/v1/status returns 200 once Nexus
	// has finished starting. start_period is generous — first boot can
	// take a couple of minutes.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"curl\", \"-f\", \"http://localhost:8081/service/rest/v1/status\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 3m\n")

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
