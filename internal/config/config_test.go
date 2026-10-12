// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package config

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// A valid key, made up for the test (32 zero bytes).
var testKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func setRequired(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"GITLAB_TOKEN":              "test-token",
		"AUTH_USERS":                "admin:$2a$10$hash",
		"DATABASE_URL":              "postgres://u:p@localhost/db",
		"CREDENTIAL_ENCRYPTION_KEY": testKey,
		"PORT":                      "", "GITLAB_BASE_URL": "", "CACHE_TTL_SECONDS": "", "SESSION_TTL_HOURS": "", "COOKIE_SECURE": "",
	} {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" || cfg.GitLabBaseURL != "https://gitlab.com" || cfg.CacheTTL != 30*time.Second ||
		cfg.SessionTTL != 24*time.Hour || cfg.CookieSecure || len(cfg.CredentialKey) != 32 {
		t.Errorf("defaults: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "9000")
	t.Setenv("CACHE_TTL_SECONDS", "5")
	t.Setenv("SESSION_TTL_HOURS", "2")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("GITLAB_BASE_URL", "https://git.example.com")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9000" || cfg.CacheTTL != 5*time.Second || cfg.SessionTTL != 2*time.Hour || !cfg.CookieSecure || cfg.GitLabBaseURL != "https://git.example.com" {
		t.Errorf("overrides: %+v", cfg)
	}
}

func TestLoadRefusesMissingOrBadSettings(t *testing.T) {
	cases := map[string]struct{ key, value, want string }{
		"no token":     {"GITLAB_TOKEN", "", "GITLAB_TOKEN"},
		"no users":     {"AUTH_USERS", "", "AUTH_USERS"},
		"no database":  {"DATABASE_URL", "", "DATABASE_URL"},
		"no key":       {"CREDENTIAL_ENCRYPTION_KEY", "", "CREDENTIAL_ENCRYPTION_KEY"},
		"short key":    {"CREDENTIAL_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("short")), "32 bytes"},
		"bad cache":    {"CACHE_TTL_SECONDS", "soon", "CACHE_TTL_SECONDS"},
		"bad sessions": {"SESSION_TTL_HOURS", "1.5", "SESSION_TTL_HOURS"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(c.key, c.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want one mentioning %s", err, c.want)
			}
		})
	}
}
