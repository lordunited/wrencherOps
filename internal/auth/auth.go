// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package auth handles panel login and session tracking. Who may do what
// once signed in is package access's.
package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"devops-wrapper/internal/randid"
)

// Users maps username to bcrypt password hash, loaded from AUTH_USERS:
// the built-in admin accounts.
type Users map[string]string

// ParseUsers parses the AUTH_USERS env format: "user1:hash1,user2:hash2".
// Hashes are generated with the hashpassword command.
func ParseUsers(raw string) (Users, error) {
	users := make(Users)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		username, hash, found := strings.Cut(entry, ":")
		if !found || username == "" || hash == "" {
			return nil, fmt.Errorf("invalid AUTH_USERS entry %q, expected username:bcryptHash", entry)
		}
		users[username] = hash
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("AUTH_USERS must contain at least one username:bcryptHash entry")
	}
	return users, nil
}

// SessionStore persists sessions in Postgres, so logins survive a backend
// restart instead of being silently invalidated by one.
type SessionStore struct {
	pool *pgxpool.Pool
	ttl  time.Duration
}

func NewSessionStore(pool *pgxpool.Pool, ttl time.Duration) *SessionStore {
	return &SessionStore{pool: pool, ttl: ttl}
}

func (s *SessionStore) Create(ctx context.Context, username string) (token string, expiresAt time.Time, err error) {
	token, err = randid.New(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(s.ttl)

	_, err = s.pool.Exec(ctx, "INSERT INTO sessions (token, username, expires_at) VALUES ($1, $2, $3)",
		token, username, expiresAt)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (s *SessionStore) Validate(ctx context.Context, token string) (username string, ok bool) {
	var expiresAt time.Time
	err := s.pool.QueryRow(ctx, "SELECT username, expires_at FROM sessions WHERE token = $1", token).
		Scan(&username, &expiresAt)
	if err != nil {
		return "", false
	}
	if time.Now().After(expiresAt) {
		s.pool.Exec(ctx, "DELETE FROM sessions WHERE token = $1", token)
		return "", false
	}
	return username, true
}

func (s *SessionStore) Delete(ctx context.Context, token string) {
	s.pool.Exec(ctx, "DELETE FROM sessions WHERE token = $1", token)
}
