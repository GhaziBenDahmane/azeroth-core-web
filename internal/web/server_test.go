package web

import (
	"bytes"
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
		"index.html":         {Data: []byte("home")},
		"404.html":           {Data: []byte("missing")},
		"admin/index.html":   {Data: []byte("admin")},
		"armory/index.html":  {Data: []byte("armory")},
		"account/index.html": {Data: []byte("account")},
		"news/index.html":    {Data: []byte("news")},
		"pages/index.html":   {Data: []byte("pages")},
		"tracker/index.html": {Data: []byte("tracker")},
		"app.js":             {Data: []byte("script")},
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
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/armory/Arthoria", nil))
	if w.Code != http.StatusOK || w.Body.String() != "armory" {
		t.Fatalf("character route = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/account/security", nil))
	if w.Code != http.StatusOK || w.Body.String() != "account" {
		t.Fatalf("account route = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/news/patch-notes", nil))
	if w.Code != http.StatusOK || w.Body.String() != "news" {
		t.Fatalf("article route = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pages/rules", nil))
	if w.Code != http.StatusOK || w.Body.String() != "pages" {
		t.Fatalf("content page route = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/tracker/42", nil))
	if w.Code != http.StatusOK || w.Body.String() != "tracker" {
		t.Fatalf("tracker route = %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/not-a-route", nil))
	if w.Code != http.StatusNotFound || w.Body.String() != "missing" {
		t.Fatalf("unknown route = %d %q", w.Code, w.Body.String())
	}
}

func TestMockDeliveryDiagnosticRequiresDesignatedCharacterConfirmation(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableAdminPanel: true, GMLevel: 3, DeliveryDiagnosticCharacter: "Portalprobe"}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	s.mock.mu.Lock()
	s.mock.stepUpUntil = time.Now().Add(time.Hour)
	s.mock.mu.Unlock()
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/admin/delivery-diagnostic", strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := post(`{"confirm":"wrong"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong confirmation = %d %s", w.Code, w.Body.String())
	}
	if w := post(`{"confirm":"Portalprobe"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"itemId":117`) || !strings.Contains(w.Body.String(), `"simulated":true`) {
		t.Fatalf("diagnostic = %d %s", w.Code, w.Body.String())
	}
}

func TestMockCommunityTrackerJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableAdminPanel: true, GMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()

	list := httptest.NewRecorder()
	h.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/community/issues?sort=votes", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"title":"Add a weekly Trial of the Crusader event"`) {
		t.Fatalf("tracker list = %d %s", list.Code, list.Body.String())
	}

	denied := httptest.NewRecorder()
	h.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/community/issues", strings.NewReader(`{"kind":"suggestion","category":"website","title":"Improve search","body":"Please add keyboard navigation to every search result."}`)))
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous submission = %d; want 401", denied.Code)
	}

	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	created := request(http.MethodPost, "/api/community/issues", `{"kind":"suggestion","category":"website","title":"Improve keyboard search","body":"Please add keyboard navigation to every search result."}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"id":3`) {
		t.Fatalf("tracker create = %d %s", created.Code, created.Body.String())
	}
	voted := request(http.MethodPost, "/api/community/issues/3/vote", `{}`)
	if voted.Code != http.StatusOK || !strings.Contains(voted.Body.String(), `"voted":true`) {
		t.Fatalf("tracker vote = %d %s", voted.Code, voted.Body.String())
	}
	commented := request(http.MethodPost, "/api/community/issues/3/comments", `{"body":"This is particularly useful for autocomplete results."}`)
	if commented.Code != http.StatusCreated {
		t.Fatalf("tracker comment = %d %s", commented.Code, commented.Body.String())
	}
	s.mock.mu.Lock()
	s.mock.stepUpUntil = time.Now().Add(time.Hour)
	s.mock.mu.Unlock()
	triaged := request(http.MethodPut, "/api/admin/community/issues/3", `{"status":"planned","priority":"high","labels":"website, accessibility","staffResponse":"Accepted for the next interface pass."}`)
	if triaged.Code != http.StatusOK {
		t.Fatalf("tracker triage = %d %s", triaged.Code, triaged.Body.String())
	}
	detail := request(http.MethodGet, "/api/community/issues/3", "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"status":"planned"`) || !strings.Contains(detail.Body.String(), `"commentCount":1`) {
		t.Fatalf("tracker detail = %d %s", detail.Code, detail.Body.String())
	}
}

func TestMockMasterAccountJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	accounts := request(http.MethodGet, "/api/identity/accounts", "")
	if accounts.Code != http.StatusOK || !strings.Contains(accounts.Body.String(), `"primary":true`) {
		t.Fatalf("identity accounts = %d %s", accounts.Code, accounts.Body.String())
	}
	if got := request(http.MethodPost, "/api/identity/accounts", `{"username":"ALT","password":"secret"}`).Code; got != http.StatusPreconditionRequired {
		t.Fatalf("link without recent authentication = %d; want 428", got)
	}
	if got := request(http.MethodPost, "/api/security/step-up", `{"password":"demo1234"}`).Code; got != http.StatusOK {
		t.Fatalf("step-up = %d", got)
	}
	if got := request(http.MethodPost, "/api/identity/accounts", `{"username":"ALT","password":"secret","label":"Alt"}`).Code; got != http.StatusCreated {
		t.Fatalf("link after recent authentication = %d", got)
	}
}

func TestMockGuildRecruitmentJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableGuilds: true, EnableAdminPanel: true, GMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	profile := httptest.NewRecorder()
	h.ServeHTTP(profile, httptest.NewRequest(http.MethodGet, "/api/guilds/1/recruitment", nil))
	if profile.Code != http.StatusOK || !strings.Contains(profile.Body.String(), `"headline":"Progress through Northrend with a steady team"`) {
		t.Fatalf("recruitment profile = %d %s", profile.Code, profile.Body.String())
	}
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	created := request(http.MethodPost, "/api/guilds/1/applications", `{"characterGuid":5,"message":"I play Marksmanship and can attend both scheduled raid nights."}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("guild application = %d %s", created.Code, created.Body.String())
	}
	duplicate := request(http.MethodPost, "/api/guilds/1/applications", `{"characterGuid":5,"message":"This duplicate application should be rejected by the portal."}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate application = %d; want 409", duplicate.Code)
	}
	applications := request(http.MethodGet, "/api/guild-applications", "")
	if applications.Code != http.StatusOK || !strings.Contains(applications.Body.String(), `"characterName":"Quickarrow"`) {
		t.Fatalf("player applications = %d %s", applications.Code, applications.Body.String())
	}
	s.mock.mu.Lock()
	s.mock.stepUpUntil = time.Now().Add(time.Hour)
	s.mock.mu.Unlock()
	updated := request(http.MethodPut, "/api/admin/guild-applications/1", `{"status":"accepted","response":"Welcome to the raid team.","staffNote":"Verified schedule internally."}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("application review = %d %s", updated.Code, updated.Body.String())
	}
	profile = request(http.MethodGet, "/api/guilds/1/recruitment", "")
	if !strings.Contains(profile.Body.String(), `"status":"accepted"`) || !strings.Contains(profile.Body.String(), `"response":"Welcome to the raid team."`) {
		t.Fatalf("profile application status missing: %s", profile.Body.String())
	}
	if strings.Contains(profile.Body.String(), `staffNote`) || strings.Contains(profile.Body.String(), "Verified schedule internally.") {
		t.Fatalf("profile leaked internal staff note: %s", profile.Body.String())
	}
	applications = request(http.MethodGet, "/api/guild-applications", "")
	if !strings.Contains(applications.Body.String(), `"response":"Welcome to the raid team."`) || strings.Contains(applications.Body.String(), `staffNote`) {
		t.Fatalf("player application response/privacy mismatch: %s", applications.Body.String())
	}
}

func TestMockArenaSeasonArchiveJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableRankings: true, EnableAdminPanel: true, GMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	archive := request(http.MethodGet, "/api/arena?bracket=3&season=season-7", "")
	if archive.Code != http.StatusOK || !strings.Contains(archive.Body.String(), `"seasonName":"Season 7"`) || !strings.Contains(archive.Body.String(), `"source":"Immutable portal season snapshot"`) {
		t.Fatalf("archived arena ladder = %d %s", archive.Code, archive.Body.String())
	}
	if got := request(http.MethodPost, "/api/security/step-up", `{"password":"demo1234"}`).Code; got != http.StatusOK {
		t.Fatalf("step-up = %d", got)
	}
	captured := request(http.MethodPost, "/api/admin/arena-seasons", `{"name":"Season 8","slug":"season-8"}`)
	if captured.Code != http.StatusCreated || !strings.Contains(captured.Body.String(), `"capturedTeams":12`) {
		t.Fatalf("season capture = %d %s", captured.Code, captured.Body.String())
	}
}

func TestMockToolsJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableAdminPanel: true, GMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	for path, expected := range map[string]string{
		"/api/tools/resources?kind=addon&q=boss": `"title":"Deadly Boss Mods"`,
		"/api/tools/items?q=shadow":              `"name":"Shadowmourne"`,
		"/api/tools/talents?class=2":             `"name":"Retribution"`,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("%s = %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestMockCharacterPrivacyJourney(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableArmory: true}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	request := httptest.NewRequest(http.MethodPut, "/api/characters/1/privacy", strings.NewReader(`{"hidden":true,"showGear":false,"showActivity":false}`))
	for _, cookie := range login.Result().Cookies() {
		request.AddCookie(cookie)
	}
	updated := httptest.NewRecorder()
	h.ServeHTTP(updated, request)
	if updated.Code != http.StatusOK {
		t.Fatalf("privacy update = %d %s", updated.Code, updated.Body.String())
	}
	search := httptest.NewRecorder()
	h.ServeHTTP(search, httptest.NewRequest(http.MethodGet, "/api/armory?q=Arthoria", nil))
	if search.Code != http.StatusOK || strings.Contains(search.Body.String(), "Arthoria") {
		t.Fatalf("hidden character leaked through search: %s", search.Body.String())
	}
	publicProfile := httptest.NewRecorder()
	h.ServeHTTP(publicProfile, httptest.NewRequest(http.MethodGet, "/api/armory/Arthoria", nil))
	if publicProfile.Code != http.StatusNotFound {
		t.Fatalf("hidden profile = %d; want 404", publicProfile.Code)
	}
	ownedRequest := httptest.NewRequest(http.MethodGet, "/api/armory/Arthoria", nil)
	for _, cookie := range login.Result().Cookies() {
		ownedRequest.AddCookie(cookie)
	}
	owned := httptest.NewRecorder()
	h.ServeHTTP(owned, ownedRequest)
	if owned.Code != http.StatusOK || !strings.Contains(owned.Body.String(), `"equipment":[]`) {
		t.Fatalf("owner privacy preview = %d %s", owned.Code, owned.Body.String())
	}
}

func TestSPAHandlerCachesHashedAssets(t *testing.T) {
	root := fstest.MapFS{"assets/app.abc123.js": {Data: []byte("script")}}
	w := httptest.NewRecorder()
	spaHandler(root).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/app.abc123.js", nil))
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset cache policy = %q", got)
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

func TestTOTPEncryptionAndRecoveryCodes(t *testing.T) {
	s := &Server{c: config.Config{TOTPEncryptionKey: bytes.Repeat([]byte{7}, 32)}}
	encrypted, err := s.encryptTOTP("JBSWY3DPEHPK3PXP")
	if err != nil || !strings.HasPrefix(string(encrypted), "v1:") || strings.Contains(string(encrypted), "JBSWY3DPEHPK3PXP") {
		t.Fatalf("encrypted secret = %q, err = %v", encrypted, err)
	}
	plain, err := s.decryptTOTP(encrypted)
	if err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decrypted secret = %q, err = %v", plain, err)
	}
	codes, hashes, err := generateRecoveryCodes(10)
	if err != nil || len(codes) != 10 || len(hashes) != 10 {
		t.Fatalf("recovery codes = %d/%d, err = %v", len(codes), len(hashes), err)
	}
	seen := map[string]bool{}
	for i, code := range codes {
		if len(code) != 19 || seen[code] || hashes[i] != recoveryCodeHash(code) {
			t.Fatalf("invalid recovery code %q", code)
		}
		seen[code] = true
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

func TestStaffPermissionTiers(t *testing.T) {
	s := &Server{c: config.Config{SupportGMLevel: 1, ModeratorGMLevel: 2, GMLevel: 3}}
	tests := []struct {
		level uint8
		role  string
		want  []string
	}{
		{0, "Player", []string{}},
		{1, "Support", []string{"support", "monitoring"}},
		{2, "Moderator", []string{"support", "monitoring", "players", "moderation", "audit"}},
		{3, "Administrator", []string{"support", "monitoring", "players", "moderation", "audit", "overview", "commerce", "content", "realm", "settings", "admin"}},
	}
	for _, test := range tests {
		if got := s.staffPermissions(test.level); !reflect.DeepEqual(got, test.want) {
			t.Errorf("level %d permissions = %v; want %v", test.level, got, test.want)
		}
		if got := staffRole(account{GMLevel: test.level}, s.c); got != test.role {
			t.Errorf("level %d role = %q; want %q", test.level, got, test.role)
		}
	}
	s.c.StaffShopManagers = map[string]bool{"MERCHANT": true}
	merchant := account{Username: "merchant"}
	if got := staffRole(merchant, s.c); got != "Shop manager" || !reflect.DeepEqual(s.staffPermissionsFor(merchant.GMLevel, merchant.Username), []string{"commerce"}) {
		t.Fatalf("shop manager role or permissions not applied")
	}
}

func TestPublicConfigIncludesHomepageContent(t *testing.T) {
	c := config.Config{
		MockMode: true, PortalName: "Frosthold", RealmName: "Frosthold", RealmKey: "frost",
		HomeHeadline: "Enter Frosthold", HomeEyebrow: "Realm status", HomePrimaryCTA: "Join now", HomeConnectTitle: "Connect today",
		GoogleClientID: "google-client", GoogleClientSecret: "google-secret",
	}
	s := &Server{c: c, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/public-config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("public config returned %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode public config: %v", err)
	}
	for key, want := range map[string]string{
		"homeHeadline": "Enter Frosthold", "homeEyebrow": "Realm status", "homePrimaryCta": "Join now", "homeConnectTitle": "Connect today",
	} {
		if got := body[key]; got != want {
			t.Errorf("%s = %#v; want %q", key, got, want)
		}
	}
	capabilities, ok := body["capabilities"].(map[string]any)
	if !ok || capabilities["googleOAuth"] != true {
		t.Fatalf("Google OAuth capability = %#v", body["capabilities"])
	}
}

func TestGoogleOAuthIsCapabilityGatedInMockMode(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, RealmKey: "default"}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/google/start", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unconfigured Google OAuth returned %d; want 404", w.Code)
	}
	s.c.GoogleClientID, s.c.GoogleClientSecret = "client", "secret"
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/auth/google/start", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("mock Google OAuth returned %d; want 501", w.Code)
	}
}

func TestMockVoteCallbackIsAuthenticatedAndIdempotent(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, VoteCallbackSecret: "provider-secret-123", VoteRewardCredits: 10}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	request := func(secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/rewards/vote/callback", strings.NewReader(`{"username":"DEMO","eventId":"vote-42"}`))
		r.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("wrong"); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid vote secret returned %d", w.Code)
	}
	if w := request("provider-secret-123"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"credits":10`) {
		t.Fatalf("vote reward failed: %d %s", w.Code, w.Body.String())
	}
	if w := request("provider-secret-123"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate vote was not idempotent: %d %s", w.Code, w.Body.String())
	}
	if s.mock.balance != 510 {
		t.Fatalf("vote reward balance = %d; want 510", s.mock.balance)
	}
}

