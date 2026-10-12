// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package gitlabctl finds GitLab Omnibus containers on a remote server and
// checks whether the services inside one are running — for connecting a
// GitLab the panel didn't deploy.
package gitlabctl

import (
	"fmt"
	"path"
	"strings"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

// IsGitLabImage reports whether an image is a GitLab Omnibus build: CE, EE
// or JH.
func IsGitLabImage(image string) bool {
	switch dockerx.ImageBase(image) {
	case "gitlab-ce", "gitlab-ee", "gitlab-jh":
		return true
	}
	return false
}

// Candidate is a GitLab container found on a server, offered for connecting.
type Candidate struct {
	Container      string   `json:"container"`
	Image          string   `json:"image"`
	State          string   `json:"state"`
	ComposeProject string   `json:"composeProject,omitempty"`
	WorkingDir     string   `json:"workingDir,omitempty"`
	ComposeFiles   []string `json:"composeFiles,omitempty"`
	// Where GitLab's config (/etc/gitlab) and data (/var/opt/gitlab) live
	// on the server, when they're bind-mounted — empty when they're in a
	// Docker volume or only inside the container.
	ConfigHostPath string `json:"configHostPath,omitempty"`
	DataHostPath   string `json:"dataHostPath,omitempty"`
}

// FindAll lists every GitLab container on the server.
func FindAll(c *sshexec.Client) ([]Candidate, error) {
	cts, err := dockerx.FindByImage(c, IsGitLabImage)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, ct := range cts {
		project, dir, files := ct.Compose()
		cand := Candidate{Container: ct.Name, Image: ct.Image, State: ct.State, ComposeProject: project, WorkingDir: dir, ComposeFiles: files}
		cand.ConfigHostPath, _ = ct.HostPath("/etc/gitlab")
		cand.DataHostPath, _ = ct.HostPath("/var/opt/gitlab")
		out = append(out, cand)
	}
	return out, nil
}

// ServiceState is one of the services GitLab runs inside its container
// (puma, sidekiq, gitaly, postgresql, nginx, ...).
type ServiceState struct {
	Name string
	Up   bool
}

// ParseStatus reads "gitlab-ctl status" output, which is runit's: one line
// per service, "run: <name>: (pid N) Ns; run: log: ..." when it's running
// and "down: <name>: Ns, normally up; ..." when it isn't — verified against
// real runit. Anything that isn't a status line is skipped.
func ParseStatus(out string) []ServiceState {
	var states []ServiceState
	for _, line := range strings.Split(out, "\n") {
		state, rest, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || (state != "run" && state != "down" && state != "fail") {
			continue
		}
		name, _, _ := strings.Cut(rest, ":")
		states = append(states, ServiceState{Name: path.Base(strings.TrimSpace(name)), Up: state == "run"})
	}
	return states
}

// CheckHealth runs "gitlab-ctl status" inside the container. The command
// exits 0 even when services are down (verified with runit's own "sv
// status", which it wraps), so health comes from reading each line, not
// from the exit code. This works whatever external_url, port or TLS setup
// the GitLab has, unlike an HTTP readiness probe, which can't assume where
// its web server listens.
func CheckHealth(c *sshexec.Client, container string) (healthy bool, detail string) {
	out, errOut, err := c.Output(sshexec.AsRoot("docker exec " + sshexec.ShellQuote(container) + " gitlab-ctl status"))
	states := ParseStatus(out)
	if len(states) == 0 {
		why := strings.TrimSpace(errOut)
		if why == "" && err != nil {
			why = err.Error()
		}
		if i := strings.IndexByte(why, '\n'); i >= 0 {
			why = why[:i]
		}
		return false, "gitlab-ctl status listed no services: " + why
	}
	var down []string
	for _, s := range states {
		if !s.Up {
			down = append(down, s.Name)
		}
	}
	if len(down) > 0 {
		return false, fmt.Sprintf("%d/%d GitLab services running; down: %s", len(states)-len(down), len(states), strings.Join(down, ", "))
	}
	return true, fmt.Sprintf("all %d GitLab services running", len(states))
}
