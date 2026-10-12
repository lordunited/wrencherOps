// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package sshexec

import (
	"io"
	"strings"
	"testing"
)

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	if r == nil {
		return "<nil>"
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The SSH password goes to sudo on stdin's first line, and only to commands
// that read it — anything else gets exactly its own stdin, so the password
// can't leak into a command that would echo or store what it reads.
func TestStdinForSendsThePasswordOnlyToPreludeCommands(t *testing.T) {
	c := &Client{password: "s3cret pass"}

	if got := readAll(t, c.stdinFor(AsRoot("docker ps"), nil)); got != "s3cret pass\n" {
		t.Errorf("prelude command stdin = %q", got)
	}
	if got := readAll(t, c.stdinFor(AsRootPrelude+"$AS_ROOT tee /x", strings.NewReader("file body\n"))); got != "s3cret pass\nfile body\n" {
		t.Errorf("prelude command with data = %q, want the password line then the data", got)
	}
	// A prelude chained with && after trimming its final "; " still reads stdin.
	if got := readAll(t, c.stdinFor(strings.TrimSuffix(AsRootPrelude, "; ")+" && true", nil)); got != "s3cret pass\n" {
		t.Errorf("trimmed prelude stdin = %q", got)
	}
	if got := c.stdinFor("kubectl get pods", nil); got != nil {
		t.Errorf("a command without the prelude got stdin %q", readAll(t, got))
	}
	if got := readAll(t, c.stdinFor("cat", strings.NewReader("only mine"))); got != "only mine" {
		t.Errorf("a command without the prelude got %q, want only its own data", got)
	}

	// No password: still one (empty) line, so the prelude never eats data.
	if got := readAll(t, (&Client{}).stdinFor(AsRootPrelude+"$AS_ROOT tee /x", strings.NewReader("body"))); got != "\nbody" {
		t.Errorf("no-password stdin = %q", got)
	}
}
