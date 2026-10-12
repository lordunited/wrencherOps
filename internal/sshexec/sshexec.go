// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package sshexec runs commands and writes files on a remote host over SSH.
// Used by the deploy feature to configure a target server. Passwords are
// used only for the duration of one connection and never persisted or logged.
package sshexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Target struct {
	Host     string
	Port     int
	Username string
	Password string
}

type Client struct {
	conn *ssh.Client
	// password is also offered to sudo: see AsRootPrelude.
	password string
}

// Dial connects to the target over SSH with password authentication.
//
// Host key verification is intentionally skipped (InsecureIgnoreHostKey):
// this tool connects to whatever server the operator names at request time,
// with no prior known_hosts entry to check against, so there is no
// trust-on-first-use record to verify against here. That means a
// man-in-the-middle on the network path to the target could intercept the
// session. Acceptable for a trusted internal network; worth hardening
// (pinned host keys) before pointing this at anything over an untrusted link.
func Dial(target Target) (*Client, error) {
	// Stray whitespace from a form would otherwise make the address
	// undialable ("lookup  10.0.0.1: no such host").
	target.Host = strings.TrimSpace(target.Host)
	target.Username = strings.TrimSpace(target.Username)
	config := &ssh.ClientConfig{
		User:            target.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(target.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", target.Host, target.Port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, password: target.Password}, nil
}

// stdinFor is the stdin a command gets. A command using AsRootPrelude
// reads its first line as the password to offer sudo, so it always gets
// one — the SSH password, or an empty line — followed by in. The password
// travels only on stdin: never in the command line, which any user on the
// server could see in ps.
func (c *Client) stdinFor(cmd string, in io.Reader) io.Reader {
	if !strings.Contains(cmd, passwordRead) {
		return in
	}
	line := strings.NewReader(c.password + "\n")
	if in == nil {
		return line
	}
	return io.MultiReader(line, in)
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// commandTimeout bounds how long a single remote command may run. Without
// this, a command that hangs — a network partition mid-command, one
// blocked waiting on stdin, a stuck pull producing no output — would block
// its goroutine forever, since the ssh package gives a Session no deadline
// of its own. Generous enough to never interfere with a real multi-GB
// image pull; anything genuinely stuck this long should fail loudly
// instead of leaving a job stuck at "running" forever with no way to tell.
const commandTimeout = 10 * time.Minute

// runSession runs cmd in session and enforces commandTimeout. On timeout it
// closes session, which is what actually unblocks the Run call in the
// other goroutine — verified against the ssh package's source: Close
// closes the underlying channel that Wait blocks reading from. done is
// buffered so that goroutine can always complete its send and exit even
// after this function has already returned on timeout.
func runSession(session *ssh.Session, cmd string) error {
	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	timer := time.NewTimer(commandTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		session.Close()
		return fmt.Errorf("command timed out after %s", commandTimeout)
	}
}

// Run executes a command, streaming combined stdout/stderr to out.
func (c *Client) Run(cmd string, out io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = c.stdinFor(cmd, nil)
	session.Stdout = out
	session.Stderr = out
	return runSession(session, cmd)
}

// RunWithInput executes a command with in as its stdin, streaming combined
// stdout/stderr to out.
func (c *Client) RunWithInput(cmd string, in io.Reader, out io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = c.stdinFor(cmd, in)
	session.Stdout = out
	session.Stderr = out
	return runSession(session, cmd)
}

// Output executes a command and returns its stdout and stderr separately,
// for commands whose output is parsed (JSON, file contents) and must not
// be mixed with warnings on stderr.
func (c *Client) Output(cmd string) (stdout, stderr string, err error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", "", err
	}
	defer session.Close()

	var out, errOut strings.Builder
	session.Stdin = c.stdinFor(cmd, nil)
	session.Stdout = &out
	session.Stderr = &errOut
	err = runSession(session, cmd)
	return out.String(), errOut.String(), err
}

// Alive reports whether the connection still answers, for callers that
// keep a Client open between requests.
func (c *Client) Alive() bool {
	_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

// HTTPGet makes an HTTP GET request to addr (host:port as the server sees
// it, e.g. a container's IP) through this SSH connection, like "ssh -L".
// It returns the status and body whatever the status, so callers can read
// error details from 4xx responses. The body is capped at 20 MB.
func (c *Client) HTTPGet(ctx context.Context, addr, pathAndQuery string) (int, []byte, error) {
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return c.conn.Dial("tcp", addr)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+pathAndQuery, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	return resp.StatusCode, body, err
}

// WriteFile writes content to remotePath, running as root if the target
// directory needs it (see AsRootPrelude) — the panel's own deploy
// directories (/opt/gitlab and the like) are typically root-owned even
// when the configured SSH user isn't root.
//
// The content travels as the command's stdin rather than inside the
// command line: the remote shell gets the whole command as one argument,
// and Linux caps a single argument at 128 KiB, which an embedded file
// would hit long before any real size limit. stdin has no such cap, and
// the content never touches the shell, so it needs no escaping either.
func (c *Client) WriteFile(remotePath string, content []byte, out io.Writer) error {
	cmd := AsRootPrelude + fmt.Sprintf("$AS_ROOT mkdir -p %s && $AS_ROOT tee %s > /dev/null",
		ShellQuote(path.Dir(remotePath)), ShellQuote(remotePath))
	return c.RunWithInput(cmd, bytes.NewReader(content), out)
}

// ShellQuote wraps s in single quotes for safe use as one POSIX shell
// argument, escaping any single quotes it contains.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// AsRootPrelude, prepended to a shell command, defines $AS_ROOT. Prefix any
// sub-command that needs root — the Docker socket, or a root-owned path —
// with "$AS_ROOT ". Every panel uses it, so a server works the same whether
// the SSH user is root, has passwordless sudo, or has sudo that asks for a
// password (the SSH password is offered):
//
//   - root: $AS_ROOT is empty.
//   - passwordless sudo: "sudo -n".
//   - sudo with a password: "sudo -A", with an askpass helper that prints
//     the password from an environment variable. The helper file holds no
//     secret and is removed when the command ends; stdin stays free, so data
//     piped into a sudo command (how files are written) still arrives.
//   - sudo without -A (e.g. some sudo-rs builds): the password is checked
//     once with "sudo -S -v", then "sudo -n" rides sudo's cached login.
//
// The password arrives as the first line of stdin (Client.stdinFor sends it
// to every command containing this prelude), so it's never in a command
// line — ps on the server never shows it — and never on disk. The block
// runs once even if a command contains the prelude twice. Every branch
// above was verified against real sudo 1.9 with a password-protected
// account: data piped in and out of sudo, command substitution, a wrong
// password ("sudo didn't accept the SSH password"), passwordless sudo, root,
// and the password absent from every process's command line.
const AsRootPrelude = `if [ -z "$PANEL_ROOT" ]; then PANEL_ROOT=1; ` + passwordRead + ` || PANEL_PW=""; ` +
	`if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; ` +
	`elif sudo -n true 2>/dev/null; then AS_ROOT="sudo -n"; ` +
	`elif [ -n "$PANEL_PW" ] && PANEL_ASKPASS=$(mktemp "${HOME:-/tmp}/.panel-askpass.XXXXXX" 2>/dev/null) && trap 'rm -f "$PANEL_ASKPASS"' EXIT && ` +
	`printf '#!/bin/sh\nprintf "%%s\\n" "$PANEL_PW"\n' > "$PANEL_ASKPASS" && chmod 700 "$PANEL_ASKPASS" && ` +
	`export PANEL_PW SUDO_ASKPASS="$PANEL_ASKPASS" && sudo -A true 2>/dev/null; then AS_ROOT="sudo -A"; ` +
	`elif [ -n "$PANEL_PW" ] && printf '%s\n' "$PANEL_PW" | sudo -S -p '' -v 2>/dev/null && sudo -n true 2>/dev/null; then AS_ROOT="sudo -n"; ` +
	`else AS_ROOT="sudo -n"; [ -n "$PANEL_PW" ] && echo "sudo didn't accept the SSH password" >&2; fi; fi; `

// passwordRead is the part of AsRootPrelude that reads stdin's first line.
// stdinFor keys on it rather than the whole prelude, so a command that uses
// the prelude trimmed or rearranged still gets its password line.
const passwordRead = `IFS= read -r PANEL_PW`

// AsRoot wraps a single, self-contained command with AsRootPrelude, for
// the common case of one command with no surrounding shell logic (a bare
// "cd a && b" needs AsRootPrelude inlined by hand instead, since only b —
// not the "cd" builtin — should run through sudo).
func AsRoot(cmd string) string {
	return AsRootPrelude + "$AS_ROOT " + cmd
}

// ComposeIn is "docker compose" for the project in dir, through $AS_ROOT
// (callers prepend AsRootPrelude), ready for a subcommand. The folder is
// given to compose rather than cd'd into: the SSH user may not be allowed
// into it (a project under another user's home, or /root), while compose
// runs as root and can read it.
func ComposeIn(dir string) string {
	return "$AS_ROOT docker compose --project-directory " + ShellQuote(dir)
}
