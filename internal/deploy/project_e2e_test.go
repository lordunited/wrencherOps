// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"os"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/sshexec"
)

// TestProjectDeployEndToEnd deploys a custom compose project — compose,
// .env, a bind-mounted config, a static site folder, a named volume and a
// file well past the 128 KiB command-line limit — to real servers over SSH,
// then redeploys without one file and checks it's removed while a file
// put there by hand isn't. It needs throwaway servers with Docker and SSH
// (never real ones): E2E_SSH_TARGETS="host:port,host:port" and
// E2E_SSH_PASSWORD, root login. Skipped without them.
func TestProjectDeployEndToEnd(t *testing.T) {
	targetsEnv, password := os.Getenv("E2E_SSH_TARGETS"), os.Getenv("E2E_SSH_PASSWORD")
	if targetsEnv == "" || password == "" {
		t.Skip("set E2E_SSH_TARGETS and E2E_SSH_PASSWORD to throwaway servers to run")
	}
	const dir = "/opt/e2eapp"
	compose := `services:
  web:
    image: nginx:${NGINX_TAG}
    ports:
      - "${WEB_PORT}:80"
    volumes:
      - ./conf/default.conf:/etc/nginx/conf.d/default.conf:ro
      - ./site:/usr/share/nginx/html:ro
      - logs:/var/log/nginx
volumes:
  logs:
`
	conf := "server {\n    listen 80;\n    location / { root /usr/share/nginx/html; }\n    location /conf { return 200 \"from the bind-mounted config\\n\"; }\n}\n"
	big := strings.Repeat("0123456789abcdef", 600*1024/16) // 600 KiB, far past the 128 KiB argument limit
	files := []ProjectFile{
		{Path: ".env", Content: "NGINX_TAG=alpine\nWEB_PORT=18181\n"},
		{Path: "conf/default.conf", Content: conf},
		{Path: "site/index.html", Content: "hello from the project\n"},
		{Path: "data/big.txt", Content: big},
	}
	if err := ValidateProjectFiles(files[1:]); err != nil {
		t.Fatal(err)
	}

	store := NewStore()
	deployTo := func(t *testing.T, target sshexec.Target, files []ProjectFile) {
		t.Helper()
		job := store.RunComposeProject(target, compose, dir, "docker-compose.yml", files)
		deadline := time.Now().Add(4 * time.Minute)
		for job.Snapshot().Status == StatusRunning {
			if time.Now().After(deadline) {
				t.Fatalf("deploy to %s timed out:\n%s", target.Host, strings.Join(job.Snapshot().Log, ""))
			}
			time.Sleep(time.Second)
		}
		if snap := job.Snapshot(); snap.Status != StatusSuccess {
			t.Fatalf("deploy to %s:%d failed:\n%s", target.Host, target.Port, strings.Join(snap.Log, ""))
		}
	}
	run := func(t *testing.T, c *sshexec.Client, cmd string) string {
		t.Helper()
		out, errOut, err := c.Output(cmd)
		if err != nil {
			t.Fatalf("%s: %v %s", cmd, err, errOut)
		}
		return strings.TrimSpace(out)
	}

	for _, hp := range strings.Split(targetsEnv, ",") {
		host, port, _ := strings.Cut(hp, ":")
		user := os.Getenv("E2E_SSH_USER")
		if user == "" {
			user = "root"
		}
		target := sshexec.Target{Host: host, Port: atoiOr(port, 22), Username: user, Password: password}
		t.Run(hp, func(t *testing.T) {
			deployTo(t, target, files)

			c, err := sshexec.Dial(target)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()

			// Checks on the server run through the same sudo prelude, since the
			// SSH user may not be root.
			run = func(t *testing.T, c *sshexec.Client, cmd string) string {
				t.Helper()
				out, errOut, err := c.Output(sshexec.AsRootPrelude + "$AS_ROOT sh -c " + sshexec.ShellQuote(cmd))
				if err != nil {
					t.Fatalf("%s: %v %s", cmd, err, errOut)
				}
				return strings.TrimSpace(out)
			}
			if got := run(t, c, "wget -qO- http://127.0.0.1:18181/"); got != "hello from the project" {
				t.Errorf("site = %q (bind-mounted folder, port from .env)", got)
			}
			if got := run(t, c, "wget -qO- http://127.0.0.1:18181/conf"); got != "from the bind-mounted config" {
				t.Errorf("/conf = %q (bind-mounted config file)", got)
			}
			if got := run(t, c, "wc -c < "+dir+"/data/big.txt"); got != "614400" {
				t.Errorf("big.txt is %s bytes, want 614400", got)
			}
			if got := run(t, c, "cat "+dir+"/"+PanelFilesList); got != ".env\nconf/default.conf\ndata/big.txt\nsite/index.html" {
				t.Errorf("%s = %q", PanelFilesList, got)
			}
			if got := run(t, c, "docker volume ls --format '{{.Name}}'"); !strings.Contains(got, "e2eapp_logs") {
				t.Errorf("named volume e2eapp_logs missing; volumes: %s", got)
			}

			// Redeploy without big.txt: it goes, a hand-made file stays.
			run(t, c, "echo mine > "+dir+"/data/manual.txt")
			deployTo(t, target, []ProjectFile{files[0], files[1], files[2]})
			if got := run(t, c, "ls "+dir+"/data"); got != "manual.txt" {
				t.Errorf("after removing big.txt from the project, data/ = %q, want only manual.txt", got)
			}
			if got := run(t, c, "wget -qO- http://127.0.0.1:18181/"); got != "hello from the project" {
				t.Errorf("site after redeploy = %q", got)
			}
		})
	}
}

func atoiOr(s string, def int) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if n == 0 {
		return def
	}
	return n
}
