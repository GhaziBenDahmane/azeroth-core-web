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
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/example/azeroth-portal/internal/config"
)

func TestSPAHandlerServesRouteIndexes(t *testing.T) {
	root := fstest.MapFS{
		"index.html":        {Data: []byte("home")},
		"admin/index.html":  {Data: []byte("admin")},
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
	w := httptest.NewRecorder()
	spaHandler(root).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/catalog/42/edit", nil))
	if w.Code != http.StatusOK || w.Body.String() != "admin" {
		t.Fatalf("nested admin route = %d %q", w.Code, w.Body.String())
	}
}

func TestMultiRealmRoutesAndRemembersSelection(t *testing.T) {
	h := MultiRealm("frost", true, map[string]http.Handler{
		"frost": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("frost")) }),
		"ember": http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ember")) }),
	})
	selected := httptest.NewRecorder()
	h.ServeHTTP(selected, httptest.NewRequest(http.MethodGet, "/api/realm?realm=ember", nil))
	if selected.Code != http.StatusOK || selected.Body.String() != "ember" || len(selected.Result().Cookies()) != 1 || !selected.Result().Cookies()[0].Secure {
		t.Fatalf("realm selection failed: %d %q %#v", selected.Code, selected.Body.String(), selected.Result().Cookies())
	}
	rememberedRequest := httptest.NewRequest(http.MethodGet, "/api/realm", nil)
	rememberedRequest.AddCookie(selected.Result().Cookies()[0])
	remembered := httptest.NewRecorder()
	h.ServeHTTP(remembered, rememberedRequest)
	if remembered.Body.String() != "ember" {
		t.Fatalf("remembered realm = %q; want ember", remembered.Body.String())
	}
	invalid := httptest.NewRecorder()
	h.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/?realm=unknown", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid explicit realm = %d; want 400", invalid.Code)
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

