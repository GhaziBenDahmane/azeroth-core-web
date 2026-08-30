package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// realmConfigSpec is deliberately a closed list. The portal never sends a
// filename, arbitrary AzerothCore key, or shell command to the realm agent.
var realmConfigSpec = []struct {
	Key   string
	Label string
}{
	{"rate.xp.quest", "Quest experience"},
	{"rate.xp.kill", "Kill experience"},
	{"rate.xp.explore", "Exploration experience"},
	{"rate.drop.item", "Item drops"},
	{"rate.reputation", "Reputation"},
	{"rate.honor", "Honor"},
	{"rate.profession", "Profession skill"},
	{"cross_faction.accounts", "Characters of both factions per account"},
	{"cross_faction.calendar", "Shared calendar"},
	{"cross_faction.channels", "Shared channels"},
	{"cross_faction.groups", "Cross-faction groups"},
	{"cross_faction.guilds", "Cross-faction guilds"},
	{"cross_faction.auctions", "Shared auction house"},
	{"cross_faction.mail", "Cross-faction mail"},
	{"cross_faction.who", "Cross-faction /who"},
	{"cross_faction.friends", "Cross-faction friends"},
	{"cross_faction.trade", "Cross-faction trade"},
}

type realmAgentSnapshot struct {
	Version         string         `json:"version"`
	Values          map[string]any `json:"values"`
	RestartRequired []string       `json:"restartRequired,omitempty"`
	BackupID        string         `json:"backupId,omitempty"`
	ObservedAt      *time.Time     `json:"observedAt,omitempty"`
}

type realmConfigItem struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Desired         any    `json:"desired"`
	Observed        any    `json:"observed,omitempty"`
	State           string `json:"state"`
	RestartRequired bool   `json:"restartRequired"`
}

func displayedRate(value string) (float64, error) {
	normalized := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "×"), "x"))
	rate, err := strconv.ParseFloat(normalized, 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 1000 {
		return 0, fmt.Errorf("%q is not a numeric rate such as 1x or 2×", value)
	}
	return rate, nil
}

func desiredRealmValues(settings siteSettings) (map[string]any, error) {
	values := map[string]any{}
	for key, value := range map[string]string{
		"rate.xp.quest": settings.QuestExperienceRate, "rate.xp.kill": settings.KillExperienceRate,
		"rate.xp.explore": settings.ExplorationExperienceRate, "rate.drop.item": settings.DropRate,
		"rate.reputation": settings.ReputationRate, "rate.honor": settings.HonorRate,
		"rate.profession": settings.ProfessionRate,
	} {
		rate, err := displayedRate(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		values[key] = rate
	}
	for key, value := range map[string]*bool{
		"cross_faction.accounts": settings.CrossFactionAccounts, "cross_faction.calendar": settings.CrossFactionCalendar,
		"cross_faction.channels": settings.CrossFactionChannels, "cross_faction.groups": settings.CrossFactionGroups,
		"cross_faction.guilds": settings.CrossFactionGuilds, "cross_faction.auctions": settings.CrossFactionAuctions,
		"cross_faction.mail": settings.CrossFactionMail, "cross_faction.who": settings.CrossFactionWho,
		"cross_faction.friends": settings.CrossFactionFriends, "cross_faction.trade": settings.CrossFactionTrade,
	} {
		values[key] = settingBool(value, false)
	}
	return values, nil
}

func sameRealmValue(desired, observed any) bool {
	switch wanted := desired.(type) {
	case float64:
		actual, ok := observed.(float64)
		return ok && math.Abs(wanted-actual) < 0.000001
	case bool:
		actual, ok := observed.(bool)
		return ok && wanted == actual
	default:
		return false
	}
}

func realmConfigItems(desired map[string]any, observed *realmAgentSnapshot) []realmConfigItem {
	restart := map[string]bool{}
	if observed != nil {
		for _, key := range observed.RestartRequired {
			restart[key] = true
		}
	}
	items := make([]realmConfigItem, 0, len(realmConfigSpec))
	for _, spec := range realmConfigSpec {
		item := realmConfigItem{Key: spec.Key, Label: spec.Label, Desired: desired[spec.Key], State: "unavailable"}
		if observed != nil {
			item.Observed = observed.Values[spec.Key]
			item.RestartRequired = restart[spec.Key]
			if _, found := observed.Values[spec.Key]; !found {
				item.State = "unknown"
			} else if sameRealmValue(item.Desired, item.Observed) {
				item.State = "in_sync"
			} else {
				item.State = "drifted"
			}
		}
		items = append(items, item)
	}
	return items
}

func (s *Server) callRealmAgent(ctx context.Context, method, route string, values map[string]any) (*realmAgentSnapshot, error) {
	if s.c.RealmAgentURL == "" {
		return nil, fmt.Errorf("realm configuration agent is not configured")
	}
	var body io.Reader
	if values != nil {
		encoded, err := json.Marshal(map[string]any{"realmKey": s.c.RealmKey, "values": values})
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, s.c.RealmAgentURL+route, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.c.RealmAgentToken)
	req.Header.Set("X-Portal-Realm", s.c.RealmKey)
	if values != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 7 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
		return nil, fmt.Errorf("agent returned %s", response.Status)
	}
	var snapshot realmAgentSnapshot
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1024*1024))
	if err := decoder.Decode(&snapshot); err != nil || snapshot.Values == nil {
		return nil, fmt.Errorf("agent returned an invalid configuration snapshot")
	}
	return &snapshot, nil
}

func (s *Server) adminRealmConfig(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "realm")
	if !ok {
		problem(w, http.StatusForbidden, "Realm operator access required")
		return
	}
	desired, err := desiredRealmValues(s.runtimeSettings(r))
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Displayed realm profile cannot be applied: "+err.Error())
		return
	}
	if s.c.MockMode {
		now := time.Now().UTC()
		snapshot := &realmAgentSnapshot{Version: "mock-agent/1", Values: desired, ObservedAt: &now}
		jsonOut(w, http.StatusOK, map[string]any{"configured": true, "mode": "mock", "items": realmConfigItems(desired, snapshot), "snapshot": snapshot})
		return
	}
	if s.c.RealmAgentURL == "" {
		jsonOut(w, http.StatusOK, map[string]any{"configured": false, "mode": "metadata_only", "items": realmConfigItems(desired, nil), "message": "Displayed values do not modify worldserver.conf. Configure REALM_AGENT_URL and REALM_AGENT_TOKEN to enable managed apply and drift detection."})
		return
	}
	route, method := "/v1/config", http.MethodGet
	if r.Method == http.MethodPost {
		route, method = "/v1/config/apply", http.MethodPost
	}
	snapshot, err := s.callRealmAgent(r.Context(), method, route, map[bool]map[string]any{true: desired, false: nil}[r.Method == http.MethodPost])
	if err != nil {
		problem(w, http.StatusBadGateway, "Realm configuration agent is unavailable: "+err.Error())
		return
	}
	if r.Method == http.MethodPost {
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'realm.config.apply',?,?)", actor.ID, s.c.RealmKey, "Applied allow-listed realm configuration; backup="+snapshot.BackupID)
	}
	jsonOut(w, http.StatusOK, map[string]any{"configured": true, "mode": "managed", "items": realmConfigItems(desired, snapshot), "snapshot": snapshot})
}
