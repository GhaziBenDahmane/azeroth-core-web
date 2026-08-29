package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mockState struct {
	mu          sync.Mutex
	balance     uint32
	orders      []map[string]any
	users       map[string]string
	totpSecret  string
	totpEnabled bool
	bans        map[string]string
	moderation  []map[string]any
	commands    []consoleEntry
	tickets     []supportTicket
	setupDone   bool
	settings    siteSettings
	news        []newsEntry
	coupons     []coupon
	couponUses  map[string]uint32
	purchases   map[uint32]uint32
	deleted     []deletedCharacter
	dailyClaim  time.Time
	services    []dashboardService
	products    []product
	voteEvents  map[string]bool
}

func newMockState() *mockState {
	published := time.Now().Add(-time.Hour)
	return &mockState{balance: 500, users: map[string]string{"DEMO": "demo1234"}, bans: map[string]string{}, couponUses: map[string]uint32{}, purchases: map[uint32]uint32{}, products: buildMockProducts(), tickets: []supportTicket{{ID: 1, AccountID: 1, Username: "DEMO", CharacterGUID: 1, Subject: "Missing quest item", Message: "The quest item did not drop after the boss encounter.", Status: "open", Created: time.Now().Add(-2 * time.Hour), Updated: time.Now().Add(-2 * time.Hour)}}, orders: []map[string]any{{"id": 1042, "itemId": 49623, "quantity": 1, "total": 85, "status": "delivered", "created": time.Now().Add(-24 * time.Hour)}}, news: []newsEntry{{ID: 1, Title: "Welcome to Azeroth", Summary: "The portal is ready for your community.", Kind: "news", Active: true, Featured: true, PublishAt: &published}}, services: []dashboardService{{Action: "unstuck", Character: "Arthoria", Response: "Character moved to homebind.", Success: true, Created: time.Now().Add(-3 * time.Hour)}}, deleted: []deletedCharacter{{GUID: 99, Name: "Oldhero", DeletedAt: uint64(time.Now().Add(-24 * time.Hour).Unix())}}}
}

var mockCharacters = []character{
	{GUID: 1, Name: "Arthoria", Race: 1, Class: 2, Gender: 1, Level: 80, Zone: 1519, Online: false, TotalTime: 4827600, Guild: "Keepers of Dawn"},
	{GUID: 2, Name: "Thornhoof", Race: 6, Class: 11, Level: 80, Zone: 1637, Online: false, TotalTime: 3196800, Guild: "Keepers of Dawn"},
	{GUID: 3, Name: "Velistra", Race: 10, Class: 8, Gender: 1, Level: 76, Zone: 4395, Online: false, TotalTime: 1912200, Guild: "Silver Covenant"},
	{GUID: 4, Name: "Grimward", Race: 5, Class: 6, Level: 80, Zone: 210, Online: false, TotalTime: 5882400, Guild: "Ashen Vanguard"},
	{GUID: 5, Name: "Quickarrow", Race: 4, Class: 3, Gender: 1, Level: 71, Zone: 65, Online: false, TotalTime: 1105200},
	{GUID: 6, Name: "Emberhex", Race: 2, Class: 9, Level: 63, Zone: 3483, Online: false, TotalTime: 748800, Guild: "Ashen Vanguard"},
	{GUID: 7, Name: "Ironward", Race: 3, Class: 1, Level: 80, Zone: 1519, Online: false, TotalTime: 2750400, Guild: "Keepers of Dawn"},
	{GUID: 8, Name: "Nightshiv", Race: 5, Class: 4, Level: 78, Zone: 4395, Online: false, TotalTime: 1641600, Guild: "Ashen Vanguard"},
	{GUID: 9, Name: "Dawnprayer", Race: 11, Class: 5, Gender: 1, Level: 74, Zone: 65, Online: false, TotalTime: 1296000, Guild: "Silver Covenant"},
	{GUID: 10, Name: "Stormcaller", Race: 8, Class: 7, Level: 69, Zone: 3483, Online: false, TotalTime: 907200, Guild: ""},
}

