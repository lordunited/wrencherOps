// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/sshexec"
)

// Env files of a detected project — the .env compose reads to fill in
// ${VARIABLES}, and every env_file its services load — shown on Detect and
// editable in place on the server: each save is checked against the version
// that was read, backed up, and optionally applied with "docker compose up
// -d", which recreates exactly the containers whose environment changed.

// EnvFile is one env file of a detected project.
type EnvFile struct {
	Path   string `json:"path"`  // absolute, on the server
	Label  string `json:"label"` // as the compose writes it, or ".env"
	Exists bool   `json:"exists"`
}

const fileMarker = "@@PANEL-FILE@@"

// envFilesOf reads the project's compose files and .env, and lists the env
// files they use: .env first (when it exists, or when the compose has a
// ${VARIABLE} to fill in), then every env_file. Relative env_file paths
// are relative to the project's folder, as compose resolves them.
func envFilesOf(client *sshexec.Client, p *DetectedProject) []EnvFile {
	envPath := p.WorkingDir + "/.env"
	var script strings.Builder
	for _, f := range append([]string{envPath}, p.ConfigFiles...) {
		q := sshexec.ShellQuote(f)
		fmt.Fprintf(&script, "echo %s; [ -f %s ] && cat %s; echo; ", fileMarker, q, q)
	}
	out, _, err := client.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(script.String())))
	if err != nil {
		return nil
	}
	parts := strings.Split(out, fileMarker+"\n")
	if len(parts) < 2 {
		return nil
	}
	env := strings.TrimSuffix(parts[1], "\n")

	var files []EnvFile
	seen := map[string]bool{}
	add := func(abs, label string) {
		if !seen[abs] {
			seen[abs] = true
			files = append(files, EnvFile{Path: abs, Label: label})
		}
	}
	usesVars := false
	for _, compose := range parts[2:] {
		if strings.Contains(compose, "${") {
			usesVars = true
		}
		a := deploy.AnalyzeProject(compose, env, nil, p.WorkingDir)
		for _, ef := range a.EnvFiles {
			abs := ef
			if !path.IsAbs(ef) {
				abs = path.Join(p.WorkingDir, ef)
			}
			if abs == envPath {
				continue
			}
			add(abs, ef)
		}
	}
	if strings.TrimSpace(env) != "" || usesVars {
		files = append([]EnvFile{{Path: envPath, Label: ".env"}}, files...)
		seen[envPath] = true
	}
	if len(files) == 0 {
		return nil
	}

	var check strings.Builder
	for _, f := range files {
		q := sshexec.ShellQuote(f.Path)
		fmt.Fprintf(&check, "[ -f %s ] && echo %s; ", q, q)
	}
	check.WriteString("true")
	existing, _, _ := client.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(check.String())))
	for i := range files {
		for _, line := range strings.Split(existing, "\n") {
			if strings.TrimSpace(line) == files[i].Path {
				files[i].Exists = true
			}
		}
	}
	return files
}

type envFileRequest struct {
	connectTarget
	WorkingDir string `json:"workingDir"`
	Path       string `json:"path"`
	Content    string `json:"content"`
	BaseSHA    string `json:"baseSha"` // "" when the file didn't exist when read
	Apply      bool   `json:"apply"`
}

// envFileSession connects, re-finds the project, and checks path is one of
// its env files — the compose decides which files are editable here.
func (h *Handler) envFileSession(w http.ResponseWriter, r *http.Request) (*envFileRequest, *sshexec.Client, *DetectedProject, *EnvFile, bool) {
	var req envFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return nil, nil, nil, nil, false
	}
	client, err := h.dialTarget(r.Context(), &req.connectTarget)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, nil, false
	}
	p, err := h.findProject(r, client, req.Host, req.WorkingDir)
	if err != nil {
		client.Close()
		http.Error(w, err.Error(), http.StatusConflict)
		return nil, nil, nil, nil, false
	}
	for i := range p.EnvFiles {
		if p.EnvFiles[i].Path == req.Path {
			return &req, client, p, &p.EnvFiles[i], true
		}
	}
	client.Close()
	http.Error(w, req.Path+" isn't an env file of this project's compose", http.StatusBadRequest)
	return nil, nil, nil, nil, false
}

