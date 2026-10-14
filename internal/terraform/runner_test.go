// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"testing"
	"time"
)

// A run that ends frees its workspace before it's reported done, so the
// next run can start right away; the ending run must not then free the
// workspace again from under that next run.
func TestRunnerReleaseKeepsTheNextRunsHold(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no tofu: every run fails at once, nothing is executed
	r := NewRunner()
	const ws = "ws-1"
	noLog := func(string) {}
	holdB := make(chan struct{})
	bDone := make(chan struct{})
	defer func() {
		close(holdB)
		<-bDone
	}()

	err := r.Start(Job{Run: &Run{ID: "run-a", WorkspaceID: ws, Kind: "plan"}}, noLog,
		func(_, _, _ string, _ Result, release func()) {
			release()
			err := r.Start(Job{Run: &Run{ID: "run-b", WorkspaceID: ws, Kind: "plan"}}, noLog,
				func(_, _, _ string, _ Result, release func()) {
					<-holdB
					release()
					close(bDone)
				})
			if err != nil {
				t.Errorf("starting the next run after release: %v", err)
			}
		})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, running := r.cancels.Load("run-a"); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run-a didn't end")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// run-a's goroutine is past its last step or about to be: give it time
	// to run what remains, then run-b must still hold the workspace.
	time.Sleep(100 * time.Millisecond)
	if id, busy := r.Busy(ws); !busy || id != "run-b" {
		t.Errorf("Busy = %q, %v; want run-b still holding the workspace", id, busy)
	}
	if err := r.Start(Job{Run: &Run{ID: "run-c", WorkspaceID: ws, Kind: "plan"}}, noLog,
		func(string, string, string, Result, func()) {}); err == nil {
		t.Error("a third run started while run-b still holds the workspace")
	}
}
