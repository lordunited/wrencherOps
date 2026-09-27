package service

import (
	"testing"
	"time"
)

func TestAdvanceStreakUnlocksAfterFiveHealthyMinutes(t *testing.T) {
	s := &Service{Origin: OriginConnected}
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	for i := 0; i <= 9; i++ { // checks every 30s for 4m30s
		if advanceStreak(s, "healthy", "ok", start.Add(time.Duration(i)*30*time.Second)) {
			t.Fatalf("unlocked early at check %d", i)
		}
	}
	if s.ControlsUnlocked() {
		t.Fatal("unlocked before 5 minutes")
	}
	if !advanceStreak(s, "healthy", "ok", start.Add(5*time.Minute)) || !s.ControlsUnlocked() {
		t.Fatal("did not unlock at 5 minutes")
	}
	// Once unlocked, a later failure doesn't lock it again.
	advanceStreak(s, "degraded", "x", start.Add(6*time.Minute))
	if !s.ControlsUnlocked() || s.HealthySince != nil {
		t.Error("failure after unlocking should reset the streak but keep controls unlocked")
	}
}

func TestAdvanceStreakResetsOnFailureAndOnGaps(t *testing.T) {
	start := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	s := &Service{Origin: OriginConnected}
	advanceStreak(s, "healthy", "ok", start)
	advanceStreak(s, "healthy", "ok", start.Add(4*time.Minute+30*time.Second))
	advanceStreak(s, "unreachable", "down", start.Add(4*time.Minute+45*time.Second))
	if s.HealthySince != nil {
		t.Fatal("failure didn't reset the streak")
	}
	if advanceStreak(s, "healthy", "ok", start.Add(5*time.Minute)) || s.ControlsUnlocked() {
		t.Fatal("unlocked right after a failure")
	}

	// A long gap between checks (panel was down) restarts the count.
	g := &Service{Origin: OriginConnected}
	advanceStreak(g, "healthy", "ok", start)
	if advanceStreak(g, "healthy", "ok", start.Add(10*time.Minute)) {
		t.Fatal("unlocked across an unobserved 10-minute gap")
	}
	if !g.HealthySince.Equal(start.Add(10 * time.Minute)) {
		t.Errorf("streak should restart after a gap, started %v", g.HealthySince)
	}
}

func TestPanelStacksAreAlwaysUnlocked(t *testing.T) {
	s := &Service{Origin: OriginPanel}
	if !s.ControlsUnlocked() {
		t.Error("a panel-deployed stack should never be locked")
	}
	if advanceStreak(s, "healthy", "ok", time.Now()) {
		t.Error("panel stacks don't unlock; they're never locked")
	}
}
