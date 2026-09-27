package service

import (
	"context"
	"fmt"
	"log"
	"strings"
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
func (h *Handler) recordHealth(ctx context.Context, id, health, detail string) (*Service, error) {
	var unlockedNow bool
	now := time.Now()
	svc, err := h.services.Update(ctx, id, func(s *Service) {
		unlockedNow = advanceStreak(s, health, detail, now)
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
func advanceStreak(s *Service, health, detail string, now time.Time) (unlockedNow bool) {
	previous := s.LastSyncedAt
	s.Health = health
	s.HealthDetail = detail
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

// StartMonitor watches connected stacks whose controls are still locked,
// checking each one every monitorInterval until it unlocks. It returns
// immediately; the watching stops when ctx is canceled.
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

func (h *Handler) monitorOnce(ctx context.Context) {
	services, err := h.services.List(ctx)
	if err != nil {
		log.Printf("monitor: %v", err)
		return
	}
	for _, svc := range services {
		if !svc.Connected() || svc.ControlsUnlockedAt != nil || svc.EncryptedPassword == "" {
			continue
		}
		password, err := h.secrets.Decrypt(svc.EncryptedPassword)
		if err != nil {
			continue
		}
		target := sshexec.Target{Host: svc.Host, Port: svc.Port, Username: svc.Username, Password: password}
		if svc.ComposeFiles == "" {
			h.backfillCompose(ctx, target, svc)
		}
		health, detail := CheckHealth(target, svc)
		if _, err := h.recordHealth(ctx, svc.ID, health, detail); err != nil {
			log.Printf("monitor: %s: %v", svc.ID, err)
		}
	}
}

// backfillCompose fills in the compose project and files of a stack
// connected before they were recorded, from its container's labels.
func (h *Handler) backfillCompose(ctx context.Context, target sshexec.Target, svc *Service) {
	client, err := sshexec.Dial(target)
	if err != nil {
		return
	}
	defer client.Close()
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
