// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

// Observed is what a health check saw besides the verdict: every running
// container's resource use, every port the stack publishes, and every
// container's state (running, paused, stopped, ...).
type Observed struct {
	Stats      []dockerx.ContainerStats
	Ports      []PortStatus
	Containers []ContainerState
}

// PortStatus is one port a container publishes on the server, as Docker
// has it (so it's what the compose's "ports:" became), and whether it
// answers right now.
type PortStatus struct {
	Service       string `json:"service"` // the compose service, or the container's name outside Compose
	Container     string `json:"container"`
	HostIP        string `json:"hostIp"` // "" for every address
	HostPort      string `json:"hostPort"`
	ContainerPort string `json:"containerPort"`
	Protocol      string `json:"protocol"` // "tcp" | "udp" | "sctp"
	// State: "open" (accepts TCP connections from the panel), "closed" (its
	// container runs, but the panel can't connect), "local" (bound to the
	// server's loopback, so reachable only on the server), "published" (UDP
	// or SCTP, which can't be checked by connecting), or "stopped" (its
	// container isn't running, so nothing listens).
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

// containerPorts reads the published ports of the named containers in one
// round trip. A running container's actual bindings are used (they include
// a port Docker picked itself); a stopped one's configured bindings say
// what it would publish.
func containerPorts(client *sshexec.Client, names []string) []PortStatus {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = sshexec.ShellQuote(n)
	}
	format := `{{.Name}}	{{.State.Running}}	{{index .Config.Labels "com.docker.compose.service"}}	{{json .HostConfig.PortBindings}}	{{json .NetworkSettings.Ports}}`
	out, _, err := client.Output(sshexec.AsRoot("docker inspect --format " + sshexec.ShellQuote(format) + " " + strings.Join(quoted, " ")))
	if err != nil && strings.TrimSpace(out) == "" {
		return nil
	}
	return parsePorts(out)
}

func parsePorts(out string) []PortStatus {
	var ports []PortStatus
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		name, running, service := strings.TrimPrefix(f[0], "/"), f[1] == "true", f[2]
		if service == "" {
			service = name
		}
		bindings := map[string][]portBinding{}
		raw := f[3]
		if running {
			raw = f[4]
		}
		json.Unmarshal([]byte(raw), &bindings)
		for spec, list := range bindings {
			cport, proto, _ := strings.Cut(spec, "/")
			if proto == "" {
				proto = "tcp"
			}
			seen := map[string]bool{}
			for _, b := range list {
				// Docker binds IPv4 and IPv6 separately; on every address,
				// that's still one port.
				ip := b.HostIP
				if ip == "0.0.0.0" || ip == "::" {
					ip = ""
				}
				key := ip + "|" + b.HostPort
				if seen[key] {
					continue
				}
				seen[key] = true
				p := PortStatus{Service: service, Container: name, HostIP: ip, HostPort: b.HostPort, ContainerPort: cport, Protocol: proto}
				if !running {
					p.State, p.Detail = "stopped", "its container isn't running"
				}
				ports = append(ports, p)
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		a, _ := strconv.Atoi(ports[i].HostPort)
		b, _ := strconv.Atoi(ports[j].HostPort)
		if a != b {
			return a < b
		}
		if ports[i].Protocol != ports[j].Protocol {
			return ports[i].Protocol < ports[j].Protocol
		}
		return ports[i].HostIP < ports[j].HostIP
	})
	return ports
}

// portDialTimeout bounds each connection attempt: an open port answers in
// milliseconds, so only a dropped (firewalled) one ever takes this long.
const portDialTimeout = 3 * time.Second

// probePorts tries a TCP connection from the panel to every published TCP
// port of a running container, all at once — what someone using the
// service would do — and sets each one's state from the result. host is
// the address the panel reaches the server at.
func probePorts(host string, ports []PortStatus) {
	var wg sync.WaitGroup
	for i := range ports {
		p := &ports[i]
		if p.State == "stopped" {
			continue
		}
		if p.Protocol != "tcp" {
			p.State, p.Detail = "published", strings.ToUpper(p.Protocol)+" can't be checked by connecting"
			continue
		}
		if ip := net.ParseIP(p.HostIP); ip != nil && ip.IsLoopback() {
			p.State, p.Detail = "local", "bound to "+p.HostIP+": reachable only on the server itself"
			continue
		}
		addr := host
		if p.HostIP != "" {
			addr = p.HostIP
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, p.HostPort), portDialTimeout)
			if err == nil {
				conn.Close()
				p.State = "open"
				return
			}
			p.State = "closed"
			var ne net.Error
			switch {
			case errors.As(err, &ne) && ne.Timeout(), errors.Is(err, os.ErrDeadlineExceeded):
				p.Detail = "no answer within " + portDialTimeout.String() + " — a firewall between the panel and the server likely drops it"
			case strings.Contains(err.Error(), "refused"):
				p.Detail = "connection refused — nothing listens on it, or a firewall rejects it"
			default:
				p.Detail = err.Error()
			}
		}()
	}
	wg.Wait()
}
