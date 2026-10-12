// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package randid

import (
	"encoding/base64"
	"regexp"
	"testing"
)

func TestNew(t *testing.T) {
	urlSafe := regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := New(16)
		if err != nil {
			t.Fatal(err)
		}
		if !urlSafe.MatchString(id) {
			t.Fatalf("%q isn't URL-safe", id)
		}
		if b, err := base64.RawURLEncoding.DecodeString(id); err != nil || len(b) != 16 {
			t.Fatalf("%q doesn't decode to 16 bytes", id)
		}
		if seen[id] {
			t.Fatalf("%q came up twice", id)
		}
		seen[id] = true
	}
	if id, _ := New(32); len(id) != 43 {
		t.Errorf("32 bytes encode to %d characters, want 43", len(id))
	}
}
