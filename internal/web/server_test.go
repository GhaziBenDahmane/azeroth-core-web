package web

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
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
	s := &Server{c: config.Config{MockMode: true, EnableRealmStatus: true, EnableGuilds: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
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

func TestPublicBrandingConfig(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, PortalName: "Frosthold", RealmName: "Frosthold One", RealmAddress: "logon.frosthold.test", BrandMark: "FH", ExpansionName: "Wrath", ClientVersion: "3.3.5a"}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	r := httptest.NewRequest("GET", "/api/public-config", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var body map[string]any
	if w.Code != 200 || json.NewDecoder(w.Body).Decode(&body) != nil {
		t.Fatalf("public config returned %d: %s", w.Code, w.Body.String())
	}
	if body["portalName"] != "Frosthold" || body["realmAddress"] != "logon.frosthold.test" || body["brandMark"] != "FH" {
		t.Fatalf("unexpected public branding config: %#v", body)
	}
	if _, exposed := body["realmControlToken"]; exposed {
		t.Fatal("private realm control token was exposed")
	}
}

func TestDisabledFeatureReturnsNotFound(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableArmory: false}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	r := httptest.NewRequest("GET", "/api/armory", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled armory returned %d, want 404", w.Code)
	}
}

func TestTrustedProxyClientIP(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.2:4321"
	r.Header.Set("X-Forwarded-For", "203.0.113.8, 10.0.0.2")

	untrusted := &Server{c: config.Config{TrustProxy: false}}
	if got := untrusted.clientIP(r); got != "10.0.0.2" {
		t.Fatalf("untrusted proxy address = %q", got)
	}
	trusted := &Server{c: config.Config{TrustProxy: true}}
	if got := trusted.clientIP(r); got != "203.0.113.8" {
		t.Fatalf("trusted proxy address = %q", got)
	}
}

func TestSecureResponsesIncludeHSTS(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, CookieSecure: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatal("secure response did not include HSTS")
	}
}

func TestMockFirstRunSetupLocksAfterCreation(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableSetup: true, EnableRegistration: true, SetupToken: "0123456789abcdef", SetupGMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()

	status := httptest.NewRecorder()
	h.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/setup/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"required":true`) {
		t.Fatalf("unexpected initial setup status: %d %s", status.Code, status.Body.String())
	}
	blockedRegistration := httptest.NewRecorder()
	h.ServeHTTP(blockedRegistration, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"PLAYER","password":"securepass","email":"player@example.com"}`)))
	if blockedRegistration.Code != http.StatusServiceUnavailable {
		t.Fatalf("registration before setup returned %d, want 503", blockedRegistration.Code)
	}

	body := `{"token":"0123456789abcdef","username":"OWNER","password":"securepass","email":"owner@example.com"}`
	created := httptest.NewRecorder()
	h.ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(body)))
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"gmLevel":3`) {
		t.Fatalf("setup failed: %d %s", created.Code, created.Body.String())
	}

	repeated := httptest.NewRecorder()
	h.ServeHTTP(repeated, httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader(body)))
	if repeated.Code != http.StatusConflict {
		t.Fatalf("repeated setup returned %d, want 409", repeated.Code)
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

func TestGMConsoleCommandPolicy(t *testing.T) {
	command, ok := normalizeConsoleCommand(".server info")
	if !ok || command != "server info" {
		t.Fatalf("normalization returned %q, %v", command, ok)
	}
	allowed := []string{"server info", "lookup"}
	if !consoleCommandAllowed("server info", false, allowed) || !consoleCommandAllowed("lookup item frostmourne", false, allowed) {
		t.Fatal("allowed console command was rejected")
	}
	if consoleCommandAllowed("server shutdown 10", false, allowed) || consoleCommandAllowed("lookupanything", false, allowed) {
		t.Fatal("disallowed console command was accepted")
	}
	if _, ok := normalizeConsoleCommand("server info\nserver shutdown 10"); ok {
		t.Fatal("multiline console command was accepted")
	}
	if got := auditConsoleCommand("account set password PLAYER secret secret"); got != "account set password [arguments redacted]" {
		t.Fatalf("sensitive command was not redacted: %q", got)
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

func TestStartRealmWebhook(t *testing.T) {
	called := false
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Errorf("got method %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer control-secret" {
			t.Errorf("got authorization %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("got content type %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer hook.Close()

	s := &Server{c: config.Config{RealmStartWebhook: hook.URL, RealmControlToken: "control-secret"}}
	if err := s.startRealm(httptest.NewRequest(http.MethodPost, "/api/admin/moderation", nil)); err != nil {
		t.Fatalf("start webhook failed: %v", err)
	}
	if !called {
		t.Fatal("start webhook was not called")
	}
}
