// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/dockerx"
	"devops-wrapper/internal/randid"
	"devops-wrapper/internal/sshexec"
)

// An app's .env is kept by the panel and written to each server's deploy
// folder on a deploy; saving the app doesn't touch the servers, and an edit
// made on a server never comes back. EnvSync is how one server's .env
// compares with the app's, so the two can't drift apart unseen.
type EnvSync struct {
	ServiceID string `json:"serviceId"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	// Status: "in-sync"; "differs" (both have one); "not-on-server" (the
	// app has one, the server none: it's written on the next deploy);
	// "only-on-server" (the server has one the app doesn't); "unreachable".
	Status       string `json:"status"`
	ServerExists bool   `json:"serverExists"`
	// ServerEnv and ServerSHA are the server's file, when it differs: what
	// a pull would take, and the version it must still be.
	ServerEnv string `json:"serverEnv,omitempty"`
	ServerSHA string `json:"serverSha,omitempty"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checkedAt"`
}

// sameEnv compares two .env files as compose reads them: line endings and
// trailing blank lines don't matter.
func sameEnv(a, b string) bool {
	norm := func(s string) string {
		return strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), " \t\r\n")
	}
	return norm(a) == norm(b)
}

// envStatus is the comparison of the app's .env with a server's.
func envStatus(appEnv, serverEnv string, serverExists bool) string {
	appHas := strings.TrimSpace(appEnv) != ""
	switch {
	case !serverExists && !appHas:
		return "in-sync"
	case !serverExists:
		return "not-on-server"
	case !appHas && strings.TrimSpace(serverEnv) != "":
		return "only-on-server"
	case sameEnv(appEnv, serverEnv):
		return "in-sync"
	}
	return "differs"
}

func envPath(dir string) string { return path.Join(dir, ".env") }

// checkEnv reads one server's .env and compares it with the app's.
func (h *Handler) checkEnv(ctx context.Context, a *App, s *Service) EnvSync {
	res := EnvSync{ServiceID: s.ID, Host: s.Host, Path: envPath(s.DeployDir), CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	client, err := h.dialTarget(ctx, &connectTarget{Host: s.Host, Port: s.Port, Username: s.Username})
	if err != nil {
		res.Status, res.Error = "unreachable", err.Error()
		return res
	}
	defer client.Close()
	content, exists, err := readFile(client, res.Path)
	if err != nil {
		res.Status, res.Error = "unreachable", err.Error()
		return res
	}
	res.ServerExists = exists
	res.Status = envStatus(a.Env, content, exists)
	if res.Status == "differs" || res.Status == "only-on-server" {
		res.ServerEnv, res.ServerSHA = content, dockerx.SHA(content)
	}
	return res
}

// maxEnvChecks bounds how many servers are read at once.
const maxEnvChecks = 4

// handleEnvSync compares the app's .env with every server's.
func (h *Handler) handleEnvSync(w http.ResponseWriter, r *http.Request) {
	a, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}
	if err := h.withServers(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]EnvSync, len(a.Servers))
	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(maxEnvChecks)
	for i, s := range a.Servers {
		g.Go(func() error {
			out[i] = h.checkEnv(ctx, a, s)
			return nil
		})
	}
	g.Wait()
	writeJSON(w, out, nil)
}

type envActionRequest struct {
	ServiceID string `json:"serviceId"`
	// ServerSHA, for a pull: the server's .env must still be the version
	// that was shown.
	ServerSHA string `json:"serverSha"`
}

// envTarget finds the app and one of its servers for a pull or push.
func (h *Handler) envTarget(w http.ResponseWriter, r *http.Request) (*App, *Service, *envActionRequest, *sshexec.Client, bool) {
	var req envActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return nil, nil, nil, nil, false
	}
	a, ok := h.apps.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return nil, nil, nil, nil, false
	}
	if err := h.withServers(r.Context(), a); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, nil, nil, false
	}
	var svc *Service
	for _, s := range a.Servers {
		if s.ID == req.ServiceID {
			svc = s
		}
	}
	if svc == nil {
		http.Error(w, "that server isn't one of this app's", http.StatusNotFound)
		return nil, nil, nil, nil, false
	}
	client, err := h.dialTarget(r.Context(), &connectTarget{Host: svc.Host, Port: svc.Port, Username: svc.Username})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return nil, nil, nil, nil, false
	}
	return a, svc, &req, client, true
}

