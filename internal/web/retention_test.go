package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSanctionExpiry(t *testing.T) {
	start := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		duration string
		want     time.Duration
		nilValue bool
	}{{"30m", 30 * time.Minute, false}, {"1w2d", 9 * 24 * time.Hour, false}, {"-1", 0, true}, {"", 0, true}, {"bad", 0, true}}
	for _, test := range tests {
		expires := sanctionExpiry(test.duration, start)
		if test.nilValue {
			if expires != nil {
				t.Fatalf("%q returned %v", test.duration, expires)
			}
			continue
		}
		if expires == nil || expires.Sub(start) != test.want {
			t.Fatalf("%q expiry = %v, want %v", test.duration, expires, test.want)
		}
	}
}

func TestLoyaltyLevels(t *testing.T) {
	tests := []struct {
		points    uint32
		name      string
		remaining uint32
	}{{0, "Initiate", 30}, {30, "Adventurer", 70}, {249, "Veteran", 1}, {250, "Champion", 250}, {500, "Legend", 0}}
	for _, test := range tests {
		level := loyaltyForPoints(test.points)
		if level.Name != test.name || level.Remaining != test.remaining {
			t.Fatalf("loyaltyForPoints(%d) = %s/%d, want %s/%d", test.points, level.Name, level.Remaining, test.name, test.remaining)
		}
	}
}

func TestPlayerMissionValidationConstrainsProgressSources(t *testing.T) {
	valid := playerMission{Slug: "raid-vanguard", Name: "Raid vanguard", Description: "Verified monthly kills", Category: "pve", Metric: "raid_kills", Target: 3, RewardCredits: 15}
	if err := validatePlayerMission(valid); err != nil {
		t.Fatalf("valid mission rejected: %v", err)
	}
	invalid := valid
	invalid.Metric = "sql_expression"
	if err := validatePlayerMission(invalid); err == nil {
		t.Fatal("unsupported progress source accepted")
	}
	invalid = valid
	invalid.Category = "pvp"
	if err := validatePlayerMission(invalid); err == nil {
		t.Fatal("mismatched mission category accepted")
	}
}

func TestVoteCampaignValidationRequiresCompleteCommunityGoal(t *testing.T) {
	start := time.Now().Add(time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	valid := voteCampaignInput{Name: "September draw", PrizeDescription: "Spectral Tiger", MinimumVotes: 2, WinnerCount: 3, TargetEntries: 250, CommunityRewardDescription: "Bonus XP weekend"}
	if err := validateVoteCampaignInput(&valid, start, end); err != nil {
		t.Fatalf("valid voting campaign rejected: %v", err)
	}
	valid.CommunityRewardDescription = ""
	if err := validateVoteCampaignInput(&valid, start, end); err == nil {
		t.Fatal("campaign target without a community reward was accepted")
	}
	valid.TargetEntries = 0
	valid.CommunityRewardDescription = "Bonus XP weekend"
	if err := validateVoteCampaignInput(&valid, start, end); err == nil {
		t.Fatal("community reward without a target was accepted")
	}
}

func TestGuildRecruitmentDiscordInviteValidation(t *testing.T) {
	valid := guildRecruitment{GuildID: 1, Headline: "Raid with us", Description: "A complete recruitment description for our guild.", DiscordURL: "https://discord.gg/realm"}
	if err := validateGuildRecruitment(&valid); err != nil {
		t.Fatalf("valid Discord invite rejected: %v", err)
	}
	for _, candidate := range []string{"http://discord.gg/realm", "https://discord.gg.evil.test/realm", "javascript:alert(1)"} {
		invalid := valid
		invalid.DiscordURL = candidate
		if err := validateGuildRecruitment(&invalid); err == nil {
			t.Fatalf("unsafe Discord invite accepted: %s", candidate)
		}
	}
}

func TestMockMissionClaimIsIdempotent(t *testing.T) {
	s := &Server{mock: newMockState()}
	s.c.MockMode = true
	request := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/rewards/missions/2/claim", nil)
		req.SetPathValue("id", "2")
		req.AddCookie(&http.Cookie{Name: "portal_demo", Value: "DEMO"})
		return req
	}
	first := httptest.NewRecorder()
	s.claimPlayerMission(first, request())
	if first.Code != http.StatusOK {
		t.Fatalf("first claim status = %d: %s", first.Code, first.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &body); err != nil || body["credits"] != float64(10) {
		t.Fatalf("first claim body = %s, err = %v", first.Body.String(), err)
	}
	second := httptest.NewRecorder()
	s.claimPlayerMission(second, request())
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate claim status = %d, want 409", second.Code)
	}
}

func TestDiscordSnowflakeValidation(t *testing.T) {
	if !discordSnowflakePattern.MatchString("123456789012345678") {
		t.Fatal("valid Discord snowflake rejected")
	}
	for _, value := range []string{"", "123", "123456789012345x78", "12345678901234567890123"} {
		if discordSnowflakePattern.MatchString(value) {
			t.Fatalf("invalid Discord snowflake accepted: %q", value)
		}
	}
}