func readFile(client *sshexec.Client, p string) (content string, exists bool, err error) {
	q := sshexec.ShellQuote(p)
	out, errOut, err := client.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(fmt.Sprintf("if [ -f %s ]; then echo yes; cat %s; else echo no; fi", q, q))))
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %v %s", p, err, strings.TrimSpace(errOut))
	}
	flag, rest, _ := strings.Cut(out, "\n")
	return rest, flag == "yes", nil
}

type envFileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA     string `json:"sha"` // "" when the file doesn't exist yet
	Exists  bool   `json:"exists"`
}

func (h *Handler) handleReadEnvFile(w http.ResponseWriter, r *http.Request) {
	req, client, _, _, ok := h.envFileSession(w, r)
	if !ok {
		return
	}
	defer client.Close()
	content, exists, err := readFile(client, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	resp := envFileContent{Path: req.Path, Content: content, Exists: exists}
	if exists {
		resp.SHA = dockerx.SHA(content)
	}
	writeJSON(w, resp, nil)
}

// handleSaveEnvFile writes an env file on the server: only if it's still the
// version that was read, with the previous one kept under
// /var/backups/devops-panel. With apply, the project is then brought up again
// as it runs (its own project name and compose files), so compose recreates
// the containers whose environment changed and leaves the rest alone.
type envFileSaved struct {
	applyResponse
	SHA string `json:"sha"` // of what's on the server now: the base for the next save
}

func (h *Handler) handleSaveEnvFile(w http.ResponseWriter, r *http.Request) {
	req, client, p, _, ok := h.envFileSession(w, r)
	if !ok {
		return
	}
	defer client.Close()
	if strings.ContainsRune(req.Content, 0) {
		http.Error(w, "an env file must be text", http.StatusBadRequest)
		return
	}

	current, exists, err := readFile(client, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	switch {
	case req.BaseSHA == "" && exists:
		http.Error(w, req.Path+" was created on the server since it was read; open it again", http.StatusConflict)
		return
	case req.BaseSHA != "" && (!exists || dockerx.SHA(current) != req.BaseSHA):
		http.Error(w, req.Path+" was changed on the server since it was read; open it again", http.StatusConflict)
		return
	}

	var log strings.Builder
	if exists {
		backup := dockerx.BackupPath(req.Path, time.Now().UTC().Format("20060102-150405"))
		cmd := sshexec.AsRootPrelude + fmt.Sprintf("$AS_ROOT mkdir -p %s && $AS_ROOT cp -p %s %s",
			sshexec.ShellQuote(path.Dir(backup)), sshexec.ShellQuote(req.Path), sshexec.ShellQuote(backup))
		if _, errOut, err := client.Output(cmd); err != nil {
			http.Error(w, fmt.Sprintf("backing up %s failed, so nothing was written: %v %s", req.Path, err, strings.TrimSpace(errOut)), http.StatusBadGateway)
			return
		}
		fmt.Fprintf(&log, "Previous version kept at %s\n", backup)
	}
	if err := client.WriteFile(req.Path, []byte(req.Content), &log); err != nil {
		http.Error(w, fmt.Sprintf("writing %s failed: %v", req.Path, err), http.StatusBadGateway)
		return
	}
	fmt.Fprintf(&log, "Wrote %s\n", req.Path)

	resp := envFileSaved{applyResponse: applyResponse{OK: true}, SHA: dockerx.SHA(req.Content)}
	if req.Apply {
		if !strings.Contains(p.Status, "running") {
			// Bringing it up would start a stopped project, not just apply a file.
			fmt.Fprintln(&log, "Not applied: the project isn't running; it uses the new file when it next starts.")
		} else {
			project := deploy.Project{Dir: p.WorkingDir, Name: p.Name, Files: p.ConfigFiles}
			fmt.Fprintln(&log, "Applying (docker compose up -d): containers whose environment changed are recreated...")
			if err := client.Run(sshexec.AsRootPrelude+project.Command()+" up -d", &log); err != nil {
				resp.OK = false
				resp.Error = "the file was saved, but applying it failed: " + err.Error()
			} else {
				fmt.Fprintln(&log, "Applied.")
			}
		}
	}
	resp.Log = log.String()
	writeJSON(w, resp, nil)
}
