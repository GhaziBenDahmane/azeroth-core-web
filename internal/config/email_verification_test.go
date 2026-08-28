package config

import (
	"strings"
	"testing"
)

func TestEmailVerificationRequiresSMTPConfiguration(t *testing.T) {
	t.Setenv("AUTH_DSN", "portal:secret@tcp(database:3306)/acore_auth")
	t.Setenv("PUBLIC_URL", "http://portal.example.com")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("ENABLE_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "true")
	t.Setenv("SMTP_ADDR", "")
	t.Setenv("SMTP_FROM", "")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_ADDR") {
		t.Fatalf("Load() error = %v, want SMTP_ADDR validation error", err)
	}

	t.Setenv("SMTP_ADDR", "smtp.example.com:587")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() with SMTP configuration failed: %v", err)
	}
}

func TestEmailVerificationCanRemainDisabled(t *testing.T) {
	t.Setenv("AUTH_DSN", "portal:secret@tcp(database:3306)/acore_auth")
	t.Setenv("PUBLIC_URL", "http://portal.example.com")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("SMTP_ADDR", "")
	t.Setenv("SMTP_FROM", "")

	if _, err := Load(); err != nil {
		t.Fatalf("Load() with optional verification failed: %v", err)
	}
}
