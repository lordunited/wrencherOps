// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"devops-wrapper/internal/secretbox"
)

// TestMain runs main itself when asked to, so a test can run the command.
func TestMain(m *testing.M) {
	if os.Getenv("RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGenkeyPrintsAUsableKey(t *testing.T) {
	keys := map[string]bool{}
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "RUN_MAIN=1")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		key := strings.TrimSpace(string(out))
		if _, err := secretbox.ParseKey(key); err != nil {
			t.Fatalf("genkey printed a key the panel refuses: %v", err)
		}
		keys[key] = true
	}
	if len(keys) != 2 {
		t.Error("two runs printed the same key")
	}
}
