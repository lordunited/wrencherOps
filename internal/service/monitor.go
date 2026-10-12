// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"devops-wrapper/internal/promctl"
	"devops-wrapper/internal/sshexec"
)

// A connected stack's Start/Stop/Down stay locked until the panel has
// watched it stay healthy for ControlsUnlockAfter without a break. The
// monitor below does the watching, so nobody has to keep clicking Sync.

const (
	monitorInterval = 30 * time.Second
	// maxCheckGap is the longest gap between checks that still counts as
	// continuous observation. After a longer gap (the panel was down, say)
	// the streak restarts, because the stack wasn't actually watched.
	maxCheckGap = 90 * time.Second
)

// recordHealth stores a health check result and advances or resets the
// service's healthy streak, unlocking a connected stack's controls when the
// streak first reaches ControlsUnlockAfter.
func (h *Handler) recordHealth(ctx context.Context, id, health, detail string, obs Observed) (*Service, error) {
	var unlockedNow bool
	now := time.Now()
	svc, err := h.services.Update(ctx, id, func(s *Service) {
		unlockedNow = advanceStreak(s, health, detail, obs, now)
	})
	if err == nil && unlockedNow {
		h.recordAction(ctx, svc, "unlock", "success",
			fmt.Sprintf("Every part of the stack has been healthy for %s without a break (%s).\nStart, Stop and Down are now available.\n",
				ControlsUnlockAfter, detail), now)
	}
	return svc, err
}

// advanceStreak applies one health check result to s at time now and
// reports whether it just unlocked the stack's controls.
func advanceStreak(s *Service, health, detail string, obs Observed, now time.Time) (unlockedNow bool) {
	previous := s.LastSyncedAt
	s.Health = health
	s.HealthDetail = detail
	s.HealthStats = obs.Stats
	s.HealthPorts = obs.Ports
	s.HealthContainers = obs.Containers
	s.LastSyncedAt = &now

	if health != "healthy" {
		s.HealthySince = nil
		return false
	}
	if s.HealthySince == nil || previous == nil || now.Sub(*previous) > maxCheckGap {
		s.HealthySince = &now
	}
	if s.Connected() && s.ControlsUnlockedAt == nil && now.Sub(*s.HealthySince) >= ControlsUnlockAfter {
		s.ControlsUnlockedAt = &now
		return true
	}
	return false
}

// monitorParallel is how many servers the monitor checks at once.
const monitorParallel = 4

// StartMonitor checks the health of every service with a saved SSH
// password every monitorInterval, and saves each result, so every list and
// page shows a current health rather than "unknown" or whatever the last
// manual Sync found. It also advances connected stacks' healthy streaks
// toward unlocking their controls. It returns immediately; the watching
// stops when ctx is canceled.
func (h *Handler) StartMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(monitorInterval)
		defer ticker.Stop()
		for {
			h.monitorOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// serverLogin is one SSH login to a server; every service on it is checked
// over one connection.
type serverLogin struct {
	host     string
	port     int
	username string
	password string
}

func (h *Handler) monitorOnce(ctx context.Context) {
	services, err := h.services.List(ctx)
	if err != nil {
		log.Printf("monitor: %v", err)
		return
	}
	// One connection per server login rather than per service: most servers
	// run several stacks, and each SSH handshake costs CPU on both ends.
	byLogin := map[serverLogin][]*Service{}
	for _, svc := range services {
		if svc.EncryptedPassword == "" || inTransition(svc.Status) {
			continue
		}
		password, err := h.secrets.Decrypt(svc.EncryptedPassword)
		if err != nil {
			continue
		}
		key := serverLogin{strings.TrimSpace(svc.Host), svc.Port, strings.TrimSpace(svc.Username), password}
		byLogin[key] = append(byLogin[key], svc)
	}
	sem := make(chan struct{}, monitorParallel)
	var wg sync.WaitGroup
	for login, list := range byLogin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			h.checkServer(ctx, login, list)
		}()
	}
	wg.Wait()
}

// inTransition reports a service mid-job: its health is checked when the
// job ends (watchJob), not while it's changing.
func inTransition(s Status) bool {
	switch s {
	case StatusDeploying, StatusStarting, StatusStopping, StatusRemoving:
		return true
	}
	return false
}