func buildMockProducts() []product {
	products := []product{
		{ID: 1, ItemID: 49284, Quantity: 1, Price: 120, Name: "Reins of the Swift Spectral Tiger", Description: "The WotLK mount item, delivered directly through in-game mail.", Category: "Mounts"},
		{ID: 2, ItemID: 49623, Quantity: 1, Price: 85, Name: "Shadowmourne", Description: "A legendary two-handed axe for the realm's mightiest champions.", Category: "Weapons"},
		{ID: 3, ItemID: 51809, Quantity: 1, Price: 45, Name: "Portable Hole", Description: "A spacious 24-slot bag for long expeditions across Northrend.", Category: "Utility"},
		{ID: 4, ItemID: 23713, Quantity: 1, Price: 60, Name: "Hippogryph Hatchling", Description: "A loyal companion from the forests of Feralas.", Category: "Companions"},
		{ID: 5, ItemID: 37719, Quantity: 5, Price: 15, Name: "Adventurer Supply Bundle", Description: "Useful supplies for your next adventure.", Category: "Utility"},
		{ID: 6, ItemID: 50818, Quantity: 1, Price: 75, Name: "Invincible's Reins", Description: "The famed steed of the fallen prince awaits a new rider.", Category: "Mounts"},
		{ID: 7, Price: 40, Name: "Instant Level 80", Description: "Raise one existing character to level 80 and receive a starter travel kit.", Category: "Services", Tier: "Level 80", ServiceLevel: 80, Includes: []string{"Level 80 boost", "Four 20-slot bags", "Cold Weather Flying starter gold"}},
		{ID: 8, Price: 35, Name: "Race Change", Description: "Choose a new race from your current faction on your next login.", Category: "Services", Tier: "Character", ServiceAction: "race_change", Includes: []string{"AzerothCore race-change flag", "Same-faction races", "Applied to an offline character"}},
		{ID: 9, Price: 50, Name: "Faction Change", Description: "Choose a compatible race from the opposite faction on your next login.", Category: "Services", Tier: "Character", ServiceAction: "faction_change", Includes: []string{"AzerothCore faction-change flag", "Alliance ↔ Horde", "Applied to an offline character"}},
	}
	sets := []struct {
		ID                                 uint8
		Class, Spec, PvPArmor, T8Set, Role string
	}{
		{1, "Warrior", "Arms", "Plate", "Siegebreaker", "strength"}, {1, "Warrior", "Protection", "", "Siegebreaker", "tank"},
		{2, "Paladin", "Retribution", "Scaled", "Aegis", "strength"}, {2, "Paladin", "Holy", "Ornamented", "Aegis", "healer"}, {2, "Paladin", "Protection", "", "Aegis", "tank"},
		{3, "Hunter", "Marksmanship", "Chain", "Scourgestalker", "agility"}, {4, "Rogue", "Assassination", "Leather", "Terrorblade", "agility"},
		{5, "Priest", "Shadow", "Satin", "Sanctification", "caster"}, {5, "Priest", "Holy", "Mooncloth", "Sanctification", "healer"},
		{6, "Death Knight", "Unholy", "Dreadplate", "Darkruned", "strength"}, {6, "Death Knight", "Blood", "", "Darkruned", "tank"},
		{7, "Shaman", "Enhancement", "Linked", "Worldbreaker", "agility"}, {7, "Shaman", "Elemental", "Mail", "Worldbreaker", "caster"}, {7, "Shaman", "Restoration", "Ringmail", "Worldbreaker", "healer"},
		{8, "Mage", "Frost", "Silk", "Kirin Tor", "caster"}, {9, "Warlock", "Affliction", "Felweave", "Deathbringer", "caster"},
		{11, "Druid", "Feral", "Dragonhide", "Nightsong", "agility"}, {11, "Druid", "Balance", "Wyrmhide", "Nightsong", "caster"}, {11, "Druid", "Restoration", "Kodohide", "Nightsong", "healer"},
	}
	kits := map[string][]string{"strength": {"20 × primary-stat gems + meta", "DPS head, shoulder & leg enhancements", "Weapon, cloak, chest, wrist, glove & boot enchants"}, "agility": {"20 × primary-stat gems + meta", "Agility head, shoulder & leg enhancements", "Weapon, cloak, chest, wrist, glove & boot enchants"}, "caster": {"20 × spell-power gems + meta", "Caster head, shoulder & leg enhancements", "Weapon, cloak, chest, wrist, glove & boot enchants"}, "healer": {"20 × healing gems + meta", "Healer head, shoulder & leg enhancements", "Weapon, cloak, chest, wrist, glove & boot enchants"}, "tank": {"20 × stamina gems + meta", "Tank head, shoulder & leg enhancements", "Weapon, cloak, chest, wrist, glove & boot enchants"}}
	id := uint32(10)
	for _, set := range sets {
		for _, season := range []struct {
			Name  string
			Price uint32
		}{{"S6", 110}, {"S7", 155}} {
			if set.PvPArmor == "" {
				continue
			}
			gladiator := map[string]string{"S6": "Furious", "S7": "Relentless"}[season.Name] + " Gladiator's " + set.PvPArmor
			includes := []string{"Complete 5-piece " + gladiator + " set", "Matching neck, cloak, wrists, belt & boots", "2 rings and 2 PvP trinkets", "Alliance/Horde Medallion selected at checkout", "Spec weapon set + ranged weapon or relic"}
			includes = append(includes, kits[set.Role]...)
			includes = append(includes, "Level 80 service")
			products = append(products, product{ID: id, Price: season.Price, Name: set.Class + " " + set.Spec + " " + season.Name + " Package", Description: "WotLK " + gladiator + " starter loadout for " + set.Spec + ".", Category: "PvP", ClassID: set.ID, ClassName: set.Class, Tier: season.Name, ServiceLevel: 80, Includes: includes})
			id++
		}
		includes := []string{"Complete 5-piece Conqueror's " + set.T8Set + " set", "Ulduar neck, cloak, wrists, belt & boots", "2 Ulduar rings and 2 trinkets", "Spec weapon set + ranged weapon or relic"}
		includes = append(includes, kits[set.Role]...)
		includes = append(includes, "Level 80 service")
		products = append(products, product{ID: id, Price: 135, Name: set.Class + " " + set.Spec + " T8 Package", Description: "Ulduar raid-ready WotLK tier 8 loadout for " + set.Spec + ".", Category: "PvE", ClassID: set.ID, ClassName: set.Class, Tier: "T8", ServiceLevel: 80, Includes: includes})
		id++
	}
	products = append(products,
		product{ID: id, Price: 20, Name: "5,000 Gold", Description: "A starter gold package delivered safely by in-game mail.", Category: "Gold", Tier: "5K", Gold: 5000, Includes: []string{"5,000 in-game gold", "Mailbox delivery", "Any class"}},
		product{ID: id + 1, Price: 65, Name: "20,000 Gold", Description: "Enough gold for professions, consumables, and raid preparation.", Category: "Gold", Tier: "20K", Gold: 20000, Includes: []string{"20,000 in-game gold", "Mailbox delivery", "Any class"}},
		product{ID: id + 2, Price: 140, Name: "50,000 Gold", Description: "A treasury package for established adventurers.", Category: "Gold", Tier: "50K", Gold: 50000, Includes: []string{"50,000 in-game gold", "Mailbox delivery", "Any class"}},
	)
	for i := range products {
		products[i].Active = true
		products[i].CategoryOrder = map[string]int{"Featured": -10, "PvP": 10, "PvE": 20, "Mounts": 30, "Weapons": 40, "Gold": 50, "Services": 60, "Utility": 70, "Companions": 80}[products[i].Category]
		if products[i].ID == 1 {
			products[i].Featured = true
			products[i].SalePrice = 95
			products[i].StockLimit = 10
			products[i].SoldCount = 7
		}
	}
	return products
}

