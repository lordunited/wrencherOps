// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"devops-wrapper/internal/sshexec"
)

// TestPortsEndToEnd runs a stack with every kind of published port on a
// throwaway Docker server and checks the health check reports each one:
// 18081 open (the server's port is reachable from the panel), 18083 not
// reachable (published on the server, but nothing forwards it to the panel
// — as a firewall would), a loopback-only port, a UDP port, and a stopped
// container's port. The server must forward 18081 to the panel's side and
// not 18083. Runs only with E2E_PORTS=1, E2E_SSH_TARGET (host:port) and
// E2E_SSH_PASSWORD. Never point it at a real server.
func TestPortsEndToEnd(t *testing.T) {
	target, password := os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if os.Getenv("E2E_PORTS") != "1" || target == "" || password == "" {
		t.Skip("set E2E_PORTS=1, E2E_SSH_TARGET and E2E_SSH_PASSWORD to a throwaway server to run")
	}
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	ssh := sshTarget(host, port, password)
	c, err := sshexec.Dial(ssh)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	const dir = "/opt/ports"
	compose := `services:
  web:
    image: nginx:alpine
    ports: ["18081:80", "18083:80", "127.0.0.1:18084:80", "18085:53/udp"]
  db:
    image: nginx:alpine
    ports: ["18086:80"]
`
	setup := "mkdir -p " + dir + " && cd " + dir + " && printf %s " + sshexec.ShellQuote(compose) + " > docker-compose.yml && docker compose up -d && docker compose stop db"
	if _, errOut, err := c.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(setup))); err != nil {
		t.Fatalf("setup: %v %s", err, errOut)
	}

	health, detail, obs := CheckHealth(ssh, &Service{Type: "docker", DeployDir: dir})
	t.Logf("health %s: %s", health, detail)
	got := map[string]PortStatus{}
	for _, p := range obs.Ports {
		got[p.HostIP+":"+p.HostPort+"/"+p.Protocol] = p
		t.Logf("%s %s:%s->%s/%s %s %s", p.Service, p.HostIP, p.HostPort, p.ContainerPort, p.Protocol, p.State, p.Detail)
	}
	for key, want := range map[string]string{
		":18081/tcp":          "open",
		":18083/tcp":          "closed",
		"127.0.0.1:18084/tcp": "local",
		":18085/udp":          "published",
		":18086/tcp":          "stopped",
	} {
		if p, ok := got[key]; !ok || p.State != want {
			t.Errorf("%s: %+v, want state %s", key, p, want)
		}
	}
	if len(obs.Ports) != 5 {
		t.Errorf("%d ports, want 5 (IPv4 and IPv6 bindings of one port counted once)", len(obs.Ports))
	}
}
