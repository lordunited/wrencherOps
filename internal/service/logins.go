// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"devops-wrapper/internal/sshexec"
)

// SSH passwords are saved per server login — host, SSH port and user — not
// per service: every service on that login shares one saved password, so a
// server's password is entered once. A password is saved only after it has
// logged in; one the server refuses is never saved, and never replaces the
// one that works.

// isAuthError reports whether err is the server refusing the credentials,
// as opposed to not being reachable at all.
func isAuthError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unable to authenticate")
}

// rememberLogin saves password, which just logged in, for every service on
// that login. A failure is logged, not returned: the action it came from
// worked, and the password will be offered again next time.
func (h *Handler) rememberLogin(ctx context.Context, host string, port int, username, password string) {
	enc, err := h.secrets.Encrypt(password)
	if err != nil {
		log.Printf("saving the SSH password for %s@%s: %v", username, host, err)
		return
	}
	if err := h.services.SetLoginPassword(ctx, host, port, username, enc); err != nil {
		log.Printf("saving the SSH password for %s@%s: %v", username, host, err)
	}
}

// savedLogin returns the saved password for a login, or "" if there's none.
func (h *Handler) savedLogin(ctx context.Context, host string, port int, username string) (string, error) {
	enc, err := h.services.LoginPassword(ctx, host, port, username)
	if err != nil || enc == "" {
		return "", err
	}
	return h.secrets.Decrypt(enc)
}

// verifyAndRemember logs in with a password just typed. If it works, it's
// saved for the whole login. If the server refuses it, that's an error and
// nothing is saved. If the server can't be reached, it can't be verified:
// it's returned for this one action and not saved.
func (h *Handler) verifyAndRemember(ctx context.Context, host string, port int, username, password string) error {
	client, err := sshexec.Dial(sshexec.Target{Host: host, Port: port, Username: username, Password: password})
	if isAuthError(err) {
		return fmt.Errorf("%s@%s refused that SSH password; it wasn't saved, and the saved one (if any) is kept", username, host)
	}
	if err != nil {
		return nil
	}
	client.Close()
	h.rememberLogin(ctx, host, port, username, password)
	return nil
}

// dialTarget connects to a server given by address and login: with the
// password typed (saved for the login once it works), or, when none was
// typed, with the one saved for that login.
func (h *Handler) dialTarget(ctx context.Context, t *connectTarget) (*sshexec.Client, error) {
	if t.Port == 0 {
		t.Port = 22
	}
	t.Host = strings.TrimSpace(t.Host)
	t.Username = strings.TrimSpace(t.Username)
	if t.Host == "" || t.Username == "" {
		return nil, fmt.Errorf("enter the server's address and SSH user")
	}
	usedSaved := false
	if t.Password == "" {
		pw, err := h.savedLogin(ctx, t.Host, t.Port, t.Username)
		if err != nil {
			return nil, err
		}
		if pw == "" {
			return nil, fmt.Errorf("no SSH password is saved for %s@%s yet; enter it once and it's saved", t.Username, t.Host)
		}
		t.Password, usedSaved = pw, true
	}
	client, err := sshexec.Dial(sshexec.Target{Host: t.Host, Port: t.Port, Username: t.Username, Password: t.Password})
	if err != nil {
		if usedSaved && isAuthError(err) {
			return nil, fmt.Errorf("%s@%s refused the saved SSH password (was it changed?); enter the current one", t.Username, t.Host)
		}
		return nil, fmt.Errorf("SSH connection failed: %v", err)
	}
	if !usedSaved {
		h.rememberLogin(ctx, t.Host, t.Port, t.Username, t.Password)
	}
	return client, nil
}