func (s *Server) mockHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/setup/status", s.setupStatus)
	m.HandleFunc("POST /api/setup", s.rate(5, time.Hour, s.setup))
	m.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := s.runtimeSettings(r)
		active, message := s.maintenanceActive(r)
		jsonOut(w, 200, map[string]any{"online": true, "realm": cfg.RealmName, "address": cfg.RealmAddress, "maintenance": active, "maintenanceMessage": message, "checkedAt": time.Now(), "demo": true})
	})
	m.HandleFunc("POST /api/auth/register", s.feature(s.c.EnableRegistration, "Registration", s.mockRegister))
	m.HandleFunc("POST /api/auth/login", s.mockLogin)
	m.HandleFunc("POST /api/auth/logout", s.mockLogout)
	m.HandleFunc("POST /api/auth/password/request", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"message": "Demo recovery request accepted. No email was sent."})
	})
	m.HandleFunc("POST /api/auth/password/reset", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, map[string]bool{"ok": true}) })
	m.HandleFunc("POST /api/auth/email/verify", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"ok": true, "message": "Demo email verified."})
	})
	m.HandleFunc("POST /api/auth/email/resend", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"message": "If that address is awaiting verification, a new link has been sent."})
	})
	m.HandleFunc("GET /api/public-config", s.publicConfig)
	m.HandleFunc("GET /api/me", s.mockMe)
	m.HandleFunc("GET /api/characters", s.mockOwnCharacters)
	m.HandleFunc("GET /api/armory", s.feature(s.c.EnableArmory, "Armory", s.mockArmory))
	m.HandleFunc("GET /api/armory/{name}", s.feature(s.c.EnableArmory, "Armory", s.mockCharacter))
	m.HandleFunc("GET /api/armory/{name}/insights", s.feature(s.c.EnableArmory, "Armory", s.armoryInsights))
	m.HandleFunc("GET /api/arena", s.feature(s.c.EnableRankings, "Rankings", s.mockArena))
	m.HandleFunc("GET /api/rankings", s.feature(s.c.EnableRankings, "Rankings", s.mockExpandedRankings))
	m.HandleFunc("GET /api/rankings/raids", s.feature(s.c.EnableRankings, "Rankings", s.raidRankings))
	m.HandleFunc("GET /api/progression/{name}", s.feature(s.c.EnableArmory, "Armory", s.mockProgression))
	m.HandleFunc("GET /api/realm", s.feature(s.c.EnableRealmStatus, "Realm status", s.mockRealm))
	m.HandleFunc("GET /api/guilds", s.feature(s.c.EnableGuilds, "Guilds", s.mockGuilds))
	m.HandleFunc("GET /api/guilds/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.mockGuild))
	m.HandleFunc("GET /api/shop", s.feature(s.c.EnableShop, "Shop", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()
		out := []product{}
		s.mock.mu.Lock()
		for _, p := range s.mock.products {
			if p.Active && (p.StartsAt == nil || !now.Before(*p.StartsAt)) && (p.EndsAt == nil || now.Before(*p.EndsAt)) {
				out = append(out, p)
			}
		}
		s.mock.mu.Unlock()
		jsonOut(w, 200, map[string]any{"products": out, "deliveryEnabled": true})
	}))
	m.HandleFunc("POST /api/shop/purchase", s.feature(s.c.EnableShop, "Shop", s.mockPurchase))
	m.HandleFunc("GET /api/characters/deleted", s.mockDeletedCharacters)
	m.HandleFunc("POST /api/characters/{guid}/service", s.mockCharacterService)
	m.HandleFunc("GET /api/orders", s.mockOrders)
	m.HandleFunc("GET /api/dashboard", s.dashboard)
	m.HandleFunc("POST /api/rewards/daily", s.rate(3, time.Hour, s.claimDailyReward))
	m.HandleFunc("POST /api/rewards/vote/callback", s.rate(60, time.Minute, s.voteRewardCallback))
	m.HandleFunc("GET /api/tickets", s.feature(s.c.EnableSupport, "Support", s.mockTickets))
	m.HandleFunc("POST /api/tickets", s.feature(s.c.EnableSupport, "Support", s.mockCreateTicket))
	m.HandleFunc("POST /api/admin/credits", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.mockCredits)))
	m.HandleFunc("GET /api/admin/orders", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrders))
	m.HandleFunc("GET /api/admin/status", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminStatus))
	m.HandleFunc("GET /api/admin/analytics", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAnalytics))
	m.HandleFunc("GET /api/admin/ledger", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"entries": []map[string]any{{"id": 1, "Actor": "DEMO", "Target": "DEMO", "Amount": 500, "Reason": "Demo starting balance", "Created": time.Now().Add(-48 * time.Hour)}}})
	}))
	m.HandleFunc("GET /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) {
		s.mock.mu.Lock()
		out := append([]product(nil), s.mock.products...)
		s.mock.mu.Unlock()
		jsonOut(w, 200, map[string]any{"products": out})
	}))
	m.HandleFunc("GET /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDetail))
	m.HandleFunc("GET /api/admin/items", s.feature(s.c.EnableAdminPanel, "Administration", s.adminItemSearch))
	m.HandleFunc("GET /api/admin/accounts", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminAccounts))
	m.HandleFunc("POST /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.mockAdminModeration)))
	m.HandleFunc("GET /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminModerationLog))
	m.HandleFunc("GET /api/admin/audit", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAudit))
	m.HandleFunc("GET /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.mockAdminConsoleHistory))
	m.HandleFunc("POST /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.rate(20, time.Minute, s.mockAdminConsoleExecute)))
	m.HandleFunc("GET /api/admin/tickets", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.mockAdminTickets))
	m.HandleFunc("POST /api/admin/tickets/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.mockAdminTicketUpdate))
	m.HandleFunc("POST /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProduct))
	m.HandleFunc("PUT /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProductUpdate))
	m.HandleFunc("DELETE /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProductDelete))
	m.HandleFunc("GET /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("POST /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("DELETE /api/admin/coupons/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponDelete))
	m.HandleFunc("GET /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("POST /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("PUT /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("DELETE /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("GET /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("PUT /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("POST /api/admin/orders/{id}/retry", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrderAction))
	m.HandleFunc("POST /api/admin/orders/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrderAction))
	m.HandleFunc("POST /api/billing/checkout", s.feature(s.c.EnableShop, "Shop", s.mockBillingCheckout))
	m.HandleFunc("GET /api/security/sessions", s.mockSecuritySessions)
	m.HandleFunc("DELETE /api/security/sessions/{id}", s.mockSecurityRevoke)
	m.HandleFunc("POST /api/security/password", s.mockSecurityPassword)
	m.HandleFunc("POST /api/security/totp/setup", s.mockTOTPSetup)
	m.HandleFunc("POST /api/security/totp/enable", s.mockTOTPEnable)
	m.HandleFunc("POST /api/security/totp/disable", s.mockTOTPDisable)
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /readyz", s.ready)
	m.HandleFunc("GET /metrics", s.prometheusMetrics)
	m.Handle("/", spaHandler(s.static))
	return m
}

func (s *Server) mockRegister(w http.ResponseWriter, r *http.Request) {
	if s.c.EnableSetup {
		complete, err := s.isSetupComplete(r)
		if err != nil || !complete {
			problem(w, http.StatusServiceUnavailable, "Complete first-time setup before registering accounts")
			return
		}
	}
	var in struct{ Username, Password, Email, TurnstileToken string }
	if !decode(w, r, &in) {
		return
	}
	if len(in.Username) < 3 || len(in.Password) < 8 {
		problem(w, 422, "Use at least 3 username and 8 password characters")
		return
	}
	s.mock.mu.Lock()
	s.mock.users[strings.ToUpper(in.Username)] = in.Password
	s.mock.mu.Unlock()
	jsonOut(w, 201, map[string]any{"ok": true, "verificationRequired": s.c.RequireEmailVerification, "message": "Demo account created."})
}
func (s *Server) mockLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password, OTP string }
	if !decode(w, r, &in) {
		return
	}
	username := strings.ToUpper(in.Username)
	s.mock.mu.Lock()
	password, ok := s.mock.users[username]
	s.mock.mu.Unlock()
	if !ok || password != in.Password {
		problem(w, 401, "Invalid username or password. Try DEMO / demo1234")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_demo", Value: username, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]any{"account": account{ID: 1, Username: username, Email: "demo@example.com"}})
}
func (s *Server) mockUser(r *http.Request) (string, bool) {
	c, e := r.Cookie("portal_demo")
	if e != nil {
		return "", false
	}
	return c.Value, true
}
func (s *Server) mockLogout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "portal_demo", Path: "/", MaxAge: -1, HttpOnly: true})
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockMe(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mockUser(r)
	if !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	balance := s.mock.balance
	s.mock.mu.Unlock()
	a := account{ID: 1, Username: u, Email: "demo@example.com", GMLevel: 3}
	jsonOut(w, 200, map[string]any{"account": a, "balance": balance, "staffRole": staffRole(a, s.c), "permissions": s.staffPermissionsFor(a.GMLevel, a.Username)})
}
func (s *Server) mockOwnCharacters(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	jsonOut(w, 200, map[string]any{"characters": mockCharacters})
}
func (s *Server) mockArmory(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	out := []character{}
	for _, c := range mockCharacters {
		if strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}
func (s *Server) mockCharacter(w http.ResponseWriter, r *http.Request) {
	for _, c := range mockCharacters {
		if strings.EqualFold(c.Name, r.PathValue("name")) {
			items := []map[string]any{
				{"slot": 0, "entry": 51272, "name": "Sanctified Lightsworn Headpiece", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2245, "icon": "inv_helmet_96", "stats": []string{"+124 Strength", "+172 Stamina", "Meta Socket · Red Socket"}},
				{"slot": 1, "entry": 50763, "name": "Marrowgar's Scratching Choker", "quality": 4, "itemLevel": 264, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_necklace_53", "stats": []string{"+90 Strength", "+90 Stamina", "+60 Critical Strike"}},
				{"slot": 2, "entry": 51274, "name": "Sanctified Lightsworn Shoulderplates", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2072, "icon": "inv_shoulder_117", "stats": []string{"+108 Strength", "+146 Stamina", "+82 Haste"}},
				{"slot": 3, "entry": 43348, "name": "Tabard of the Explorer", "quality": 3, "itemLevel": 1, "requiredLevel": 1, "armor": 0, "icon": "inv_shirt_guildtabard_01", "stats": []string{"Soulbound"}},
				{"slot": 4, "entry": 51270, "name": "Sanctified Lightsworn Battleplate", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2760, "icon": "inv_chest_plate_26", "stats": []string{"+148 Strength", "+196 Stamina", "Red Socket · Blue Socket"}},
				{"slot": 5, "entry": 50620, "name": "Coldwraith Links", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1210, "icon": "inv_belt_60", "stats": []string{"+104 Strength", "+139 Stamina", "+80 Expertise"}},
				{"slot": 6, "entry": 51271, "name": "Sanctified Lightsworn Legplates", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2417, "icon": "inv_pants_plate_35", "stats": []string{"+148 Strength", "+196 Stamina", "+92 Critical Strike"}},
				{"slot": 7, "entry": 50639, "name": "Blood-Soaked Saronite Stompers", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1478, "icon": "inv_boots_plate_06", "stats": []string{"+104 Strength", "+139 Stamina", "+72 Haste"}},
				{"slot": 8, "entry": 50659, "name": "Polar Bear Claw Bracers", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 975, "icon": "inv_bracer_45", "stats": []string{"+80 Strength", "+108 Stamina", "+56 Critical Strike"}},
				{"slot": 9, "entry": 51269, "name": "Sanctified Lightsworn Gauntlets", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1726, "icon": "inv_gauntlets_92", "stats": []string{"+108 Strength", "+146 Stamina", "+74 Haste"}},
				{"slot": 10, "entry": 50402, "name": "Ashen Band of Endless Might", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_ring_84", "stats": []string{"+99 Strength", "+107 Stamina", "+59 Critical Strike"}},
				{"slot": 11, "entry": 50693, "name": "Might of Blight", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_ring_83", "stats": []string{"+99 Strength", "+107 Stamina", "+59 Haste"}},
				{"slot": 12, "entry": 50363, "name": "Deathbringer's Will", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_trinket_04", "stats": []string{"+167 Armor Penetration", "Chance on hit: transform your power"}},
				{"slot": 13, "entry": 54590, "name": "Sharpened Twilight Scale", "quality": 4, "itemLevel": 284, "requiredLevel": 80, "armor": 0, "icon": "inv_misc_monsterscales_15", "stats": []string{"+184 Armor Penetration", "Chance on hit: +1472 Attack Power"}},
				{"slot": 14, "entry": 50677, "name": "Winding Sheet", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 183, "icon": "inv_misc_cape_16", "stats": []string{"+90 Strength", "+90 Stamina", "+60 Haste"}},
				{"slot": 15, "entry": 50730, "name": "Glorenzelg, High-Blade of the Silver Hand", "quality": 4, "itemLevel": 271, "requiredLevel": 80, "armor": 0, "icon": "inv_sword_140", "stats": []string{"Two-Hand Sword", "954–1,432 Damage · Speed 3.60", "+164 Strength · +183 Stamina"}},
				{"slot": 18, "entry": 50455, "name": "Warsong Gulch Mark of Honor", "quality": 3, "itemLevel": 80, "requiredLevel": 80, "armor": 0, "icon": "inv_bannerpvp_02", "stats": []string{"Relic"}},
			}
			personalArena := []characterArenaTeam{}
			if c.Name == "Arthoria" {
				personalArena = []characterArenaTeam{{ID: 1, Rank: 1, Name: "Relentless", Bracket: 2, Rating: 2478, SeasonGames: 184, SeasonWins: 137, PersonalRating: 2491, PersonalGames: 172, PersonalWins: 129}, {ID: 4, Rank: 4, Name: "Ice Block Heroes", Bracket: 3, Rating: 2155, SeasonGames: 128, SeasonWins: 82, PersonalRating: 2142, PersonalGames: 119, PersonalWins: 75}}
			}
			jsonOut(w, 200, map[string]any{"character": c, "equipment": items, "profile": characterProfile{Achievements: 164, Exalted: 12, TalentSpecs: 2, TalentSpells: 44, Glyphs: 6, Professions: []profession{{ID: 164, Name: "Blacksmithing", Value: 450, Maximum: 450}, {ID: 186, Name: "Mining", Value: 450, Maximum: 450}}}, "arenaTeams": personalArena})
			return
		}
	}
	problem(w, 404, "Character not found")
}

func (s *Server) mockArena(w http.ResponseWriter, r *http.Request) {
	bracket := 2
	if r.URL.Query().Get("bracket") == "3" {
		bracket = 3
	} else if r.URL.Query().Get("bracket") == "5" {
		bracket = 5
	}
	teams := []arenaTeam{{ID: 1, Rank: 1, Name: "Relentless", Bracket: uint8(bracket), Rating: 2478, SeasonGames: 184, SeasonWins: 137, Members: []arenaMember{{"Arthoria", 2, 2491, 172, 129}, {"Velistra", 8, 2465, 166, 122}}}, {ID: 2, Rank: 2, Name: "No Trinket Needed", Bracket: uint8(bracket), Rating: 2396, SeasonGames: 201, SeasonWins: 143, Members: []arenaMember{{"Grimward", 6, 2410, 190, 136}, {"Quickarrow", 3, 2382, 181, 127}}}, {ID: 3, Rank: 3, Name: "Mana Burn Society", Bracket: uint8(bracket), Rating: 2284, SeasonGames: 156, SeasonWins: 104, Members: []arenaMember{{"Emberhex", 9, 2301, 149, 101}, {"Thornhoof", 11, 2267, 144, 96}}}, {ID: 4, Rank: 4, Name: "Ice Block Heroes", Bracket: uint8(bracket), Rating: 2155, SeasonGames: 128, SeasonWins: 82, Members: []arenaMember{{"Velistra", 8, 2168, 122, 79}, {"Arthoria", 2, 2142, 119, 75}}}}
	jsonOut(w, 200, map[string]any{"bracket": bracket, "teams": teams})
}

func (s *Server) mockProgression(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	found := false
	guild := ""
	for _, c := range mockCharacters {
		if strings.EqualFold(c.Name, name) {
			found = true
			name = c.Name
			guild = c.Guild
		}
	}
	if !found {
		problem(w, 404, "Character not found")
		return
	}
	base := time.Date(2026, 1, 10, 20, 15, 0, 0, time.UTC)
	out := []map[string]any{}
	for i, d := range progressDefinitions {
		entry := map[string]any{"achievement": d.Achievement, "raid": d.Raid, "section": d.Section, "difficulty": d.Difficulty, "bosses": d.Bosses}
		completed := d.Raid == "Naxxramas" || d.Raid == "Ulduar" || (d.Raid == "Icecrown Citadel" && d.Difficulty == "10 player" && i < 14)
		if completed {
			entry["characterDate"] = uint64(base.Add(time.Duration(i) * 9 * 24 * time.Hour).Unix())
			entry["guildDate"] = uint64(base.Add(time.Duration(i) * 8 * 24 * time.Hour).Unix())
		}
		out = append(out, entry)
	}
	jsonOut(w, 200, map[string]any{"character": name, "guild": guild, "progression": out, "source": "achievement timestamps"})
}
func (s *Server) mockPurchase(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		ProductID, CharacterGUID uint32
		Coupon                   string `json:"coupon"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	var p *product
	for i := range s.mock.products {
		if s.mock.products[i].ID == in.ProductID {
			p = &s.mock.products[i]
		}
	}
	if p == nil {
		problem(w, 404, "Product not found")
		return
	}
	characterClass := uint8(0)
	characterOnline := false
	for _, c := range mockCharacters {
		if c.GUID == in.CharacterGUID {
			characterClass = c.Class
			characterOnline = c.Online
		}
	}
	if characterClass == 0 {
		problem(w, 422, "Choose one of your characters")
		return
	}
	if characterOnline {
		problem(w, 409, "Character must be offline for delivery")
		return
	}
	if p.ClassID != 0 && p.ClassID != characterClass {
		problem(w, 422, "This package does not match the selected character's class")
		return
	}
	if p.PerAccountLimit > 0 && s.mock.purchases[p.ID] >= p.PerAccountLimit {
		problem(w, 409, "This product's account purchase limit has been reached")
		return
	}
	if p.StockLimit > 0 && p.SoldCount >= p.StockLimit {
		problem(w, 409, "This product is sold out")
		return
	}
	total := p.Price
	if p.SalePrice > 0 && p.SalePrice < total {
		total = p.SalePrice
	}
	code := strings.ToUpper(strings.TrimSpace(in.Coupon))
	couponApplied := false
	if code != "" {
		var found *coupon
		for i := range s.mock.coupons {
			if s.mock.coupons[i].Code == code && s.mock.coupons[i].Active {
				found = &s.mock.coupons[i]
				break
			}
		}
		if found == nil || (found.PerAccountLimit > 0 && s.mock.couponUses[code] >= found.PerAccountLimit) {
			problem(w, 422, "Coupon is invalid or already used")
			return
		}
		discount := uint32(uint64(total)*uint64(found.DiscountPercent)/100) + found.DiscountCredits
		if discount > total {
			discount = total
		}
		total -= discount
		couponApplied = true
	}
	if s.mock.balance < total {
		problem(w, 422, "Not enough credits")
		return
	}
	s.mock.balance -= total
	s.mock.purchases[p.ID]++
	if p.StockLimit > 0 {
		p.SoldCount++
	}
	if couponApplied {
		s.mock.couponUses[code]++
	}
	id := 1043 + len(s.mock.orders)
	s.mock.orders = append([]map[string]any{{"id": id, "itemId": p.ItemID, "quantity": p.Quantity, "total": total, "coupon": code, "status": "delivered", "created": time.Now()}}, s.mock.orders...)
	jsonOut(w, 201, map[string]any{"ok": true, "orderId": id, "message": "Demo delivery complete — no real game data was changed."})
}
func (s *Server) mockOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	orders := append([]map[string]any(nil), s.mock.orders...)
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"orders": orders})
}

func (s *Server) mockAdminStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "monitoring"); !ok {
		problem(w, http.StatusForbidden, "GM access required")
		return
	}
	cfg := s.runtimeSettings(r)
	active, message := s.maintenanceActive(r)
	jsonOut(w, http.StatusOK, map[string]any{"online": true, "realm": cfg.RealmName, "address": cfg.RealmAddress, "shopDelivery": true, "portal": true, "database": true, "soapConfigured": true, "maintenance": active, "maintenanceMessage": message, "checkedAt": time.Now(), "demo": true})
}

func (s *Server) mockTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	tickets := append([]supportTicket(nil), s.mock.tickets...)
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"tickets": tickets})
}

func (s *Server) mockCreateTicket(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		CharacterGUID    uint32
		Subject, Message string
	}
	if !decode(w, r, &in) {
		return
	}
	in.Subject, in.Message = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Message)
	if len(in.Subject) < 3 || len(in.Message) < 10 {
		problem(w, 422, "Enter a subject and detailed message")
		return
	}
	s.mock.mu.Lock()
	id := uint64(len(s.mock.tickets) + 1)
	now := time.Now()
	s.mock.tickets = append([]supportTicket{{ID: id, AccountID: 1, Username: "DEMO", CharacterGUID: in.CharacterGUID, Subject: in.Subject, Message: in.Message, Status: "open", Created: now, Updated: now}}, s.mock.tickets...)
	s.mock.mu.Unlock()
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) mockAdminTickets(w http.ResponseWriter, r *http.Request) { s.mockTickets(w, r) }

func (s *Server) mockAdminTicketUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var in struct{ Status, Response string }
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for i := range s.mock.tickets {
		if s.mock.tickets[i].ID == id {
			s.mock.tickets[i].Status = in.Status
			s.mock.tickets[i].Response = in.Response
			s.mock.tickets[i].GM = "DEMO"
			s.mock.tickets[i].Updated = time.Now()
			jsonOut(w, 200, map[string]any{"ok": true})
			return
		}
	}
	problem(w, 404, "Ticket not found")
}

func (s *Server) mockCredits(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Username string `json:"username"`
		Amount   uint32 `json:"amount"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Amount == 0 || in.Amount > 1000000 || len(strings.TrimSpace(in.Reason)) < 3 {
		problem(w, 422, "Enter an account, amount, and reason")
		return
	}
	s.mock.mu.Lock()
	s.mock.balance += in.Amount
	balance := s.mock.balance
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"ok": true, "username": strings.ToUpper(in.Username), "amount": in.Amount, "balance": balance})
}

func (s *Server) mockAdminAccounts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	q := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("q")))
	type mockAccount struct {
		ID         uint32      `json:"id"`
		Username   string      `json:"username"`
		Email      string      `json:"email"`
		Locked     bool        `json:"locked"`
		Banned     bool        `json:"banned"`
		BanUntil   uint64      `json:"banUntil"`
		BanReason  string      `json:"banReason"`
		Characters []character `json:"characters"`
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	accounts := []mockAccount{}
	for _, a := range []mockAccount{{1, "DEMO", "demo@example.com", false, false, 0, "", mockCharacters[:3]}, {2, "FROSTBYTE", "frost@example.com", false, false, 0, "", mockCharacters[3:6]}} {
		if q != "" && !strings.Contains(a.Username, q) && !strings.Contains(strings.ToUpper(a.Email), q) {
			continue
		}
		if reason, banned := s.mock.bans[a.Username]; banned {
			a.Banned, a.BanReason, a.BanUntil = true, reason, uint64(time.Now().Add(7*24*time.Hour).Unix())
		}
		accounts = append(accounts, a)
	}
	jsonOut(w, 200, map[string]any{"accounts": accounts})
}

func (s *Server) mockAdminModeration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	var in struct {
		Action, Target, Duration, Reason string
		Level, RealmID                   int
	}
	if !decode(w, r, &in) {
		return
	}
	in.Action, in.Target, in.Reason = strings.ToLower(strings.TrimSpace(in.Action)), strings.ToUpper(strings.TrimSpace(in.Target)), strings.TrimSpace(in.Reason)
	allowed := map[string]bool{"ban": true, "unban": true, "kick": true, "mute": true, "unmute": true, "ip_ban": true, "ip_unban": true, "gm_level": true, "announce": true, "motd": true, "start": true, "restart": true, "shutdown": true, "cancel_shutdown": true}
	if !allowed[in.Action] {
		problem(w, 422, "Unsupported moderation action")
		return
	}
	if len(in.Reason) < 3 {
		problem(w, 422, "Reason is required")
		return
	}
	if in.Action == "start" || in.Action == "restart" || in.Action == "shutdown" || in.Action == "cancel_shutdown" || in.Action == "announce" || in.Action == "motd" {
		in.Target = "realm"
	}
	s.mock.mu.Lock()
	if in.Action == "ban" {
		s.mock.bans[in.Target] = in.Reason
	}
	if in.Action == "unban" {
		delete(s.mock.bans, in.Target)
	}
	s.mock.moderation = append([]map[string]any{{"id": len(s.mock.moderation) + 1, "Actor": "DEMO", "Target": in.Target, "Action": in.Action, "Duration": in.Duration, "Reason": in.Reason, "Status": "executed", "Created": time.Now()}}, s.mock.moderation...)
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"ok": true, "action": in.Action, "target": in.Target})
}

func (s *Server) mockAdminModerationLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	s.mock.mu.Lock()
	entries := append([]map[string]any(nil), s.mock.moderation...)
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"entries": entries})
}

func (s *Server) mockAdminConsoleExecute(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusForbidden, "GM console access required")
		return
	}
	var in struct {
		Command string `json:"command"`
	}
	if !decode(w, r, &in) {
		return
	}
	command, valid := normalizeConsoleCommand(in.Command)
	if !valid {
		problem(w, http.StatusUnprocessableEntity, "Command must contain 1–255 characters on one line")
		return
	}
	if !consoleCommandAllowed(command, s.c.GMConsoleAllowAll, s.c.GMConsoleAllowed) {
		problem(w, http.StatusForbidden, "Command is not included in GM_CONSOLE_ALLOWED_PREFIXES")
		return
	}
	response := "AzerothCore demo console: " + command + " completed successfully."
	s.mock.mu.Lock()
	id := uint64(len(s.mock.commands) + 1)
	entry := consoleEntry{ID: id, Actor: "DEMO", Command: auditConsoleCommand(command), Response: response, Success: true, IP: s.clientIP(r), Created: time.Now()}
	s.mock.commands = append([]consoleEntry{entry}, s.mock.commands...)
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "command": command, "output": response, "auditId": id})
}

