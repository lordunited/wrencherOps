// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestContainerStatesEndToEnd: paused, stopped and mixed stacks on a real
// server read for what they are. The server needs these compose projects
// (each with services web, api, worker on nginx:alpine): /opt/mixed (api
// paused, worker stopped), /opt/allpaused (all paused), /opt/halted (all
// stopped), /opt/gone (taken down), and /opt/jobs (a "migrate" job app waits
// to complete, then app). Runs with E2E_STATES=1, E2E_SSH_TARGET and
// E2E_SSH_PASSWORD.
func TestContainerStatesEndToEnd(t *testing.T) {
	target, password := os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if os.Getenv("E2E_STATES") != "1" || target == "" || password == "" {
		t.Skip("set E2E_STATES=1, E2E_SSH_TARGET and E2E_SSH_PASSWORD to a throwaway server to run")
	}
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	ssh := sshTarget(host, port, password)

	roles := func(obs Observed) string {
		var r []string
		for _, c := range obs.Containers {
			r = append(r, c.Service+"="+c.Role)
		}
		return strings.Join(r, " ")
	}
	for _, c := range []struct {
		dir, wantHealth, wantDetail, wantRoles string
	}{
		{"/opt/mixed", "degraded", "1/3 running · paused: api · stopped: worker (exited with code 0)", "api=paused web=running worker=stopped"},
		{"/opt/allpaused", "paused", "all 3 containers paused", "api=paused web=paused worker=paused"},
		{"/opt/halted", "stopped", "stopped — 3 containers not running", "api=stopped web=stopped worker=stopped"},
		{"/opt/gone", "down", "nothing of it runs", ""},
		{"/opt/jobs", "healthy", "1/1 running, 1 setup job completed", "app=running migrate=job"},
	} {
		health, detail, obs := CheckHealth(ssh, &Service{Type: "docker", DeployDir: c.dir, Status: StatusDeployed})
		t.Logf("%-15s %-9s %s  [%s]", c.dir, health, detail, roles(obs))
		if health != c.wantHealth || !strings.Contains(detail, c.wantDetail) || roles(obs) != c.wantRoles {
			t.Errorf("%s: %s %q [%s]; want %s %q [%s]", c.dir, health, detail, roles(obs), c.wantHealth, c.wantDetail, c.wantRoles)
		}
	}
	// Every container carries its image, both ways of reading them.
	_, _, obs := CheckHealth(ssh, &Service{Type: "docker", DeployDir: "/opt/mixed", Status: StatusDeployed})
	_, _, watched := CheckHealth(ssh, &Service{Type: "docker", Origin: OriginConnected, ComposeProject: "mixed", Status: StatusConnected})
	for _, c := range append(obs.Containers, watched.Containers...) {
		if c.Image != "nginx:alpine" {
			t.Errorf("%s: image %q, want nginx:alpine", c.Name, c.Image)
		}
	}
	// A watched (connected) stack reads containers by project label instead
	// of compose: paused still reads as paused.
	health, detail, _ := CheckHealth(ssh, &Service{Type: "docker", Origin: OriginConnected, ComposeProject: "allpaused", Status: StatusConnected})
	if health != "paused" {
		t.Errorf("connected paused stack: %s %q", health, detail)
	}
}
