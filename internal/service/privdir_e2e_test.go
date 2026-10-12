// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/sshexec"
)

// TestRootOnlyFolderEndToEnd: a project in a folder the SSH user can't
// enter (root-only, like /root/... or another user's home) is deployed,
// health-checked, stopped and started through sudo — as on a server where
// the SSH user isn't root. E2E_SSH_USER must not be root.
func TestRootOnlyFolderEndToEnd(t *testing.T) {
	target, password := os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_PASSWORD")
	if target == "" || password == "" || e2eUser() == "root" {
		t.Skip("set E2E_SSH_TARGET, E2E_SSH_PASSWORD and a non-root E2E_SSH_USER to a throwaway server to run")
	}
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	ssh := sshTarget(host, port, password)
	c, err := sshexec.Dial(ssh)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	const dir = "/srv/locked/app"
	if _, errOut, err := c.Output(sshexec.AsRoot("sh -c 'mkdir -p /srv/locked && chmod 700 /srv/locked'")); err != nil {
		t.Fatalf("setup: %v %s", err, errOut)
	}
	if _, _, err := c.Output("cd " + dir + " 2>/dev/null || cd /srv/locked"); err == nil {
		t.Fatal("the SSH user can enter /srv/locked; the test needs a folder it can't")
	}

	jobs := deploy.NewStore()
	job := jobs.RunComposeProject(ssh, "services:\n  web:\n    image: nginx:alpine\n    ports: [\"18090:80\"]\n", dir, "docker-compose.yml",
		[]deploy.ProjectFile{{Path: ".env", Content: "X=1\n"}})
	for job.Snapshot().Status == deploy.StatusRunning {
		time.Sleep(time.Second)
	}
	if log := strings.Join(job.Snapshot().Log, ""); job.Snapshot().Status != deploy.StatusSuccess {
		t.Fatalf("deploy into a root-only folder failed:\n%s", log)
	}

	svc := &Service{Type: "docker", DeployDir: dir, Status: StatusDeployed}
	health, detail, obs := CheckHealth(ssh, svc)
	if health != "healthy" || len(obs.Ports) != 1 {
		t.Fatalf("health %s %q, ports %+v", health, detail, obs.Ports)
	}
	t.Logf("health: %s (%s), port %s %s", health, detail, obs.Ports[0].HostPort, obs.Ports[0].State)

	for _, action := range []string{"stop", "start"} {
		if err := c.Run(sshexec.AsRootPrelude+deploy.Project{Dir: dir}.Command()+" "+action, &strings.Builder{}); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if health, detail, _ := CheckHealth(ssh, svc); health != "healthy" {
		t.Errorf("after stop+start: %s %q", health, detail)
	}
}