func (s *Server) mockAdminConsoleHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusForbidden, "GM console access required")
		return
	}
	s.mock.mu.Lock()
	entries := append([]consoleEntry(nil), s.mock.commands...)
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]any{"entries": entries, "allowAll": s.c.GMConsoleAllowAll, "allowedPrefixes": s.c.GMConsoleAllowed})
}

func (s *Server) mockRealm(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, 200, map[string]any{"name": s.c.RealmName, "address": s.c.RealmAddress, "port": 8085, "population": 1.2, "characters": 18472, "online": 846, "allianceOnline": 431, "hordeOnline": 415, "uptime": 1209480, "recordOnline": 1732})
}

func mockGuildData() []map[string]any {
	return []map[string]any{{"id": 1, "name": "Keepers of Dawn", "leader": "Arthoria", "members": 42, "averageLevel": 78.4, "online": 14}, {"id": 2, "name": "Ashen Vanguard", "leader": "Grimward", "members": 36, "averageLevel": 76.9, "online": 9}, {"id": 3, "name": "Silver Covenant", "leader": "Velistra", "members": 29, "averageLevel": 73.2, "online": 6}}
}

func (s *Server) mockGuilds(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, 200, map[string]any{"guilds": mockGuildData()})
}
func (s *Server) mockGuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var guild map[string]any
	for _, g := range mockGuildData() {
		if id == fmt.Sprint(g["id"]) {
			guild = g
		}
	}
	if guild == nil {
		problem(w, 404, "Guild not found")
		return
	}
	members := []map[string]any{}
	for _, c := range mockCharacters {
		if c.Guild == guild["name"] {
			members = append(members, map[string]any{"guid": c.GUID, "name": c.Name, "race": c.Race, "class": c.Class, "gender": c.Gender, "level": c.Level, "zone": c.Zone, "online": c.Online, "totalTime": c.TotalTime, "rank": map[bool]string{true: "Guild Master", false: "Raider"}[c.Name == guild["leader"]]})
		}
	}
	guild["motd"] = "Strength through fellowship. Raid nights Thursday and Sunday."
	guild["info"] = "A progression guild welcoming committed adventurers across Northrend."
	jsonOut(w, 200, map[string]any{"guild": guild, "members": members})
}

