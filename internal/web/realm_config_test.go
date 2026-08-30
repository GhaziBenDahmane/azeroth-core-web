package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/example/azeroth-portal/internal/config"
)

func TestDesiredRealmValuesAndDrift(t *testing.T) {
	settings := siteSettings{
		QuestExperienceRate: "2×", KillExperienceRate: "3x", ExplorationExperienceRate: "1.5",
		DropRate: "1×", ReputationRate: "2×", HonorRate: "1×", ProfessionRate: "4×",
		CrossFactionGroups: boolPtr(true), CrossFactionGuilds: boolPtr(false),
	}
	desired, err := desiredRealmValues(settings)
	if err != nil {
		t.Fatal(err)
	}
	if desired["rate.xp.quest"] != float64(2) || desired["rate.xp.explore"] != 1.5 || desired["cross_faction.groups"] != true {
		t.Fatalf("unexpected desired values: %#v", desired)
	}
	observed := make(map[string]any, len(desired))
	for key, value := range desired {
		observed[key] = value
	}
	observed["rate.xp.quest"] = float64(1)
	items := realmConfigItems(desired, &realmAgentSnapshot{Values: observed, RestartRequired: []string{"rate.xp.quest"}})
	drifted := 0
	for _, item := range items {
		if item.State == "drifted" {
			drifted++
			if item.Key != "rate.xp.quest" || !item.RestartRequired {
				t.Fatalf("unexpected drift item: %#v", item)
			}
		}
	}
	if drifted != 1 {
		t.Fatalf("drifted settings = %d, want 1", drifted)
	}
	if _, err := displayedRate("fast"); err == nil {
		t.Fatal("non-numeric rate accepted for managed configuration")
	}
}

func TestRealmAgentUsesAuthenticatedAllowListedContract(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef"
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/config/apply" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("X-Portal-Realm") != "frost" {
			t.Fatalf("agent authentication headers missing")
		}
		var payload struct {
			RealmKey string         `json:"realmKey"`
			Values   map[string]any `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.RealmKey != "frost" || payload.Values["rate.xp.quest"] != float64(2) {
			t.Fatalf("unexpected payload: %#v, err=%v", payload, err)
		}
		_ = json.NewEncoder(w).Encode(realmAgentSnapshot{Version: "test/1", Values: payload.Values, BackupID: "backup-1"})
	}))
	defer agent.Close()
	s := &Server{c: config.Config{RealmKey: "frost", RealmAgentURL: agent.URL, RealmAgentToken: token}}
	snapshot, err := s.callRealmAgent(context.Background(), http.MethodPost, "/v1/config/apply", map[string]any{"rate.xp.quest": float64(2)})
	if err != nil || snapshot.BackupID != "backup-1" {
		t.Fatalf("callRealmAgent() snapshot=%#v, err=%v", snapshot, err)
	}
}

func TestParseItemEnhancements(t *testing.T) {
	parsed := parseItemEnhancements("3817 0 0 0 0 0 3637 0 0 3525 0 0 0 0 0")
	if len(parsed) != 3 || parsed[0].Kind != "enchant" || parsed[0].EnchantmentID != 3817 || parsed[1].Kind != "gem" || parsed[2].Slot != 3 {
		t.Fatalf("unexpected item enhancements: %#v", parsed)
	}
	if parsed := parseItemEnhancements("invalid short"); len(parsed) != 0 {
		t.Fatalf("invalid enhancement encoding parsed as %#v", parsed)
	}
}
