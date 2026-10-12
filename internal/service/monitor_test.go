// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"testing"
	"time"

	"devops-wrapper/internal/dockerx"
)

func TestAdvanceStreakUnlocksAfterFiveHealthyMinutes(t *testing.T) {
	s := &Service{Origin: OriginConnected}
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for i := 0; i <= 9; i++ { // checks every 30s for 4m30s
		if advanceStreak(s, "healthy", "ok", Observed{}, start.Add(time.Duration(i)*30*time.Second)) {
			t.Fatalf("unlocked early at check %d", i)
		}
	}
	if s.ControlsUnlocked() {
		t.Fatal("unlocked before 5 minutes")
	}
	if !advanceStreak(s, "healthy", "ok", Observed{}, start.Add(5*time.Minute)) || !s.ControlsUnlocked() {
		t.Fatal("did not unlock at 5 minutes")
	}
	// Once unlocked, a later failure doesn't lock it again.
	advanceStreak(s, "degraded", "x", Observed{}, start.Add(6*time.Minute))
	if !s.ControlsUnlocked() || s.HealthySince != nil {
		t.Error("failure after unlocking should reset the streak but keep controls unlocked")
	}
}

func TestAdvanceStreakResetsOnFailureAndOnGaps(t *testing.T) {
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	s := &Service{Origin: OriginConnected}
	advanceStreak(s, "healthy", "ok", Observed{}, start)
	advanceStreak(s, "healthy", "ok", Observed{}, start.Add(4*time.Minute+30*time.Second))
	advanceStreak(s, "unreachable", "down", Observed{}, start.Add(4*time.Minute+45*time.Second))
	if s.HealthySince != nil {
		t.Fatal("failure didn't reset the streak")
	}
	if advanceStreak(s, "healthy", "ok", Observed{}, start.Add(5*time.Minute)) || s.ControlsUnlocked() {
		t.Fatal("unlocked right after a failure")
	}

	// A long gap between checks (panel was down) restarts the count.
	g := &Service{Origin: OriginConnected}
	advanceStreak(g, "healthy", "ok", Observed{}, start)
	if advanceStreak(g, "healthy", "ok", Observed{}, start.Add(10*time.Minute)) {
		t.Fatal("unlocked across an unobserved 10-minute gap")
	}
	if !g.HealthySince.Equal(start.Add(10 * time.Minute)) {
		t.Errorf("streak should restart after a gap, started %v", g.HealthySince)
	}
}

func TestAdvanceStreakStoresLatestStats(t *testing.T) {
	s := &Service{Origin: OriginPanel}
	stats := []dockerx.ContainerStats{{Name: "app", CPU: "1.23%", Memory: "64MiB / 512MiB"}}
	ports := []PortStatus{{HostPort: "8080", ContainerPort: "80", Protocol: "tcp", State: "open"}}
	advanceStreak(s, "healthy", "ok", Observed{Stats: stats, Ports: ports}, time.Now())
	if len(s.HealthStats) != 1 || s.HealthStats[0].Name != "app" {
		t.Errorf("HealthStats = %+v, want the stats passed in", s.HealthStats)
	}
	if len(s.HealthPorts) != 1 || s.HealthPorts[0].State != "open" {
		t.Errorf("HealthPorts = %+v, want the ports passed in", s.HealthPorts)
	}
	// A check with no running containers (e.g. everything's down) clears
	// stale stats rather than leaving the last-seen reading in place.
	advanceStreak(s, "unreachable", "no containers found", Observed{}, time.Now())
	if s.HealthStats != nil || s.HealthPorts != nil {
		t.Errorf("stats and ports should be cleared when a check reports none, got %+v %+v", s.HealthStats, s.HealthPorts)
	}
}

func TestPanelStacksAreAlwaysUnlocked(t *testing.T) {
	s := &Service{Origin: OriginPanel}
	if !s.ControlsUnlocked() {
		t.Error("a panel-deployed stack should never be locked")
	}
	if advanceStreak(s, "healthy", "ok", Observed{}, time.Now()) {
		t.Error("panel stacks don't unlock; they're never locked")
	}
}

// A finished setup job doesn't count against a stack's health; a stopped
// long-running service does, however cleanly it exited.
func TestCompletedJob(t *testing.T) {
	for _, c := range []struct {
		status, exit, restart string
		awaited               bool
		want                  bool
	}{
		{"exited", "0", "on-failure", false, true}, // SigNoz's migrator, done
		{"exited", "0", "no", true, true},          // another service waits for it to complete
		{"exited", "0", "", true, true},
		{"exited", "0", "no", false, false},            // a plain service, "docker compose stop"ped
		{"exited", "0", "", false, false},              // the same, no restart policy at all
		{"exited", "1", "on-failure", false, false},    // a job that failed
		{"exited", "0", "unless-stopped", true, false}, // a service someone stopped
		{"exited", "0", "always", false, false},
		{"running", "0", "on-failure", false, false},
		{"restarting", "0", "on-failure", false, false},
	} {
		if got := completedJob(c.status, c.exit, c.restart, c.awaited); got != c.want {
			t.Errorf("completedJob(%q, %q, %q, awaited=%v) = %v, want %v", c.status, c.exit, c.restart, c.awaited, got, c.want)
		}
	}
}

func TestCleanStop(t *testing.T) {
	for reason, want := range map[string]bool{
		"": true, "exited with code 0": true, "exited with code 0; 2026/10/05 [notice] 1#1: gracefully shutting down": true,
		"exited with code 143": true, "exited with code 137; killed": true,
		"exited with code 1; panic: boom": false, "OOM-killed; exited with code 137": false, "crash-looping (last exit code 1)": false,
	} {
		if got := cleanStop(reason); got != want {
			t.Errorf("cleanStop(%q) = %v, want %v", reason, got, want)
		}
	}
}
