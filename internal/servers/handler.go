// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"devops-wrapper/internal/access"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/sshexec"
)

// checkInterval is how often every server is checked.
const checkInterval = 60 * time.Second

// checkParallel is how many servers are checked at once.
const checkParallel = 4

type Handler struct {
	store   *Store
	secrets *secretbox.Box
	// checking holds the IDs of servers being checked right now.
	checking sync.Map
}

func NewHandler(store *Store, secrets *secretbox.Box) *Handler {
	return &Handler{store: store, secrets: secrets}
}

// PathPrefixes are the URL prefixes RegisterRoutes serves.
func (h *Handler) PathPrefixes() []string {
	return []string{"/api/servers", "/api/servers/"}
}

func (h *Handler) RegisterRoutes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("GET /api/servers", h.handleList)
	mux.HandleFunc("POST /api/servers", h.handleAdd)
	mux.HandleFunc("GET /api/servers/alerts", h.handleAlerts)
	mux.HandleFunc("POST /api/servers/alerts/{id}/ack", h.handleAck)
	mux.HandleFunc("GET /api/servers/rules", h.handleRules)
	mux.HandleFunc("POST /api/servers/rules", h.handleAddRule)
	mux.HandleFunc("PUT /api/servers/rules/{id}", h.handleUpdateRule)
	mux.HandleFunc("DELETE /api/servers/rules/{id}", h.handleDeleteRule)
	mux.HandleFunc("POST /api/servers/check", h.handleCheckAll)
	mux.HandleFunc("GET /api/servers/{id}", h.handleGet)
	mux.HandleFunc("PUT /api/servers/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/servers/{id}", h.handleRemove)
	mux.HandleFunc("POST /api/servers/{id}/check", h.handleCheck)
}

// Start seeds the built-in rules and checks every server every
// checkInterval, adding servers the panel's services run on as they appear.
func (h *Handler) Start(ctx context.Context) error {
	if err := h.store.SeedRules(ctx); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			h.checkAll(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (h *Handler) checkAll(ctx context.Context) {
	if _, err := h.store.DiscoverFromServices(ctx); err != nil {
		log.Printf("servers: discovering from services: %v", err)
	}
	list, err := h.store.List(ctx)
	if err != nil {
		log.Printf("servers: %v", err)
		return
	}
	sem := make(chan struct{}, checkParallel)
	var wg sync.WaitGroup
	for _, s := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.check(ctx, s)
		}()
	}
	wg.Wait()
}

// dial logs in to s: with its own saved password, else (or when that's
// refused) with one saved for the same login on the panel's services —
// which is then kept as the server's. A password the server refuses is
// never kept.
func (h *Handler) dial(ctx context.Context, s *Server) (*sshexec.Client, error) {
	var candidates []string
	if s.EncryptedPassword != "" {
		candidates = append(candidates, s.EncryptedPassword)
	}
	if more, err := h.store.ServicePasswords(ctx, s.Host, s.Port, s.Username); err == nil {
		candidates = append(candidates, more...)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no SSH password is saved for %s@%s", s.Username, s.Host)
	}
	tried := map[string]bool{}
	var lastErr error
	for _, enc := range candidates {
		pw, err := h.secrets.Decrypt(enc)
		if err != nil || tried[pw] {
			continue
		}
		tried[pw] = true
		client, err := sshexec.Dial(sshexec.Target{Host: s.Host, Port: s.Port, Username: s.Username, Password: pw})
		if err == nil {
			if enc != s.EncryptedPassword {
				if err := h.store.SetPassword(ctx, s.ID, enc); err == nil {
					s.EncryptedPassword, s.HasSavedPassword = enc, true
				}
			}
			return client, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "unable to authenticate") {
			break // unreachable: another password won't help
		}
	}
	if lastErr != nil && strings.Contains(lastErr.Error(), "unable to authenticate") {
		return nil, fmt.Errorf("%s@%s refused the saved SSH password; enter the current one on the server's page", s.Username, s.Host)
	}
	return nil, fmt.Errorf("SSH connection failed: %v", lastErr)
}

