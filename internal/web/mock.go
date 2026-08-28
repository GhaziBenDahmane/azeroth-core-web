package web

import (
	"fmt"
	"net/http"
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
}

func newMockState() *mockState {
	return &mockState{balance: 500, users: map[string]string{"DEMO": "demo1234"}, orders: []map[string]any{{"id": 1042, "itemId": 49623, "quantity": 1, "total": 85, "status": "delivered", "created": time.Now().Add(-24 * time.Hour)}}}
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

var mockProducts = buildMockProducts()

func buildMockProducts() []product {
	products := []product{
		{ID: 1, ItemID: 49284, Quantity: 1, Price: 120, Name: "Swift Spectral Tiger", Description: "A rare spectral mount delivered directly through in-game mail.", Category: "Mounts"},
		{ID: 2, ItemID: 49623, Quantity: 1, Price: 85, Name: "Shadowmourne", Description: "A legendary two-handed axe for the realm's mightiest champions.", Category: "Weapons"},
		{ID: 3, ItemID: 51809, Quantity: 1, Price: 45, Name: "Portable Hole", Description: "A spacious 24-slot bag for long expeditions across Northrend.", Category: "Utility"},
		{ID: 4, ItemID: 23713, Quantity: 1, Price: 60, Name: "Hippogryph Hatchling", Description: "A loyal companion from the forests of Feralas.", Category: "Companions"},
		{ID: 5, ItemID: 37719, Quantity: 5, Price: 15, Name: "Adventurer Supply Bundle", Description: "Useful supplies for your next adventure.", Category: "Utility"},
		{ID: 6, ItemID: 50818, Quantity: 1, Price: 75, Name: "Invincible's Reins", Description: "The famed steed of the fallen prince awaits a new rider.", Category: "Mounts"},
		{ID: 7, Price: 40, Name: "Instant Level 80", Description: "Raise one existing character to level 80 and receive a starter travel kit.", Category: "Services", Tier: "Level 80", ServiceLevel: 80, Includes: []string{"Level 80 boost", "Four 20-slot bags", "Cold Weather Flying starter gold"}},
	}
	classes := []struct {
		ID   uint8
		Name string
	}{{1, "Warrior"}, {2, "Paladin"}, {3, "Hunter"}, {4, "Rogue"}, {5, "Priest"}, {6, "Death Knight"}, {7, "Shaman"}, {8, "Mage"}, {9, "Warlock"}, {11, "Druid"}}
	tiers := []struct {
		Name, Category, Description string
		Price                       uint32
	}{{"S6", "PvP", "Furious Gladiator loadout", 110}, {"S7", "PvP", "Relentless Gladiator loadout", 155}, {"T8", "PvE", "Ulduar raid-ready tier loadout", 135}}
	id := uint32(8)
	for _, class := range classes {
		for _, tier := range tiers {
			products = append(products, product{ID: id, Price: tier.Price, Name: class.Name + " " + tier.Name + " Package", Description: tier.Description + " selected for your class, with matching accessories and upgrades.", Category: tier.Category, ClassID: class.ID, ClassName: class.Name, Tier: tier.Name, ServiceLevel: 80, Includes: []string{"5-piece class armor set", "Class-appropriate weapon set", "Matching off-pieces and accessories", "Cut epic gem package", "Complete enchant scroll kit"}})
			id++
		}
	}
	products = append(products,
		product{ID: id, Price: 20, Name: "5,000 Gold", Description: "A starter gold package delivered safely by in-game mail.", Category: "Gold", Tier: "5K", Gold: 5000, Includes: []string{"5,000 in-game gold", "Mailbox delivery", "Any class"}},
		product{ID: id + 1, Price: 65, Name: "20,000 Gold", Description: "Enough gold for professions, consumables, and raid preparation.", Category: "Gold", Tier: "20K", Gold: 20000, Includes: []string{"20,000 in-game gold", "Mailbox delivery", "Any class"}},
		product{ID: id + 2, Price: 140, Name: "50,000 Gold", Description: "A treasury package for established adventurers.", Category: "Gold", Tier: "50K", Gold: 50000, Includes: []string{"50,000 in-game gold", "Mailbox delivery", "Any class"}},
	)
	return products
}

func (s *Server) mockHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"online": true, "realm": "Azeroth Demo", "address": "logon.demo.local", "shopDelivery": true, "demo": true})
	})
	m.HandleFunc("POST /api/auth/register", s.mockRegister)
	m.HandleFunc("POST /api/auth/login", s.mockLogin)
	m.HandleFunc("POST /api/auth/logout", s.mockLogout)
	m.HandleFunc("POST /api/auth/password/request", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"message": "Demo recovery request accepted. No email was sent."})
	})
	m.HandleFunc("POST /api/auth/password/reset", func(w http.ResponseWriter, r *http.Request) { jsonOut(w, 200, map[string]bool{"ok": true}) })
	m.HandleFunc("GET /api/public-config", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"turnstileSiteKey": "", "passwordResetEnabled": true})
	})
	m.HandleFunc("GET /api/me", s.mockMe)
	m.HandleFunc("GET /api/characters", s.mockOwnCharacters)
	m.HandleFunc("GET /api/armory", s.mockArmory)
	m.HandleFunc("GET /api/armory/{name}", s.mockCharacter)
	m.HandleFunc("GET /api/arena", s.mockArena)
	m.HandleFunc("GET /api/progression/{name}", s.mockProgression)
	m.HandleFunc("GET /api/realm", s.mockRealm)
	m.HandleFunc("GET /api/guilds", s.mockGuilds)
	m.HandleFunc("GET /api/guilds/{id}", s.mockGuild)
	m.HandleFunc("GET /api/shop", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"products": mockProducts, "deliveryEnabled": true})
	})
	m.HandleFunc("POST /api/shop/purchase", s.mockPurchase)
	m.HandleFunc("GET /api/orders", s.mockOrders)
	m.HandleFunc("POST /api/admin/credits", s.rate(30, time.Minute, s.mockCredits))
	m.HandleFunc("GET /api/admin/orders", s.mockAdminOrders)
	m.HandleFunc("GET /api/admin/ledger", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"entries": []map[string]any{{"id": 1, "Actor": "DEMO", "Target": "DEMO", "Amount": 500, "Reason": "Demo starting balance", "Created": time.Now().Add(-48 * time.Hour)}}})
	})
	m.HandleFunc("GET /api/admin/products", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"products": mockProducts})
	})
	m.HandleFunc("POST /api/admin/products", s.mockAdminProduct)
	m.HandleFunc("POST /api/admin/orders/{id}/retry", s.mockAdminOrderAction)
	m.HandleFunc("POST /api/admin/orders/{id}/refund", s.mockAdminOrderAction)
	m.HandleFunc("POST /api/billing/checkout", s.mockBillingCheckout)
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
	jsonOut(w, 201, map[string]any{"ok": true, "message": "Demo account created."})
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
	jsonOut(w, 200, map[string]any{"account": account{ID: 1, Username: u, Email: "demo@example.com", GMLevel: 3}, "balance": balance})
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
			jsonOut(w, 200, map[string]any{"character": c, "equipment": items, "profile": characterProfile{Achievements: 164, Exalted: 12, TalentSpecs: 2, TalentSpells: 44, Glyphs: 6, Professions: []profession{{ID: 164, Name: "Blacksmithing", Value: 450, Maximum: 450}, {ID: 186, Name: "Mining", Value: 450, Maximum: 450}}}})
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
	var in struct{ ProductID, CharacterGUID uint32 }
	if !decode(w, r, &in) {
		return
	}
	var p *product
	for i := range mockProducts {
		if mockProducts[i].ID == in.ProductID {
			p = &mockProducts[i]
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
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	if s.mock.balance < p.Price {
		problem(w, 422, "Not enough credits")
		return
	}
	s.mock.balance -= p.Price
	id := 1043 + len(s.mock.orders)
	s.mock.orders = append([]map[string]any{{"id": id, "itemId": p.ItemID, "quantity": p.Quantity, "total": p.Price, "status": "delivered", "created": time.Now()}}, s.mock.orders...)
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

func (s *Server) mockRealm(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, 200, map[string]any{"name": "Azeroth Demo", "address": "logon.demo.local", "port": 8085, "population": 1.2, "characters": 18472, "online": 846, "allianceOnline": 431, "hordeOnline": 415, "uptime": 1209480, "recordOnline": 1732})
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
	jsonOut(w, 200, map[string]string{"secret": "JBSWY3DPEHPK3PXP", "uri": "otpauth://totp/Azeroth%20Demo:" + u + "?secret=JBSWY3DPEHPK3PXP&issuer=Azeroth%20Demo"})
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
	if p.Name == "" || p.Price == 0 || p.ItemID == 0 || p.Quantity == 0 {
		problem(w, 422, "Name, price, item ID, and quantity are required")
		return
	}
	s.mock.mu.Lock()
	p.ID = uint32(len(mockProducts) + 1)
	mockProducts = append(mockProducts, p)
	s.mock.mu.Unlock()
	jsonOut(w, 201, map[string]any{"id": p.ID})
}