// checkSoon checks a service's health in the background, a moment from now
// (so whatever just created or changed it has finished saving).
func (h *Handler) checkSoon(id string) {
	go func() {
		time.Sleep(2 * time.Second)
		ctx := context.Background()
		svc, ok := h.services.Get(ctx, id)
		if !ok || svc.EncryptedPassword == "" {
			return
		}
		h.checkAndRecord(ctx, svc)
	}()
}

// checkAndRecord runs one health check of svc with its saved password and
// saves the result — unless a check of it is already running.
func (h *Handler) checkAndRecord(ctx context.Context, svc *Service) {
	password, err := h.secrets.Decrypt(svc.EncryptedPassword)
	if err != nil {
		return
	}
	h.checkServer(ctx, serverLogin{strings.TrimSpace(svc.Host), svc.Port, strings.TrimSpace(svc.Username), password}, []*Service{svc})
}

// sessionsPerServer is how many services on one server are checked at
// once over its single connection (OpenSSH allows 10 sessions on one by
// default).
const sessionsPerServer = 4

// checkServer checks every service in list — all on one server login — over
// a single SSH connection, a few at a time, saving each result. A service
// whose check is already running elsewhere is skipped. If the server refuses
// the saved password, another one saved on the same login is tried (and kept
// if it works); if it can't be reached at all, every service records why.
func (h *Handler) checkServer(ctx context.Context, login serverLogin, list []*Service) {
	target := sshexec.Target{Host: login.host, Port: login.port, Username: login.username, Password: login.password}
	client, err := sshexec.Dial(target)
	if isAuthError(err) {
		if pw := h.workingLoginPassword(ctx, list[0], login.password); pw != "" {
			target.Password = pw
			client, err = sshexec.Dial(target)
		}
	}
	if client != nil {
		defer client.Close()
	}

	sem := make(chan struct{}, sessionsPerServer)
	var wg sync.WaitGroup
	for _, svc := range list {
		if _, busy := h.checking.LoadOrStore(svc.ID, true); busy {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer h.checking.Delete(svc.ID)
			sem <- struct{}{}
			defer func() { <-sem }()
			var health, detail string
			var obs Observed
			if err != nil {
				health, detail = "unreachable", "SSH connection failed: "+err.Error()
			} else {
				if svc.Connected() && svc.ComposeFiles == "" && svc.Container != "" {
					h.backfillCompose(ctx, client, svc)
				}
				health, detail, obs = CheckHealthOn(client, target.Host, svc)
			}
			if _, rerr := h.recordHealth(ctx, svc.ID, health, detail, obs); rerr != nil {
				log.Printf("monitor: %s: %v", svc.ID, rerr)
			}
		}()
	}
	wg.Wait()
}

// backfillCompose fills in the compose project and files of a stack
// connected before they were recorded, from its container's labels.
func (h *Handler) backfillCompose(ctx context.Context, client *sshexec.Client, svc *Service) {
	inst, err := promctl.DiscoverContainer(client, svc.Container)
	if err != nil {
		return
	}
	files := inst.Labels["com.docker.compose.project.config_files"]
	project := inst.Labels["com.docker.compose.project"]
	if files == "" && project == "" {
		return
	}
	h.services.Update(ctx, svc.ID, func(s *Service) {
		s.ComposeFiles = strings.TrimSpace(files)
		if s.ComposeProject == "" {
			s.ComposeProject = project
		}
	})
}

// workingLoginPassword tries the other passwords saved on svc's server
// login (besides refused) and returns the first that logs in, after saving
// it for the whole login; "" if none does.
func (h *Handler) workingLoginPassword(ctx context.Context, svc *Service, refused string) string {
	encs, err := h.services.LoginPasswords(ctx, svc.Host, svc.Port, svc.Username)
	if err != nil {
		return ""
	}
	for _, enc := range encs {
		pw, err := h.secrets.Decrypt(enc)
		if err != nil || pw == refused {
			continue
		}
		client, err := sshexec.Dial(sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: pw})
		if err != nil {
			continue
		}
		client.Close()
		if err := h.services.SetLoginPassword(ctx, svc.Host, svc.Port, svc.Username, enc); err != nil {
			log.Printf("monitor: saving %s@%s's password: %v", svc.Username, svc.Host, err)
		}
		return pw
	}
	return ""
}
