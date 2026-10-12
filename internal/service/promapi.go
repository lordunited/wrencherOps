// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// The PromQL editor's autocomplete and query check call the Prometheus
// HTTP API many times while someone types. Opening a fresh SSH connection
// and re-discovering the container for each call would make every
// suggestion wait on a handshake, so connections are kept for a short
// while and reused.

const promConnIdle = 2 * time.Minute

// promAPIPath allows only the read-only endpoints the editor needs.
var promAPIPath = regexp.MustCompile(`^v1/(labels|series|metadata|query|status/flags|status/buildinfo|format_query|parse_query|label/[A-Za-z_][A-Za-z0-9_]*/values)$`)

type promConn struct {
	serviceID string
	client    *sshexec.Client
	inst      *promctl.Instance
	lastUsed  time.Time
}

type promConnPool struct {
	mu    sync.Mutex
	conns map[string]*promConn
}

func (p *promConnPool) key(svc *Service, password string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s",
		svc.ID, svc.Host, svc.Port, svc.Username, svc.DeployDir, svc.Container, password)))
	return hex.EncodeToString(sum[:])
}

// get returns a live connection for svc, opening one if needed. It also
// closes connections that have sat idle past promConnIdle.
func (p *promConnPool) get(svc *Service, password string) (*promConn, error) {
	k := p.key(svc, password)
	p.mu.Lock()
	if p.conns == nil {
		p.conns = map[string]*promConn{}
	}
	now := time.Now()
	for key, c := range p.conns {
		if now.Sub(c.lastUsed) > promConnIdle {
			c.client.Close()
			delete(p.conns, key)
		}
	}
	if c, ok := p.conns[k]; ok {
		c.lastUsed = now
		p.mu.Unlock()
		if c.client.Alive() {
			return c, nil
		}
		p.drop(svc, password)
	} else {
		p.mu.Unlock()
	}

	client, err := sshexec.Dial(sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password})
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %v", err)
	}
	inst, err := discoverPrometheus(client, svc)
	if err != nil {
		client.Close()
		return nil, err
	}
	c := &promConn{serviceID: svc.ID, client: client, inst: inst, lastUsed: time.Now()}
	p.mu.Lock()
	if old, ok := p.conns[k]; ok {
		old.client.Close()
	}
	p.conns[k] = c
	p.mu.Unlock()
	return c, nil
}

// dropService closes every kept connection of a service, so the next use
// rediscovers its container (after its mounts changed, say).
func (p *promConnPool) dropService(serviceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, c := range p.conns {
		if c.serviceID == serviceID {
			c.client.Close()
			delete(p.conns, k)
		}
	}
}

func (p *promConnPool) drop(svc *Service, password string) {
	k := p.key(svc, password)
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[k]; ok {
		c.client.Close()
		delete(p.conns, k)
	}
}

// handlePromAPI proxies a read-only Prometheus API call for the service's
// Prometheus, using the saved SSH password. The response is Prometheus's
// own JSON, status included, so a PromQL error arrives with its message.
func (h *Handler) handlePromAPI(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	if !promAPIPath.MatchString(rest) {
		http.Error(w, "not an allowed Prometheus API call", http.StatusNotFound)
		return
	}
	svc, ok := h.services.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if !hasPrometheus(svc) {
		http.Error(w, "this service does not run Prometheus", http.StatusBadRequest)
		return
	}
	password, err := h.resolvePassword(r.Context(), svc, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	target := "/api/" + rest
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	for attempt := 0; attempt < 2; attempt++ {
		conn, err := h.promPool.get(svc, password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		status, body, err := promctl.APIGet(r.Context(), conn.client, conn.inst, target)
		if err != nil {
			// The connection or container may have gone away; start over once.
			h.promPool.drop(svc, password)
			continue
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(body)
		return
	}
	http.Error(w, "Prometheus did not answer", http.StatusBadGateway)
}
