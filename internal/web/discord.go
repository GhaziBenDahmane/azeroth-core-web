package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// notifyDiscord is deliberately best-effort: external notification delivery
// must never make account, shop, support, or moderation transactions fail.
func (s *Server) notifyDiscord(title, description string) {
	if s.c.DiscordWebhookURL == "" {
		return
	}
	if len(title) > 256 {
		title = title[:256]
	}
	if len(description) > 4000 {
		description = description[:4000]
	}
	payload := map[string]any{
		"username":         s.c.PortalName,
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title": title, "description": description, "color": 0x3b82f6,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"footer":    map[string]string{"text": s.c.RealmName},
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 6 * time.Second}
	response, err := client.Post(s.c.DiscordWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		slog.Warn("discord notification failed", "error", err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		slog.Warn("discord notification rejected", "status", response.Status)
	}
}

func (s *Server) notifyDiscordAsync(title, format string, args ...any) {
	if s.c.DiscordWebhookURL == "" {
		return
	}
	description := fmt.Sprintf(format, args...)
	go s.notifyDiscord(title, description)
}
