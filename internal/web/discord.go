package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
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
	response, err := (&http.Client{Timeout: 6 * time.Second}).Post(s.c.DiscordWebhookURL, "application/json", bytes.NewReader(body))
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

var discordSnowflakePattern = regexp.MustCompile(`^[0-9]{16,22}$`)

type discordWidget struct {
	Name          string `json:"name"`
	InstantInvite string `json:"instant_invite"`
	PresenceCount int    `json:"presence_count"`
	Members       []struct {
		Username  string `json:"username"`
		AvatarURL string `json:"avatar_url"`
		Status    string `json:"status"`
	} `json:"members"`
}

func (s *Server) discordStatus(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"configured": true, "name": s.c.RealmName + " Community", "online": 128, "invite": s.c.CommunityURL, "members": []map[string]any{{"username": "Arthoria", "status": "online"}, {"username": "Grimward", "status": "dnd"}, {"username": "Thornhoof", "status": "idle"}}})
		return
	}
	if !discordSnowflakePattern.MatchString(s.c.DiscordGuildID) {
		jsonOut(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://discord.com/api/guilds/"+s.c.DiscordGuildID+"/widget.json", nil)
	if err != nil {
		problem(w, http.StatusBadGateway, "Discord status is unavailable")
		return
	}
	response, err := (&http.Client{Timeout: 4 * time.Second}).Do(req)
	if err != nil {
		problem(w, http.StatusBadGateway, "Discord status is temporarily unavailable")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		problem(w, http.StatusBadGateway, fmt.Sprintf("Discord widget returned %s", response.Status))
		return
	}
	var widget discordWidget
	if json.NewDecoder(http.MaxBytesReader(w, response.Body, 256<<10)).Decode(&widget) != nil {
		problem(w, http.StatusBadGateway, "Discord returned an invalid widget")
		return
	}
	if len(widget.Members) > 20 {
		widget.Members = widget.Members[:20]
	}
	jsonOut(w, http.StatusOK, map[string]any{"configured": true, "name": widget.Name, "online": widget.PresenceCount, "invite": widget.InstantInvite, "members": widget.Members})
}
