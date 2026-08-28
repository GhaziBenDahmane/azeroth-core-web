package store

import (
	"strings"
	"testing"

	"github.com/example/azeroth-portal/internal/config"
)

func TestBootstrapAccountRejectsUnsafeInputBeforeConnecting(t *testing.T) {
	c := config.Config{AuthDSN: "invalid", AuthDB: "acore_auth"}
	tests := []struct {
		name, username, password, email string
		level, realm                    int
		want                            string
	}{
		{"username", "BAD USER", "securepass", "portal@example.invalid", 3, -1, "username"},
		{"password", "PORTAL", "short", "portal@example.invalid", 3, -1, "password"},
		{"email", "PORTAL", "securepass", "invalid", 3, -1, "email"},
		{"level", "PORTAL", "securepass", "portal@example.invalid", 4, -1, "GM level"},
		{"realm", "PORTAL", "securepass", "portal@example.invalid", 3, -2, "realm ID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := BootstrapAccount(c, tt.username, tt.password, tt.email, tt.level, tt.realm)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}
