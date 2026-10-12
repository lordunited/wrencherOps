// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"strings"
	"testing"
)

// The stats listener is only ever queried from inside the container, so
// publishing it would expose HAProxy's stats page to anyone who can reach
// the server, for no benefit.
func TestHAProxyComposeDoesNotPublishStatsPort(t *testing.T) {
	yaml := HAProxyConfig{HTTPPort: 80, HTTPSPort: 443, ImageTag: "lts-alpine", DeployDir: "/opt/haproxy", ContainerName: "haproxy"}.ComposeYAML()
	if strings.Contains(yaml, fmt.Sprintf(":%d'", HAProxyStatsPort)) {
		t.Errorf("compose publishes the stats port %d:\n%s", HAProxyStatsPort, yaml)
	}
}