// handlePullEnv makes a server's .env the app's: the app's .env is replaced
// by the server's, if it's still the version that was shown. Nothing is
// written to any server; the other servers get it on the next deploy.
func (h *Handler) handlePullEnv(w http.ResponseWriter, r *http.Request) {
	a, svc, req, client, ok := h.envTarget(w, r)
	if !ok {
		return
	}
	defer client.Close()
	p := envPath(svc.DeployDir)
	content, exists, err := readFile(client, p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !exists {
		http.Error(w, svc.Host+" has no "+p+" to take", http.StatusConflict)
		return
	}
	if req.ServerSHA == "" || dockerx.SHA(content) != req.ServerSHA {
		http.Error(w, p+" on "+svc.Host+" changed since it was compared; check again", http.StatusConflict)
		return
	}
	started := time.Now()
	updated, err := h.apps.Update(r.Context(), a.ID, func(a *App) { a.Env = content })
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.recordEnv(r.Context(), svc, "env pull", fmt.Sprintf("The app's .env was replaced by %s on %s (%d bytes).\nThe app's other servers get it on the next deploy.\n", p, svc.Host, len(content)), started)
	h.withServers(r.Context(), updated)
	updated.analyze()
	writeJSON(w, updated, nil)
}

// handlePushEnv writes the app's .env to one server's deploy folder, the
// previous file kept under /var/backups/devops-panel. Containers keep their
// environment until the stack is brought up again (Deploy).
func (h *Handler) handlePushEnv(w http.ResponseWriter, r *http.Request) {
	a, svc, _, client, ok := h.envTarget(w, r)
	if !ok {
		return
	}
	defer client.Close()
	if strings.TrimSpace(a.Env) == "" {
		http.Error(w, "the app has no .env to write; to keep the server's, take it into the app instead", http.StatusBadRequest)
		return
	}
	p := envPath(svc.DeployDir)
	started := time.Now()
	current, exists, err := readFile(client, p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var out strings.Builder
	if exists {
		if sameEnv(current, a.Env) {
			writeJSON(w, map[string]any{"ok": true, "log": p + " already matches the app's .env; nothing written.\n"}, nil)
			return
		}
		backup := dockerx.BackupPath(p, started.UTC().Format("20060102-150405"))
		cmd := sshexec.AsRootPrelude + fmt.Sprintf("$AS_ROOT mkdir -p %s && $AS_ROOT cp -p %s %s",
			sshexec.ShellQuote(path.Dir(backup)), sshexec.ShellQuote(p), sshexec.ShellQuote(backup))
		if _, errOut, err := client.Output(cmd); err != nil {
			http.Error(w, fmt.Sprintf("backing up %s failed, so nothing was written: %v %s", p, err, strings.TrimSpace(errOut)), http.StatusBadGateway)
			return
		}
		fmt.Fprintf(&out, "Previous version kept at %s\n", backup)
	}
	if err := client.WriteFile(p, []byte(a.Env), &out); err != nil {
		http.Error(w, fmt.Sprintf("writing %s failed: %v", p, err), http.StatusBadGateway)
		return
	}
	fmt.Fprintf(&out, "Wrote the app's .env to %s on %s.\nRunning containers keep their environment until the stack is deployed again.\n", p, svc.Host)
	h.recordEnv(r.Context(), svc, "env push", out.String(), started)
	writeJSON(w, map[string]any{"ok": true, "log": out.String()}, nil)
}

// recordEnv saves a pull or push to the server's Activity history.
func (h *Handler) recordEnv(ctx context.Context, svc *Service, action, text string, started time.Time) {
	id, err := randid.New(16)
	if err != nil {
		log.Printf("service logs: env id: %v", err)
		return
	}
	finished := time.Now()
	if err := h.logs.Record(ctx, LogEntry{ID: id, ServiceID: svc.ID, ServiceName: svc.Name, Host: svc.Host, Action: action, Status: "success", Log: text, StartedAt: started, FinishedAt: &finished}); err != nil {
		log.Printf("service logs: record %s for %s: %v", action, svc.ID, err)
	}
}
