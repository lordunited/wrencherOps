// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"devops-wrapper/internal/auth"
)

// TestMain runs main itself when asked to, so a test can run the command.
func TestMain(m *testing.M) {
	if os.Getenv("RUN_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func run(args ...string) (string, error) {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "RUN_MAIN=1")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func TestHashpasswordPrintsAWorkingHash(t *testing.T) {
	hash, err := run("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse battery")) != nil {
		t.Fatalf("the hash doesn't match its password: %s", hash)
	}
	if users, err := auth.ParseUsers("admin:" + hash); err != nil || users["admin"] != hash {
		t.Errorf("the hash doesn't make an AUTH_USERS entry: %v", err)
	}
}

func TestHashpasswordNeedsExactlyOneArgument(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		if _, err := run(args...); err == nil {
			t.Errorf("args %v: want a failure", args)
		}
	}
}
