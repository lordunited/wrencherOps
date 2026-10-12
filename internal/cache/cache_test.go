// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchCachesUntilExpiry(t *testing.T) {
	c := New(50 * time.Millisecond)
	calls := 0
	fn := func() (int, error) { calls++; return calls, nil }
	if v, _ := Fetch(c, "k", fn); v != 1 {
		t.Fatalf("first fetch = %d", v)
	}
	if v, _ := Fetch(c, "k", fn); v != 1 || calls != 1 {
		t.Fatalf("second fetch = %d after %d calls; want the cached 1", v, calls)
	}
	if v, _ := Fetch(c, "other", fn); v != 2 {
		t.Fatalf("another key = %d", v)
	}
	time.Sleep(60 * time.Millisecond)
	if v, _ := Fetch(c, "k", fn); v != 3 {
		t.Fatalf("after expiry = %d, want a fresh 3", v)
	}
}

func TestFetchDoesNotCacheErrors(t *testing.T) {
	c := New(time.Minute)
	boom := errors.New("boom")
	if _, err := Fetch(c, "k", func() (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	v, err := Fetch(c, "k", func() (string, error) { return "ok", nil })
	if err != nil || v != "ok" {
		t.Fatalf("after an error the next fetch runs: %q, %v", v, err)
	}
}

// Concurrent fetches of one key run fn once.
func TestFetchCoalesces(t *testing.T) {
	c := New(time.Minute)
	var calls atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Fetch(c, "k", func() (int, error) {
				calls.Add(1)
				<-release
				return 7, nil
			})
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Errorf("fn ran %d times, want 1", n)
	}
}