// collect reads a snapshot over an open connection, as root.
func collect(client *sshexec.Client, loginUser string) (*Snapshot, error) {
	// The panel's address comes from the login shell's own environment:
	// sudo would reset it.
	cmd := "echo '" + mClient + "'; echo \"${SSH_CLIENT%% *}\"; " + sshexec.AsRoot("sh -c "+sshexec.ShellQuote(collectScript))
	out, errOut, err := client.Output(cmd)
	if !strings.Contains(out, mEnd) {
		msg := strings.TrimSpace(errOut)
		if msg == "" && err != nil {
			msg = err.Error()
		}
		lines := strings.Split(msg, "\n")
		return nil, fmt.Errorf("reading the server's state failed: %s", strings.TrimSpace(lines[len(lines)-1]))
	}
	return parse(out, loginUser), nil
}

// check reads s's state over one SSH connection, saves it, and brings its
// alerts in line with the rules. It returns false if a check of s was
// already running.
func (h *Handler) check(ctx context.Context, s *Server) bool {
	if _, busy := h.checking.LoadOrStore(s.ID, true); busy {
		return false
	}
	defer h.checking.Delete(s.ID)

	var snap *Snapshot
	problem := ""
	client, err := h.dial(ctx, s)
	if err != nil {
		problem = err.Error()
	} else {
		snap, err = collect(client, s.Username)
		client.Close()
		if err != nil {
			problem = err.Error()
		}
	}
	if err := h.store.SaveCheck(ctx, s.ID, snap, problem); err != nil {
		log.Printf("servers: saving %s: %v", s.Host, err)
	}
	rules, err := h.store.Rules(ctx)
	if err != nil {
		log.Printf("servers: rules: %v", err)
		return true
	}
	findings, judged := Evaluate(rules, s.ID, snap, problem)
	if _, err := h.store.ApplyFindings(ctx, s.ID, findings, judged); err != nil {
		log.Printf("servers: alerts for %s: %v", s.Host, err)
	}
	return true
}

// withAlerts fills in each server's open alerts.
func (h *Handler) withAlerts(ctx context.Context, list ...*Server) error {
	alerts, err := h.store.Alerts(ctx, false)
	if err != nil {
		return err
	}
	by := map[string]*Server{}
	for _, s := range list {
		by[s.ID] = s
	}
	for _, a := range alerts {
		if s, ok := by[a.ServerID]; ok {
			s.Alerts = append(s.Alerts, a)
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	if status != 0 {
		w.WriteHeader(status)
	}
	json.NewEncoder(w).Encode(v)
}

// handleList lists the servers. Someone who can't see the Servers tab (an
// Ansible user picking where to run, or one managing notifications) gets
// only which servers there are — not who logs in to them, their disks or
// their alerts.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !access.Allowed(r.Context(), access.View, "servers") {
		for _, s := range list {
			s.Snapshot, s.CheckError, s.CheckedAt = nil, "", nil
		}
		writeJSON(w, list, 0)
		return
	}
	if err := h.withAlerts(r.Context(), list...); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, list, 0)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	s, ok := h.store.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if err := h.withAlerts(r.Context(), s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, s, 0)
}

type serverRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// login checks a password just typed by logging in with it; it's kept
// (for the server, and for the panel's services on the same login) only if
// that works.
func (h *Handler) login(ctx context.Context, host string, port int, username, password string) (string, error) {
	client, err := sshexec.Dial(sshexec.Target{Host: host, Port: port, Username: username, Password: password})
	if err != nil {
		if strings.Contains(err.Error(), "unable to authenticate") {
			return "", fmt.Errorf("%s@%s refused that SSH password; nothing was saved", username, host)
		}
		return "", fmt.Errorf("SSH connection failed: %v", err)
	}
	client.Close()
	enc, err := h.secrets.Encrypt(password)
	if err != nil {
		return "", err
	}
	if err := h.store.SetServicesPassword(ctx, host, port, username, enc); err != nil {
		log.Printf("servers: saving the password for %s@%s's services: %v", username, host, err)
	}
	return enc, nil
}

