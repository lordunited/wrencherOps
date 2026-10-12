// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package dockerx has helpers for working with a Docker container on a
// remote server over SSH: inspecting it, mapping its paths to the server's,
// reading its files, and writing files with a backup.
package dockerx

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"devops-wrapper/internal/sshexec"
)

// Container is what the panel needs to know about a running container.
type Container struct {
	Name   string
	Image  string
	Args   []string
	State  string
	Labels map[string]string
	Mounts []Mount
	IP     string
}

// Mount is one bind mount: a server path shown inside the container.
type Mount struct {
	Source      string
	Destination string
}

// Inspect reads a container's details.
func Inspect(c *sshexec.Client, name string) (*Container, error) {
	out, errOut, err := c.Output(sshexec.AsRoot("docker inspect --format '{{json .}}' " + sshexec.ShellQuote(name)))
	if err != nil {
		return nil, fmt.Errorf("container %s not found on the server: %v %s", name, err, strings.TrimSpace(errOut))
	}
	var raw struct {
		Name   string   `json:"Name"`
		Args   []string `json:"Args"`
		Config struct {
			Image  string            `json:"Image"`
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status string `json:"Status"`
		} `json:"State"`
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
		NetworkSettings struct {
			Networks map[string]struct {
				IPAddress string `json:"IPAddress"`
			} `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return nil, fmt.Errorf("reading container details: %v", err)
	}
	ct := &Container{
		Name:   strings.TrimPrefix(raw.Name, "/"),
		Image:  raw.Config.Image,
		Args:   raw.Args,
		State:  raw.State.Status,
		Labels: raw.Config.Labels,
	}
	if ct.Name == "" {
		ct.Name = name
	}
	for _, n := range raw.NetworkSettings.Networks {
		if n.IPAddress != "" {
			ct.IP = n.IPAddress
			break
		}
	}
	for _, m := range raw.Mounts {
		if m.Type == "bind" {
			ct.Mounts = append(ct.Mounts, Mount{Source: m.Source, Destination: m.Destination})
		}
	}
	return ct, nil
}

// HostPath maps a path inside the container to the server path it's
// bind-mounted from; ok is false when it only exists in the container.
func (ct *Container) HostPath(p string) (hostPath string, ok bool) {
	best := -1
	for _, m := range ct.Mounts {
		d := strings.TrimRight(m.Destination, "/")
		if p == d || strings.HasPrefix(p, d+"/") {
			if len(d) > best {
				best = len(d)
				hostPath = strings.TrimRight(m.Source, "/") + p[len(d):]
				ok = true
			}
		}
	}
	return hostPath, ok
}

// Compose returns the container's Compose project, working folder and
// compose files, empty when it wasn't started with Compose.
func (ct *Container) Compose() (project, dir string, files []string) {
	project = ct.Labels["com.docker.compose.project"]
	dir = ct.Labels["com.docker.compose.project.working_dir"]
	if f := ct.Labels["com.docker.compose.project.config_files"]; f != "" {
		files = strings.Split(f, ",")
	}
	return project, dir, files
}

// ImageBase is an image reference without registry, repository path, tag
// or digest: "docker.io/library/nginx:1.27-alpine" -> "nginx".
func ImageBase(image string) string {
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	image, _, _ = strings.Cut(image, "@")
	image, _, _ = strings.Cut(image, ":")
	return image
}

// FindByImage lists containers (running or not) whose image matches.
func FindByImage(c *sshexec.Client, match func(image string) bool) ([]*Container, error) {
	out, errOut, err := c.Output(sshexec.AsRoot("docker ps -a --format '{{.Names}}\t{{.Image}}'"))
	if err != nil {
		return nil, fmt.Errorf("listing containers failed (the SSH user needs access to Docker, directly or via sudo): %v %s", err, strings.TrimSpace(errOut))
	}
	var found []*Container
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, image, ok := strings.Cut(line, "\t")
		if !ok || !match(image) {
			continue
		}
		if ct, err := Inspect(c, name); err == nil {
			found = append(found, ct)
		}
	}
	return found, nil
}

// File is a file read from inside a container.
type File struct {
	Path     string `json:"path"`
	HostPath string `json:"hostPath,omitempty"`
	Content  string `json:"content"`
	SHA      string `json:"sha"`
}

// SHA is the hash used to detect that a file changed since it was read.
func SHA(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Exec builds a "docker exec" running script with sh and the given
// arguments ($1, $2, ...), all safely quoted. root runs it as uid 0
// *inside the container* — separate from, and in addition to, the sudo
// this wraps the whole command in for reaching the Docker socket itself.
func Exec(container string, root bool, script string, args ...string) string {
	cmd := "docker exec "
	if root {
		cmd += "-u 0 "
	}
	cmd += sshexec.ShellQuote(container) + " sh -c " + sshexec.ShellQuote(script) + " sh"
	for _, a := range args {
		cmd += " " + sshexec.ShellQuote(a)
	}
	return sshexec.AsRoot(cmd)
}

// ReadFiles reads the given files (globs allowed) from inside the
// container. Contents travel base64-encoded so nothing breaks the framing.
func ReadFiles(c *sshexec.Client, ct *Container, paths []string) ([]File, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	script := `for P in "$@"; do for f in $P; do [ -f "$f" ] && printf '%s %s\n' "$f" "$(base64 < "$f" | tr -d '\n')"; done; done; true`
	out, errOut, err := c.Output(Exec(ct.Name, true, script, paths...))
	if err != nil {
		return nil, fmt.Errorf("reading files: %v %s", err, strings.TrimSpace(errOut))
	}
	seen := map[string]bool{}
	var files []File
	for _, line := range strings.Split(out, "\n") {
		p, encoded, _ := strings.Cut(strings.TrimSpace(line), " ")
		if p == "" || seen[p] {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		seen[p] = true
		f := File{Path: p, Content: string(raw), SHA: SHA(string(raw))}
		f.HostPath, _ = ct.HostPath(p)
		files = append(files, f)
	}
	return files, nil
}

// BackupRoot is where the previous version of each changed file is kept on
// the server, mirroring the file's own path. Keeping backups out of config
// folders stops an include glob from loading them as live config.
const BackupRoot = "/var/backups/devops-panel"

// BackupPath is where the backup of p taken at stamp is kept.
func BackupPath(p, stamp string) string {
	return BackupRoot + path.Clean("/"+p) + "." + stamp
}

// WriteFile backs up and replaces one file. A file on a bind mount is
// written on the server, in place, so a single-file mount (which pins its
// inode) sees the change. Otherwise it's written inside the container.
// It returns a human-readable note of where it went.
func WriteFile(c *sshexec.Client, ct *Container, p, content, stamp string) (string, error) {
	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	if host, ok := ct.HostPath(p); ok {
		backup := BackupPath(host, stamp)
		cmd := sshexec.AsRootPrelude + fmt.Sprintf(
			`F=%s; B=%s; $AS_ROOT mkdir -p "$(dirname "$F")" && { [ ! -f "$F" ] || { $AS_ROOT mkdir -p "$(dirname "$B")" && $AS_ROOT cp -p "$F" "$B"; }; } && echo %s | base64 -d | $AS_ROOT tee "$F" > /dev/null`,
			sshexec.ShellQuote(host), sshexec.ShellQuote(backup), b64)
		if _, stderr, err := c.Output(cmd); err != nil {
			return "", fmt.Errorf("%v %s", err, strings.TrimSpace(stderr))
		}
		return host + " on the server (previous version kept at " + backup + ")", nil
	}
	backup := BackupPath(p, stamp)
	script := `F="$1"; B="$2"; mkdir -p "$(dirname "$F")"; [ ! -f "$F" ] || { mkdir -p "$(dirname "$B")" && cp -p "$F" "$B"; }; echo "$3" | base64 -d > "$F"`
	if _, stderr, err := c.Output(Exec(ct.Name, true, script, p, backup, b64)); err != nil {
		return "", fmt.Errorf("%v %s", err, strings.TrimSpace(stderr))
	}
	return p + " inside the container", nil
}

// RestoreFile puts back the backup WriteFile took at stamp, or deletes the
// file if it was newly created.
func RestoreFile(c *sshexec.Client, ct *Container, p, stamp string) error {
	if host, ok := ct.HostPath(p); ok {
		cmd := sshexec.AsRootPrelude + fmt.Sprintf(`F=%s; B=%s; if [ -f "$B" ]; then $AS_ROOT cat "$B" | $AS_ROOT tee "$F" > /dev/null; else $AS_ROOT rm -f "$F"; fi`,
			sshexec.ShellQuote(host), sshexec.ShellQuote(BackupPath(host, stamp)))
		_, _, err := c.Output(cmd)
		return err
	}
	script := `F="$1"; B="$2"; if [ -f "$B" ]; then cat "$B" > "$F"; else rm -f "$F"; fi`
	_, _, err := c.Output(Exec(ct.Name, true, script, p, BackupPath(p, stamp)))
	return err
}

// ContainerStats is one container's resource use right now, as "docker
// stats" reports it.
type ContainerStats struct {
	Name    string `json:"name"`
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"` // e.g. "128MiB / 512MiB"
	MemPerc string `json:"memPercent"`
	NetIO   string `json:"netIO"`
	PIDs    string `json:"pids"`
}

// Stats runs "docker stats" once for every named container and returns
// each one's current reading, in whatever order the daemon returns them.
//
// names must already be known to be running: unlike most docker
// subcommands, if even one name in the list doesn't exist right now (it
// was removed a moment ago, say), the whole command fails and nothing is
// returned for any of them — verified against a real daemon. A stopped
// (but still-existing) container doesn't fail the command, but prints a
// meaningless all-zero line instead of erroring, which is exactly why
// callers should only pass container names that were just observed
// running, not merely present.
func Stats(c *sshexec.Client, names []string) []ContainerStats {
	if len(names) == 0 {
		return nil
	}
	cmd := "docker stats --no-stream --format '{{json .}}'"
	for _, n := range names {
		cmd += " " + sshexec.ShellQuote(n)
	}
	out, _, _ := c.Output(sshexec.AsRoot(cmd))
	var stats []ContainerStats
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw struct{ Name, CPUPerc, MemUsage, MemPerc, NetIO, PIDs string }
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		stats = append(stats, ContainerStats{
			Name: raw.Name, CPU: raw.CPUPerc, Memory: raw.MemUsage, MemPerc: raw.MemPerc, NetIO: raw.NetIO, PIDs: raw.PIDs,
		})
	}
	return stats
}