func TestMockPortalManagementAndSelfService(t *testing.T) {
	c := config.Config{MockMode: true, PortalName: "Azeroth", RealmName: "Azeroth", RealmAddress: "logon.test", BrandMark: "A", ThemePrimary: "#d3ae68", ThemeSecondary: "#f3d89c", ThemeAccent: "#3fd0be", ThemeBackground: "#07110f", EnableArmory: true, EnableRankings: true, EnableGuilds: true, EnableShop: true, EnableAdminPanel: true}
	s := &Server{c: c, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	if login.Code != http.StatusOK || len(login.Result().Cookies()) == 0 {
		t.Fatalf("mock login failed: %d %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	s.mock.stepUpUntil = time.Now().Add(time.Minute)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/rankings?metric=achievements", "/api/rankings?metric=guild-members", "/api/characters/deleted", "/api/admin/settings", "/api/admin/news", "/api/admin/coupons?page=1&perPage=10", "/api/admin/gift-codes?page=1&perPage=10", "/api/admin/shop/stock?page=1&perPage=10", "/api/admin/transfers?page=1&perPage=10", "/api/admin/privacy-requests?page=1&perPage=10", "/api/admin/guild-applications?page=1&perPage=10", "/api/billing/transactions", "/api/admin/payments?page=1&perPage=10", "/api/admin/audit/export?source=portal"} {
		w := do(http.MethodGet, path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(path, "page=1") && !strings.Contains(w.Body.String(), `"pagination"`) {
			t.Fatalf("%s omitted pagination metadata: %s", path, w.Body.String())
		}
	}
	if w := do(http.MethodPost, "/api/admin/accounts/2/revoke-sessions", `{}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"revoked":2`) || !strings.Contains(w.Body.String(), `"requestId"`) {
		t.Fatalf("session revocation returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/accounts/2/require-password-reset", `{"reason":"Compromised credentials"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"username":"FROSTBYTE"`) || !strings.Contains(w.Body.String(), `"requestId"`) {
		t.Fatalf("required password reset returned %d: %s", w.Code, w.Body.String())
	}
	bulk := do(http.MethodPost, "/api/admin/orders/bulk-retry", `{"ids":[1,1,0,2]}`)
	if bulk.Code != http.StatusOK || !strings.Contains(bulk.Body.String(), `"requested":4`) || !strings.Contains(bulk.Body.String(), `"succeeded":2`) || !strings.Contains(bulk.Body.String(), `"failed":2`) || !strings.Contains(bulk.Body.String(), `"status":"skipped"`) || !strings.Contains(bulk.Body.String(), `"requestId"`) {
		t.Fatalf("bulk order retry did not return a structured partial result: %d %s", bulk.Code, bulk.Body.String())
	}
	if policy := do(http.MethodGet, "/api/admin/investigations/policy", ""); policy.Code != http.StatusOK || !strings.Contains(policy.Body.String(), `"networkRetentionDays"`) || !strings.Contains(policy.Body.String(), "Raw IP addresses") {
		t.Fatalf("investigation policy = %d %s", policy.Code, policy.Body.String())
	}
	investigation := do(http.MethodPost, "/api/admin/investigations/search", `{"account":"DEMO","reason":"Review suspected account evasion"}`)
	if investigation.Code != http.StatusOK || !strings.Contains(investigation.Body.String(), `"username":"HELPER"`) || !strings.Contains(investigation.Body.String(), `"requestId"`) {
		t.Fatalf("privacy-aware investigation = %d %s", investigation.Code, investigation.Body.String())
	}
	if evidence := do(http.MethodPost, "/api/admin/investigations/evidence", `{"accountId":1,"caseReference":"CASE-1","note":"Screenshot supplied by the reporting player","evidenceUrl":"http://unsafe.test/evidence"}`); evidence.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe evidence URL = %d %s", evidence.Code, evidence.Body.String())
	}
	if evidence := do(http.MethodPost, "/api/admin/investigations/evidence", `{"accountId":1,"caseReference":"CASE-1","note":"Screenshot supplied by the reporting player","evidenceUrl":"https://evidence.example.test/case-1"}`); evidence.Code != http.StatusCreated || !strings.Contains(evidence.Body.String(), `"requestId"`) {
		t.Fatalf("evidence attachment = %d %s", evidence.Code, evidence.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/payments/cs_demo/refund", ""); w.Code != http.StatusAccepted {
		t.Fatalf("mock payment refund returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPut, "/api/wishlist/1", `{}`); w.Code != http.StatusOK {
		t.Fatalf("wishlist save returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/wishlist", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"productIds":[1]`) {
		t.Fatalf("wishlist list returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodDelete, "/api/wishlist/1", ""); w.Code != http.StatusOK {
		t.Fatalf("wishlist delete returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/armory/Arthoria/insights", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"name":"Divine Storm"`) || !strings.Contains(w.Body.String(), `"name":"The Light of Dawn"`) || !strings.Contains(w.Body.String(), `"pointsKnown":true`) {
		t.Fatalf("enriched armory insights returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/admin/items?q=bag", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Frostweave Bag") {
		t.Fatalf("item autocomplete returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/billing/packages", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"slug":"small"`) || strings.Contains(w.Body.String(), "price_") {
		t.Fatalf("public credit packages returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/credit-packages", `{"slug":"supporter","name":"Supporter pack","stripePriceId":"price_supporter","credits":250,"bonusLabel":"Thank you","sortOrder":1}`); w.Code != http.StatusCreated {
		t.Fatalf("credit package create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/billing/packages", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"slug":"supporter"`) || strings.Contains(w.Body.String(), "price_supporter") {
		t.Fatalf("configured public credit packages returned %d: %s", w.Code, w.Body.String())
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
	if w := do(http.MethodGet, "/api/notifications", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"kind":"maintenance"`) || !strings.Contains(w.Body.String(), "Restart in progress") {
		t.Fatalf("maintenance notification not delivered: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/public-config", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"portalName":"Frosthold"`) || !strings.Contains(w.Body.String(), `"experienceRate":"3×"`) || !strings.Contains(w.Body.String(), `"active":true`) {
		t.Fatalf("runtime settings not public: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/news", `{"title":"Patch notes","summary":"A new season begins.","kind":"announcement","active":true}`); w.Code != http.StatusCreated {
		t.Fatalf("news create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/news/patch-notes", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"published"`) {
		t.Fatalf("public article returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/admin/news/2/revisions", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"newsId":2`) {
		t.Fatalf("article revisions returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/pages", `{"title":"Realm rules","slug":"rules","summary":"Read before playing","body":"Be respectful and play fairly.","status":"published","showFooter":true}`); w.Code != http.StatusCreated {
		t.Fatalf("content page create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/pages/rules", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"title":"Realm rules"`) {
		t.Fatalf("public content page returned %d: %s", w.Code, w.Body.String())
	}
	eventBody := fmt.Sprintf(`{"title":"Trial of the Crusader","description":"Community raid night","category":"raid","location":"Icecrown","startsAt":%q,"status":"scheduled","maxParticipants":25}`, time.Now().Add(24*time.Hour).Format(time.RFC3339))
	if w := do(http.MethodPost, "/api/admin/events", eventBody); w.Code != http.StatusCreated {
		t.Fatalf("event create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/events", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"title":"Trial of the Crusader"`) {
		t.Fatalf("public events returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/transfers", `{"sourceRealm":"Old Realm","characterName":"Oldhero","sourceProfileUrl":"https://example.test/armory/Oldhero","playerNote":"Transfer requested"}`); w.Code != http.StatusCreated {
		t.Fatalf("transfer create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/transfers/1", `{"status":"reviewing","staffNote":"Proof review started"}`); w.Code != http.StatusOK {
		t.Fatalf("transfer review returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/transfers", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"reviewing"`) {
		t.Fatalf("transfer list returned %d: %s", w.Code, w.Body.String())
	}
	expires := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	if w := do(http.MethodPost, "/api/admin/staff", fmt.Sprintf(`{"username":"HELPER","role":"content_manager","realmKey":"*","expiresAt":%q,"permissions":["content","audit","not-valid"]}`, expires)); w.Code != http.StatusOK {
		t.Fatalf("staff role create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/admin/staff", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"username":"HELPER"`) || !strings.Contains(w.Body.String(), `"role":"content_manager"`) || !strings.Contains(w.Body.String(), `"realmKey":"*"`) || !strings.Contains(w.Body.String(), `"permissions":["content","audit"]`) || strings.Contains(w.Body.String(), "not-valid") {
		t.Fatalf("staff role list returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/staff", `{"username":"HELPER","role":"support","realmKey":"another-realm"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("foreign realm staff scope returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/staff", `{"username":"HELPER","role":"support","expiresAt":"2020-01-01T00:00:00Z"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expired staff role returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodDelete, "/api/admin/staff/2?realm=*", ""); w.Code != http.StatusOK {
		t.Fatalf("scoped staff delete returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/admin/coupons", `{"code":"WELCOME10","discountPercent":10,"perAccountLimit":1,"allowSale":true}`); w.Code != http.StatusCreated {
		t.Fatalf("coupon create returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/shop/purchase", `{"productId":1,"variantId":1,"characterGuid":1,"coupon":"WELCOME10"}`); w.Code != http.StatusCreated {
		t.Fatalf("discounted purchase returned %d: %s", w.Code, w.Body.String())
	}
	me := do(http.MethodGet, "/api/me", "")
	// Product 1 is on sale for 95 credits in the mock catalog; the coupon
	// removes another 9 credits (integer percentage calculation).
	if !strings.Contains(me.Body.String(), `"balance":414`) {
		t.Fatalf("coupon was not applied transactionally: %s", me.Body.String())
	}
	wallet := do(http.MethodGet, "/api/wallet", "")
	if wallet.Code != http.StatusOK || !strings.Contains(wallet.Body.String(), `"amount":-86`) || !strings.Contains(wallet.Body.String(), `"reason":"Order 1044 purchase"`) {
		t.Fatalf("wallet did not expose purchase debit: %d %s", wallet.Code, wallet.Body.String())
	}
	createdCode := do(http.MethodPost, "/api/admin/gift-codes", `{"credits":20,"maxUses":1}`)
	if createdCode.Code != http.StatusCreated {
		t.Fatalf("gift code create returned %d: %s", createdCode.Code, createdCode.Body.String())
	}
	var gift map[string]any
	if json.NewDecoder(createdCode.Body).Decode(&gift) != nil || gift["code"] == "" {
		t.Fatal("gift code was not returned once")
	}
	redeemBody := fmt.Sprintf(`{"code":%q}`, gift["code"])
	if w := do(http.MethodPost, "/api/gift-codes/redeem", redeemBody); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"credits":20`) {
		t.Fatalf("gift redeem returned %d: %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/gift-codes/redeem", redeemBody); w.Code != http.StatusConflict {
		t.Fatalf("gift reuse returned %d: %s", w.Code, w.Body.String())
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

func TestLauncherManifestIsRealmAwareAndVerifiable(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, RealmKey: "frost", RealmName: "Frosthold", RealmAddress: "logon.frosthold.test", ExpansionName: "Wrath of the Lich King", ClientVersion: "3.3.5a", ClientBuild: "12340"}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	r := httptest.NewRequest(http.MethodGet, "/api/launcher/manifest", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var body struct {
		SchemaVersion int              `json:"schemaVersion"`
		Realm         map[string]any   `json:"realm"`
		Client        map[string]any   `json:"client"`
		Packages      []portalDownload `json:"packages"`
		Patches       []launcherPatch  `json:"patches"`
	}
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&body) != nil {
		t.Fatalf("launcher manifest returned %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Portal-API-Version") != "1" {
		t.Fatalf("launcher API version header = %q", w.Header().Get("X-Portal-API-Version"))
	}
	if body.SchemaVersion != 2 || body.Realm["address"] != "logon.frosthold.test" || body.Client["build"] != "12340" {
		t.Fatalf("unexpected launcher identity: %#v", body)
	}
	if len(body.Packages) != 1 || len(body.Packages[0].SHA256) != 64 || body.Packages[0].VirusTotalURL == "" || body.Packages[0].Requirements == "" {
		t.Fatalf("launcher package is missing verification metadata: %#v", body.Packages)
	}
	if len(body.Packages[0].Mirrors) != 1 || len(body.Patches) != 1 || len(body.Patches[0].SHA256) != 64 || len(body.Patches[0].Mirrors) != 1 {
		t.Fatalf("launcher manifest is missing mirror or patch metadata: packages=%#v patches=%#v", body.Packages, body.Patches)
	}
}

func TestMockEventRegistrationAttendanceAndRewardAreIdempotent(t *testing.T) {
	s := &Server{c: config.Config{MockMode: true, EnableAdminPanel: true, EnableSupport: true, GMLevel: 3}, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if got := request(http.MethodPost, "/api/events/1/registration", `{"characterGuid":1}`); got.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/security/step-up", `{"password":"demo1234"}`); got.Code != http.StatusOK {
		t.Fatalf("step-up = %d %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPut, "/api/admin/events/1/participants/1", `{"status":"attended"}`); got.Code != http.StatusOK {
		t.Fatalf("attendance = %d %s", got.Code, got.Body.String())
	}
	first := request(http.MethodPost, "/api/admin/events/1/rewards", `{"accountIds":[1],"reason":"Wintergrasp attendance"}`)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"status":"awarded"`) {
		t.Fatalf("first reward = %d %s", first.Code, first.Body.String())
	}
	second := request(http.MethodPost, "/api/admin/events/1/rewards", `{"accountIds":[1],"reason":"Wintergrasp attendance"}`)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"status":"duplicate"`) {
		t.Fatalf("duplicate reward = %d %s", second.Code, second.Body.String())
	}
	list := request(http.MethodGet, "/api/events", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"viewerRegistration"`) || !strings.Contains(list.Body.String(), `"registeredCount":1`) {
		t.Fatalf("event registration state = %d %s", list.Code, list.Body.String())
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
	if got := w.Header().Get("X-Request-ID"); len(got) != 32 {
		t.Fatalf("generated request ID = %q", got)
	}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "edge-request_123")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, request)
	if got := w.Header().Get("X-Request-ID"); got != "edge-request_123" {
		t.Fatalf("forwarded request ID = %q", got)
	}
}

func TestMockAdminMutationRequiresStepUp(t *testing.T) {
	s := New(nil, config.Config{MockMode: true, EnableAdminPanel: true}, fstest.MapFS{"index.html": {Data: []byte("home")}})
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	cookies := login.Result().Cookies()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if got := request(http.MethodPost, "/api/admin/credits", `{}`).Code; got != http.StatusPreconditionRequired {
		t.Fatalf("mutation without step-up = %d; want 428", got)
	}
	if got := request(http.MethodPost, "/api/security/step-up", `{"password":"demo1234"}`).Code; got != http.StatusOK {
		t.Fatalf("step-up = %d; want 200", got)
	}
	if got := request(http.MethodPost, "/api/admin/credits", `{"username":"DEMO","amount":1,"reason":"test"}`).Code; got != http.StatusOK {
		t.Fatalf("mutation after step-up = %d; want 200", got)
	}
}

func TestMockShopProductDetailAndEligibility(t *testing.T) {
	s := New(nil, config.Config{MockMode: true, EnableShop: true}, fstest.MapFS{"shop/index.html": {Data: []byte("shop")}, "404.html": {Data: []byte("missing")}})
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		for _, cookie := range login.Result().Cookies() {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("/api/shop/7"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Complete Level 80 Boost") {
		t.Fatalf("product detail = %d %s", w.Code, w.Body.String())
	}
	if w := request("/api/shop/7/eligibility"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"eligible":true`) {
		t.Fatalf("eligibility = %d %s", w.Code, w.Body.String())
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shop/7", nil))
	if w.Code != http.StatusOK || w.Body.String() != "shop" {
		t.Fatalf("canonical shop route = %d %q", w.Code, w.Body.String())
	}
}

func TestEligibilityReasons(t *testing.T) {
	p := product{Price: 100, ClassID: 2, StockLimit: 1, SoldCount: 1, PerAccountLimit: 1}
	c := character{Class: 1, Online: true}
	reasons := eligibilityReasons(p, c, 50, 1, false)
	if len(reasons) != 6 {
		t.Fatalf("eligibility reasons = %#v; want all six blockers", reasons)
	}
}

func TestMockPrivacyWorkflow(t *testing.T) {
	s := New(nil, config.Config{MockMode: true}, fstest.MapFS{"index.html": {Data: []byte("home")}})
	h := s.Handler()
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"DEMO","password":"demo1234"}`)))
	do := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		for _, c := range login.Result().Cookies() {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := do(http.MethodGet, "/api/privacy/export", ""); w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export = %d %s", w.Code, w.Body.String())
	}
	created := do(http.MethodPost, "/api/privacy/deletion", `{"confirmation":"DELETE","note":"Please remove my account"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("deletion request = %d %s", created.Code, created.Body.String())
	}
	if listed := do(http.MethodGet, "/api/privacy/requests", ""); listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"status":"pending"`) {
		t.Fatalf("privacy list = %d %s", listed.Code, listed.Body.String())
	}
	if cancelled := do(http.MethodDelete, "/api/privacy/requests/1", ""); cancelled.Code != http.StatusOK {
		t.Fatalf("privacy cancel = %d %s", cancelled.Code, cancelled.Body.String())
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

func TestRealmActionsRequireRealmPermission(t *testing.T) {
	for _, action := range []string{"start", "announce", "motd", "gm_level", "restart", "shutdown", "cancel_shutdown"} {
		if got := moderationPermission(action); got != "realm" {
			t.Errorf("%s requires %q; want realm", action, got)
		}
	}
	for _, action := range []string{"ban", "unban", "kick", "mute", "unmute", "ip_ban", "ip_unban"} {
		if got := moderationPermission(action); got != "moderation" {
			t.Errorf("%s requires %q; want moderation", action, got)
		}
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