func (h *Handler) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req serverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Host, req.Username, req.Name = strings.TrimSpace(req.Host), strings.TrimSpace(req.Username), strings.TrimSpace(req.Name)
	if req.Port == 0 {
		req.Port = 22
	}
	if req.Host == "" || req.Username == "" {
		http.Error(w, "enter the server's address and SSH user", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		req.Name = req.Host
	}
	s := &Server{Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username}
	if req.Password != "" {
		enc, err := h.login(r.Context(), req.Host, req.Port, req.Username, req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.EncryptedPassword = enc
	} else if encs, _ := h.store.ServicePasswords(r.Context(), req.Host, req.Port, req.Username); len(encs) == 0 {
		http.Error(w, fmt.Sprintf("no SSH password is saved for %s@%s yet; enter it", req.Username, req.Host), http.StatusBadRequest)
		return
	}
	if err := h.store.Add(r.Context(), s); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, errAlreadyWatched) {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
		return
	}
	h.check(r.Context(), s)
	s, _ = h.store.Get(r.Context(), s.ID)
	h.withAlerts(r.Context(), s)
	writeJSON(w, s, http.StatusCreated)
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	s, ok := h.store.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	var req serverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		s.Name = name
	}
	userChanged := strings.TrimSpace(req.Username) != "" && strings.TrimSpace(req.Username) != s.Username
	if userChanged {
		s.Username = strings.TrimSpace(req.Username)
	}
	if req.Password != "" {
		enc, err := h.login(r.Context(), s.Host, s.Port, s.Username, req.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.EncryptedPassword = enc
	} else if userChanged {
		// Another user's login: its password, if the panel has one saved.
		encs, _ := h.store.ServicePasswords(r.Context(), s.Host, s.Port, s.Username)
		if len(encs) == 0 {
			http.Error(w, fmt.Sprintf("no SSH password is saved for %s@%s yet; enter it", s.Username, s.Host), http.StatusBadRequest)
			return
		}
		s.EncryptedPassword = encs[0]
	}
	if err := h.store.Update(r.Context(), s); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if req.Password != "" || userChanged {
		h.check(r.Context(), s)
	}
	s, _ = h.store.Get(r.Context(), s.ID)
	h.withAlerts(r.Context(), s)
	writeJSON(w, s, 0)
}

func (h *Handler) handleRemove(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Remove(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	s, ok := h.store.Get(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	if !h.check(r.Context(), s) {
		// One is running already: wait for it rather than start another.
		for i := 0; i < 60; i++ {
			if _, busy := h.checking.Load(s.ID); !busy {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	s, _ = h.store.Get(r.Context(), s.ID)
	h.withAlerts(r.Context(), s)
	writeJSON(w, s, 0)
}

// handleCheckAll checks every server now (those mid-check finish their own)
// and returns the list as it is afterwards.
func (h *Handler) handleCheckAll(w http.ResponseWriter, r *http.Request) {
	h.checkAll(r.Context())
	h.handleList(w, r)
}

func (h *Handler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.store.Alerts(r.Context(), r.URL.Query().Get("all") == "1")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, alerts, 0)
}

func (h *Handler) handleAck(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Acknowledge(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, "alert not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type rulesResponse struct {
	Rules []Rule     `json:"rules"`
	Kinds []KindInfo `json:"kinds"`
}

func (h *Handler) handleRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.store.Rules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rulesResponse{Rules: rules, Kinds: Kinds}, 0)
}

// validRule checks a rule's fields, filling in a name from its kind.
func (h *Handler) validRule(ctx context.Context, rule *Rule) error {
	k, ok := kindInfo(rule.Kind)
	if !ok {
		return fmt.Errorf("unknown kind of rule %q", rule.Kind)
	}
	if rule.Severity != "warning" && rule.Severity != "critical" {
		return fmt.Errorf("severity must be warning or critical")
	}
	if rule.Threshold < 0 {
		return fmt.Errorf("the threshold can't be negative")
	}
	if (rule.Kind == "disk" || rule.Kind == "inodes" || rule.Kind == "memory" || rule.Kind == "swap") && rule.Threshold > 100 {
		return fmt.Errorf("a percentage can't be over 100")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		rule.Name = k.Label
	}
	if rule.ServerID != "" {
		if _, ok := h.store.Get(ctx, rule.ServerID); !ok {
			return fmt.Errorf("that server isn't in the list")
		}
	}
	return nil
}

func (h *Handler) handleAddRule(w http.ResponseWriter, r *http.Request) {
	var rule Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.validRule(r.Context(), &rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.AddRule(r.Context(), &rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rule, http.StatusCreated)
}

func (h *Handler) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	current, ok := h.store.Rule(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}
	var rule Rule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	// What a rule measures is fixed; everything else can change.
	rule.ID, rule.Kind, rule.Builtin, rule.CreatedAt = current.ID, current.Kind, current.Builtin, current.CreatedAt
	if err := h.validRule(r.Context(), &rule); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateRule(r.Context(), rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rule, 0)
}

func (h *Handler) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := h.store.Rule(r.Context(), r.PathValue("id"))
	if !ok {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}
	if rule.Builtin {
		http.Error(w, "a built-in rule can't be deleted; switch it off instead", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteRule(r.Context(), rule.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
