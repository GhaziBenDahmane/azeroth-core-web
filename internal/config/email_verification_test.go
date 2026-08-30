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

func TestTOTPEncryptionKeyValidation(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "http://portal.example.com")
	t.Setenv("TOTP_ENCRYPTION_KEY", "not-a-key")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOTP_ENCRYPTION_KEY") {
		t.Fatalf("Load() error = %v, want encryption key validation error", err)
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", "BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc")
	cfg, err := Load()
	if err != nil || len(cfg.TOTPEncryptionKey) != 32 {
		t.Fatalf("valid encryption key loaded as %d bytes, err = %v", len(cfg.TOTPEncryptionKey), err)
	}
}

func TestRealmAgentRequiresSecureURLAndStrongToken(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "http://portal.example.com")
	t.Setenv("REALM_AGENT_URL", "http://agent.internal:9000")
	t.Setenv("REALM_AGENT_TOKEN", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "REALM_AGENT_URL") {
		t.Fatalf("Load() error = %v, want secure agent URL validation error", err)
	}
	t.Setenv("REALM_AGENT_URL", "http://127.0.0.1:9000")
	t.Setenv("REALM_AGENT_TOKEN", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "REALM_AGENT_TOKEN") {
		t.Fatalf("Load() error = %v, want agent token validation error", err)
	}
	t.Setenv("REALM_AGENT_TOKEN", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() rejected loopback development agent: %v", err)
	}
}

func TestDiscordOAuthRequiresPairedCredentialsAndExactRedirect(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "https://portal.example.com")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("DISCORD_CLIENT_ID", "client-id")
	t.Setenv("DISCORD_CLIENT_SECRET", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("Load() error = %v, want paired credential validation", err)
	}
	t.Setenv("DISCORD_CLIENT_SECRET", "client-secret")
	t.Setenv("DISCORD_REDIRECT_URL", "https://evil.example.com/api/auth/discord/callback")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DISCORD_REDIRECT_URL") {
		t.Fatalf("Load() error = %v, want exact redirect validation", err)
	}
	t.Setenv("DISCORD_REDIRECT_URL", "")
	cfg, err := Load()
	if err != nil || cfg.DiscordRedirectURL != "https://portal.example.com/api/auth/discord/callback" {
		t.Fatalf("derived redirect = %q, err = %v", cfg.DiscordRedirectURL, err)
	}
}

func TestGoogleOAuthRequiresPairedCredentialsAndExactRedirect(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "https://portal.example.com")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("Load() error = %v, want paired Google credential validation", err)
	}
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "https://evil.example.com/api/auth/google/callback")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "GOOGLE_REDIRECT_URL") {
		t.Fatalf("Load() error = %v, want exact Google redirect validation", err)
	}
	t.Setenv("GOOGLE_REDIRECT_URL", "")
	cfg, err := Load()
	if err != nil || cfg.GoogleRedirectURL != "https://portal.example.com/api/auth/google/callback" {
		t.Fatalf("derived Google redirect = %q, err = %v", cfg.GoogleRedirectURL, err)
	}
}

func TestAnalyticsConfigurationRequiresHTTPSPair(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "http://portal.example.com")
	t.Setenv("ANALYTICS_SCRIPT_URL", "http://analytics.example.com/script.js")
	t.Setenv("ANALYTICS_DOMAIN", "portal.example.com")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "analytics configuration") {
		t.Fatalf("Load() error = %v, want HTTPS analytics validation", err)
	}
	t.Setenv("ANALYTICS_SCRIPT_URL", "https://analytics.example.com/script.js")
	if _, err := Load(); err != nil {
		t.Fatalf("Load() rejected valid analytics configuration: %v", err)
	}
}
