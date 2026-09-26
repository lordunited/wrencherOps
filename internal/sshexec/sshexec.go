// Package sshexec runs commands and writes files on a remote host over SSH.
// Used by the deploy feature to configure a target server. Passwords are
// used only for the duration of one connection and never persisted or logged.
package sshexec

import (
	"encoding/base64"
	"fmt"
	"io"
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
	return &Client{conn: conn}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// Run executes a command, streaming combined stdout/stderr to out.
func (c *Client) Run(cmd string, out io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdout = out
	session.Stderr = out
	return session.Run(cmd)
}

// WriteFile uploads content to a remote path. Content is base64-encoded
// before being embedded in the remote command (its alphabet has no shell
// metacharacters, so it needs no quoting) and decoded on the far side,
// which avoids both a separate SFTP dependency and injection from the
// file's own content.
func (c *Client) WriteFile(remotePath string, content []byte, out io.Writer) error {
	encoded := base64.StdEncoding.EncodeToString(content)
	dir := path.Dir(remotePath)
	cmd := fmt.Sprintf("mkdir -p %s && echo %s | base64 -d > %s",
		ShellQuote(dir), encoded, ShellQuote(remotePath))
	return c.Run(cmd, out)
}

// ShellQuote wraps s in single quotes for safe use as one POSIX shell
// argument, escaping any single quotes it contains.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