func (s *Server) mockBillingCheckout(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Package string `json:"package"`
	}
	if !decode(w, r, &in) {
		return
	}
	credits := map[string]uint32{"small": 100, "medium": 550, "large": 1200}[in.Package]
	if credits == 0 {
		problem(w, 422, "Unknown credit package")
		return
	}
	s.mock.mu.Lock()
	s.mock.balance += credits
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]string{"url": "/account?payment=success&demo=1"})
}
func (s *Server) mockSecuritySessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	now := time.Now()
	jsonOut(w, 200, map[string]any{"sessions": []map[string]any{{"id": "demo-current", "Created": now.Add(-2 * time.Hour), "LastSeen": now, "Expires": now.Add(7 * 24 * time.Hour), "IP": "127.0.0.1", "UserAgent": "Demo browser", "Current": true}}})
}
func (s *Server) mockSecurityRevoke(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockSecurityPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mockUser(r)
	if !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	if s.mock.users[u] != in.CurrentPassword || len(in.NewPassword) < 8 {
		problem(w, 422, "Check the current password and use at least 8 characters")
		return
	}
	s.mock.users[u] = in.NewPassword
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockTOTPSetup(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mockUser(r)
	if !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	s.mock.totpSecret = "JBSWY3DPEHPK3PXP"
	s.mock.totpEnabled = false
	s.mock.mu.Unlock()
	issuer := s.c.PortalName
	jsonOut(w, 200, map[string]string{"secret": "JBSWY3DPEHPK3PXP", "uri": "otpauth://totp/" + url.PathEscape(issuer+":"+u) + "?secret=JBSWY3DPEHPK3PXP&issuer=" + url.QueryEscape(issuer)})
}
func (s *Server) mockTOTPEnable(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	if !validTOTP(s.mock.totpSecret, in.Code, time.Now()) {
		problem(w, 422, "Invalid authenticator code")
		return
	}
	s.mock.totpEnabled = true
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockTOTPDisable(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	s.mock.totpEnabled = false
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockAdminOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"orders": s.mock.orders})
}
func (s *Server) mockAdminOrderAction(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) mockAdminProduct(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var p product
	if !decode(w, r, &p) {
		return
	}
	if err := validateManagedProduct(p); err != nil {
		problem(w, 422, err.Error())
		return
	}
	s.mock.mu.Lock()
	p.ID = uint32(len(s.mock.products) + 1)
	p.Active = true
	s.mock.products = append(s.mock.products, p)
	s.mock.mu.Unlock()
	jsonOut(w, 201, map[string]any{"id": p.ID})
}
