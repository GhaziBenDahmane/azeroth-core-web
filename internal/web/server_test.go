package web

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/example/azeroth-portal/internal/config"
)

func TestSPAHandlerServesRouteIndexes(t *testing.T) {
	root := fstest.MapFS{
		"index.html":        {Data: []byte("home")},
		"armory/index.html": {Data: []byte("armory")},
		"app.js":            {Data: []byte("script")},
	}
	h := spaHandler(fs.FS(root))
	for route, want := range map[string]string{"/": "home", "/armory": "armory", "/armory/": "armory", "/app.js": "script"} {
		r := httptest.NewRequest("GET", route, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != want {
			t.Errorf("%s: got %d %q, want 200 %q", route, w.Code, w.Body.String(), want)
		}
	}
}

func TestStripeSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	payload := []byte(`{"id":"evt_test"}`)
	mac := hmac.New(sha256.New, []byte("whsec_test"))
	_, _ = fmt.Fprintf(mac, "%d.", now.Unix())
	_, _ = mac.Write(payload)
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil)))
	if !verifyStripeSignature(payload, header, "whsec_test", now) {
		t.Fatal("valid signature rejected")
	}
	if verifyStripeSignature([]byte("changed"), header, "whsec_test", now) {
		t.Fatal("tampered payload accepted")
	}
	if verifyStripeSignature(payload, header, "whsec_test", now.Add(6*time.Minute)) {
		t.Fatal("stale signature accepted")
	}
}

func TestTOTPWindow(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	now := time.Unix(1_700_000_000, 0)
	raw, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(now.Unix()/30))
	mac := hmac.New(sha1.New, raw)
	_, _ = mac.Write(buf)
	sum := mac.Sum(nil)
	i := sum[len(sum)-1] & 15
	value := (uint32(sum[i])&127)<<24 | uint32(sum[i+1])<<16 | uint32(sum[i+2])<<8 | uint32(sum[i+3])
	code := fmt.Sprintf("%06d", value%1_000_000)
	if !validTOTP(secret, code, now) {
		t.Fatal("valid TOTP rejected")
	}
	if validTOTP(secret, "000000", now) && code != "000000" {
		t.Fatal("invalid TOTP accepted")
	}
}

func TestMockCommunityAndOperations(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	for _, path := range []string{"/api/realm", "/api/guilds", "/api/guilds/1", "/healthz", "/readyz", "/metrics"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Origin", "")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Errorf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
}

func TestWotLKRaceFactions(t *testing.T) {
	for _, race := range []uint8{1, 3, 4, 7, 11} {
		if !isAllianceRace(race) || isHordeRace(race) {
			t.Errorf("race %d should be Alliance only", race)
		}
	}
	for _, race := range []uint8{2, 5, 6, 8, 10} {
		if !isHordeRace(race) || isAllianceRace(race) {
			t.Errorf("race %d should be Horde only", race)
		}
	}
	if isAllianceRace(0) || isHordeRace(0) {
		t.Fatal("unknown race must not be assigned a faction")
	}
}

func TestServiceCommandsAreAllowListed(t *testing.T) {
	for action, want := range map[string]string{
		"":               "",
		"race_change":    "character changerace Arthoria",
		"faction_change": "character changefaction Arthoria",
	} {
		got, err := serviceCommand(action, "Arthoria")
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", action, got, err, want)
		}
	}
	if _, err := serviceCommand("server shutdown", "Arthoria"); err == nil {
		t.Fatal("arbitrary service command was accepted")
	}
}

func TestModerationInputAllowLists(t *testing.T) {
	for _, duration := range []string{"30m", "7d", "1w", "1d12h", "-1"} {
		if !banDurationPattern.MatchString(duration) {
			t.Errorf("valid duration %q rejected", duration)
		}
	}
	for _, duration := range []string{"", "forever", "-2", "7 days", "7d\nserver shutdown"} {
		if banDurationPattern.MatchString(duration) {
			t.Errorf("unsafe duration %q accepted", duration)
		}
	}
	if !validAccountName("PLAYER123") || validAccountName("PLAYER;BAN") {
		t.Fatal("account name allow-list is incorrect")
	}
	if !validCharacterName("Arthoria") || validCharacterName("Arthoria kick") {
		t.Fatal("character name allow-list is incorrect")
	}
	if !validModerationReason("Repeated harassment") || validModerationReason("reason\nserver shutdown") || validModerationReason(`bad "quote"`) {
		t.Fatal("moderation reason allow-list is incorrect")
	}
}