func TestMockPortalManagementAndSelfService(t *testing.T) {
	c := config.Config{MockMode: true, PortalName: "Azeroth", RealmName: "Azeroth", RealmAddress: "logon.test", BrandMark: "A", ThemePrimary: "#d3ae68", ThemeSecondary: "#f3d89c", ThemeAccent: "#3fd0be", ThemeBackground: "#07110f", EnableArmory: true, EnableRankings: true, EnableShop: true, EnableAdminPanel: true}
	s := &Server{c: c, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	if login.Code != http.StatusOK || len(login.Result().Cookies()) == 0 {
		t.Fatalf("mock login failed: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/rankings?metric=achievements", "/api/rankings?metric=guild-members", "/api/characters/deleted", "/api/admin/settings", "/api/admin/news", "/api/admin/coupons"} {
		if w := do(http.MethodGet, path, ""); w.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
	if w := do(http.MethodGet, "/api/admin/items?q=bag", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Frostweave Bag") {
		t.Fatalf("item autocomplete returned %d: %s", w.Code, w.Body.String())
	}
	createdProduct := do(http.MethodPost, "/api/admin/products", `{"name":"Raid preparation","description":"Bags and supplies","price":25,"category":"Utility","gold":5000,"items":[{"itemId":41599,"quantity":4}]}`)
	if createdProduct.Code != http.StatusCreated {
		t.Fatalf("product create returned %d: %s", createdProduct.Code, createdProduct.Body.String())
	}
	var created map[string]any
	if json.NewDecoder(createdProduct.Body).Decode(&created) != nil {
		t.Fatal("invalid product response")
	}
	productID := uint32(created["id"].(float64))
	if w := do(http.MethodGet, fmt.Sprintf("/api/admin/products/%d", productID), ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Frostweave Bag") {
		t.Fatalf("product detail returned %d: %s", w.Code, w.Body.String())
	}
	updateBody := `{"name":"Raid preparation plus","description":"Bags, weapon and gold","price":30,"category":"Utility","gold":6000,"active":true,"items":[{"itemId":51809,"quantity":2},{"itemId":49623,"quantity":1}]}`
	if w := do(http.MethodPut, fmt.Sprintf("/api/admin/products/%d", productID), updateBody); w.Code != http.StatusOK {
		t.Fatalf("product update returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodDelete, fmt.Sprintf("/api/admin/products/%d", productID), ""); w.Code != http.StatusOK {
		t.Fatalf("product archive returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/characters/1/service", `{"action":"unstuck"}`); w.Code != http.StatusOK {
		t.Fatalf("unstuck returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/characters/99/service", `{"action":"restore"}`); w.Code != http.StatusOK {
		t.Fatalf("restore returned %d: %s", w.Code, w.Body.String())
	}
	settings := `{"portalName":"Frosthold","realmName":"Frosthold","brandMark":"F","tagline":"Test realm","realmAddress":"logon.frosthold.test","experienceRate":"3×","downloadUrl":"","communityUrl":"","termsUrl":"","privacyUrl":"","themePrimary":"#d3ae68","themeSecondary":"#f3d89c","themeAccent":"#3fd0be","themeBackground":"#07110f","maintenanceEnabled":true,"maintenanceMessage":"Restart in progress","registration":true,"armory":true,"rankings":true,"guilds":true,"realm":true,"shop":true,"support":true,"admin":true,"gmConsole":false}`
	if w := do(http.MethodPut, "/api/admin/settings", settings); w.Code != http.StatusOK {
		t.Fatalf("settings update returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/public-config", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"portalName":"Frosthold"`) || !strings.Contains(w.Body.String(), `"experienceRate":"3×"`) || !strings.Contains(w.Body.String(), `"active":true`) {
		t.Fatalf("runtime settings not public: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/news", `{"title":"Patch notes","summary":"A new season begins.","kind":"announcement","active":true}`); w.Code != http.StatusCreated {
		t.Fatalf("news create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/coupons", `{"code":"WELCOME10","discountPercent":10,"perAccountLimit":1}`); w.Code != http.StatusCreated {
		t.Fatalf("coupon create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/shop/purchase", `{"productId":1,"characterGuid":1,"coupon":"WELCOME10"}`); w.Code != http.StatusCreated {
		t.Fatalf("discounted purchase returned %d: %s", w.Code, w.Body.String())
	}
	me := do(http.MethodGet, "/api/me", "")
	if !strings.Contains(me.Body.String(), `"balance":392`) {
		t.Fatalf("coupon was not applied transactionally: %s", me.Body.String())
	}
}

func TestPublicBrandingConfig(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, PortalName: "Frosthold", RealmName: "Frosthold One", RealmAddress: "logon.frosthold.test", RealmKey: "frost", Realms: []config.RealmConfig{{Key: "frost", Name: "Frosthold One"}, {Key: "ember", Name: "Emberfall"}}, BrandMark: "FH", ExpansionName: "Wrath", ClientVersion: "3.3.5a"}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
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
	realms := body["realms"].([]any)
	if body["realmKey"] != "frost" || len(realms) != 2 {
		t.Fatalf("unexpected realm directory: %#v", body)
	}
	if _, exposed := body["realmControlToken"]; exposed {
		t.Fatal("private realm control token was exposed")
	}
	if _, exposed := realms[0].(map[string]any)["charactersDsn"]; exposed {
		t.Fatal("private multi-realm connection settings were exposed")
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
	if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "https://code.jquery.com") {
		t.Fatalf("content security policy does not allow the pinned jQuery CDN: %q", got)
	}
}

func TestTechnicalStatusRequiresGM(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableAdminPanel: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()

	public := httptest.NewRecorder()
	h.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if public.Code != http.StatusOK || strings.Contains(public.Body.String(), `"database"`) {
		t.Fatalf("public status leaked technical state: %d %s", public.Code, public.Body.String())
	}

	denied := httptest.NewRecorder()
	h.ServeHTTP(denied, httptest.NewRequest(http.MethodGet, "/api/admin/status", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated admin status = %d; want 403", denied.Code)
	}

	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/status", nil)
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	admin := httptest.NewRecorder()
	h.ServeHTTP(admin, request)
	if admin.Code != http.StatusOK || !strings.Contains(admin.Body.String(), `"database":true`) {
		t.Fatalf("GM status unavailable: %d %s", admin.Code, admin.Body.String())
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

func TestSplitItemStacksUsesTemplateStackLimit(t *testing.T) {
	got := splitItemStacks(40113, 45, 20)
	want := []string{"40113:20", "40113:20", "40113:5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}

	got = splitItemStacks(46152, 2, 1)
	want = []string{"46152:1", "46152:1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}

	attachments := make([]string, 25)
	for i := range attachments {
		attachments[i] = fmt.Sprintf("%d:1", i+1)
	}
	messages := chunkMailStacks(attachments)
	if len(messages) != 3 || len(messages[0]) != 12 || len(messages[1]) != 12 || len(messages[2]) != 1 {
		t.Fatalf("unexpected mail split: %v", messages)
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
