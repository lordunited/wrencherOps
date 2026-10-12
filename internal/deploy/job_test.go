// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"testing"
	"time"
)

func TestStoreSweepEvictsOnlyOldFinishedJobs(t *testing.T) {
	s := NewStore()

	oldButRunning := s.create()
	oldButRunning.updatedAt = time.Now().Add(-jobRetention - time.Minute)

	oldFinished := s.create()
	oldFinished.setStatus(StatusSuccess)
	oldFinished.updatedAt = time.Now().Add(-jobRetention - time.Minute)

	recentFinished := s.create()
	recentFinished.setStatus(StatusFailed)

	s.create() // sweep runs on every create()

	if _, ok := s.Get(oldButRunning.id); !ok {
		t.Error("a running job must never be evicted, however old — only its finished state ages out")
	}
	if _, ok := s.Get(oldFinished.id); ok {
		t.Error("a finished job past jobRetention should have been evicted")
	}
	if _, ok := s.Get(recentFinished.id); !ok {
		t.Error("a recently finished job must not be evicted yet")
	}
}
