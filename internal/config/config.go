// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"devops-wrapper/internal/secretbox"
)

type Config struct {
	Port          string
	GitLabBaseURL string
	GitLabToken   string
	CacheTTL      time.Duration
	AuthUsersRaw  string
	SessionTTL    time.Duration
	CookieSecure  bool
	DatabaseURL   string
	CredentialKey []byte
}

func Load() (Config, error) {
	cacheTTLSeconds, err := strconv.Atoi(envOrDefault("CACHE_TTL_SECONDS", "30"))
	if err != nil {
		return Config{}, fmt.Errorf("CACHE_TTL_SECONDS must be an integer: %w", err)
	}

	sessionTTLHours, err := strconv.Atoi(envOrDefault("SESSION_TTL_HOURS", "24"))
	if err != nil {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be an integer: %w", err)
	}

	cfg := Config{
		Port:          envOrDefault("PORT", "8080"),
		GitLabBaseURL: envOrDefault("GITLAB_BASE_URL", "https://gitlab.com"),
		GitLabToken:   os.Getenv("GITLAB_TOKEN"),
		CacheTTL:      time.Duration(cacheTTLSeconds) * time.Second,
		AuthUsersRaw:  os.Getenv("AUTH_USERS"),
		SessionTTL:    time.Duration(sessionTTLHours) * time.Hour,
		CookieSecure:  envOrDefault("COOKIE_SECURE", "false") == "true",
		DatabaseURL:   os.Getenv("DATABASE_URL"),
	}

	if cfg.GitLabToken == "" {
		return Config{}, fmt.Errorf("GITLAB_TOKEN environment variable is required")
	}
	if cfg.AuthUsersRaw == "" {
		return Config{}, fmt.Errorf("AUTH_USERS environment variable is required (see cmd/hashpassword to generate entries)")
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL environment variable is required, e.g. postgres://user:pass@localhost:5432/dbname")
	}

	credentialKeyRaw := os.Getenv("CREDENTIAL_ENCRYPTION_KEY")
	if credentialKeyRaw == "" {
		return Config{}, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY environment variable is required (generate one with: go run ./cmd/genkey)")
	}
	cfg.CredentialKey, err = secretbox.ParseKey(credentialKeyRaw)
	if err != nil {
		return Config{}, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY: %w", err)
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
