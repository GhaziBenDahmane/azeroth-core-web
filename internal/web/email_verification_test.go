package web

import "testing"

func TestValidEmailAddress(t *testing.T) {
	for _, value := range []string{"PLAYER@EXAMPLE.COM", "player+shop@example.co.uk"} {
		if !validEmailAddress(value) {
			t.Errorf("expected %q to be accepted", value)
		}
	}
	for _, value := range []string{"", "player", "Player <player@example.com>", "player@example.com\r\nBcc: victim@example.com"} {
		if validEmailAddress(value) {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}
