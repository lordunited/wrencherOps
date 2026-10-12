// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package randid generates URL-safe random identifiers, used anywhere this
// backend needs an unguessable ID: session tokens, job IDs, service IDs.
package randid

import (
	"crypto/rand"
	"encoding/base64"
)

func New(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
