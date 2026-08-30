package web

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type mockState struct {
	mu                 sync.Mutex
	balance            uint32
	orders             []map[string]any
	users              map[string]string
	totpSecret         string
	totpEnabled        bool
	totpRecovery       []string
	stepUpUntil        time.Time
	bans               map[string]string
	moderation         []map[string]any
	commands           []consoleEntry
	tickets            []supportTicket
	cannedReplies      []cannedReply
	pages              []contentPage
	events             []realmEvent
	eventRegistrations map[uint64]eventRegistration
	transfers          []transferRequest
	creditPackages     []creditPackage
	giftCodes          []giftCode
	giftCodeUses       map[string]bool
	setupDone          bool
	settings           siteSettings
	news               []newsEntry
	coupons            []coupon
	couponUses         map[string]uint32
	purchases          map[uint32]uint32
	deleted            []deletedCharacter
	dailyClaim         time.Time
	missionClaims      map[uint64]bool
	services           []dashboardService
	products           []product
	collections        []shopCollection
	stockMovements     []stockMovement
	voteEvents         map[string]bool
	ledger             []map[string]any
	notifications      []notification
	staff              []map[string]any
	downloads          []portalDownload
	launcherPatches    []launcherPatch
	privacy            []privacyRequest
	communityIssues    []communityIssue
	communityVotes     map[uint64]bool
	guildRecruitment   []guildRecruitment
	guildApplications  []guildApplication
	characterPrivacy   map[uint32]characterPrivacy
	wishlist           map[uint32]bool
}

func timePtr(value time.Time) *time.Time { return &value }

func newMockState() *mockState {
	published := time.Now().Add(-time.Hour)
	return &mockState{balance: 500, users: map[string]string{"DEMO": "demo1234"}, bans: map[string]string{}, couponUses: map[string]uint32{}, purchases: map[uint32]uint32{}, characterPrivacy: map[uint32]characterPrivacy{}, eventRegistrations: map[uint64]eventRegistration{}, products: buildMockProducts(), collections: []shopCollection{{ID: 1, Slug: "featured", Name: "Featured picks", Description: "Curated packages for this realm.", Active: true, Featured: true, ProductIDs: []uint32{1, 2}}}, tickets: []supportTicket{{ID: 1, AccountID: 1, Username: "DEMO", CharacterGUID: 1, Subject: "Missing quest item", Message: "The quest item did not drop after the boss encounter.", Status: "pending_staff", Category: "character", Priority: "normal", DueAt: timePtr(time.Now().Add(70 * time.Hour)), Created: time.Now().Add(-2 * time.Hour), Updated: time.Now().Add(-2 * time.Hour)}}, cannedReplies: []cannedReply{{ID: 1, Title: "Need more information", Body: "Thanks for contacting us. Please provide the character name, approximate time, and any relevant item or quest IDs.", Active: true}}, orders: []map[string]any{{"id": 1042, "itemId": 49623, "quantity": 1, "total": 85, "status": "review", "error": "Demonstration order awaiting staff reconciliation", "created": time.Now().Add(-24 * time.Hour)}}, ledger: []map[string]any{{"id": 1, "amount": 500, "reason": "Demo starting balance", "created": time.Now().Add(-48 * time.Hour)}}, notifications: []notification{{ID: 1, Kind: "order", Title: "Order delivered", Message: "Your starter package is ready in game.", ActionURL: "/account/orders", Created: time.Now().Add(-time.Hour)}}, staff: []map[string]any{{"accountId": 1, "username": "DEMO", "role": "administrator", "realmKey": "*", "permissions": []string{}, "grantedBy": "SYSTEM", "updated": time.Now()}}, news: []newsEntry{{ID: 1, Title: "Welcome to Azeroth", Slug: "welcome-to-azeroth", Summary: "The portal is ready for your community.", Body: "Welcome to the realm. This article demonstrates the complete publishing experience, including canonical links, authors, tags, and revision history.", AuthorName: "Realm Team", Tags: "welcome,community", Kind: "news", Status: "published", Active: true, Featured: true, PublishAt: &published}}, events: []realmEvent{{ID: 1, Title: "Wintergrasp community night", Description: "Join both factions for an organized battle and community prizes.", Category: "pvp", Location: "Wintergrasp", StartsAt: time.Now().Add(48 * time.Hour), Status: "scheduled", MaxParticipants: 80, SignupEnabled: true, RegistrationDeadline: timePtr(time.Now().Add(36 * time.Hour)), RewardCredits: 15}}, services: []dashboardService{{Action: "unstuck", Character: "Arthoria", Response: "Character moved to homebind.", Success: true, Created: time.Now().Add(-3 * time.Hour)}}, deleted: []deletedCharacter{{GUID: 99, Name: "Oldhero", DeletedAt: uint64(time.Now().Add(-24 * time.Hour).Unix())}}}
}

var mockCharacters = []character{
	{GUID: 1, Name: "Arthoria", Race: 1, Class: 2, Gender: 1, Level: 80, Zone: 1519, Online: false, TotalTime: 4827600, Guild: "Keepers of Dawn", GuildID: 1},
	{GUID: 2, Name: "Thornhoof", Race: 6, Class: 11, Level: 80, Zone: 1637, Online: false, TotalTime: 3196800, Guild: "Keepers of Dawn", GuildID: 1},
	{GUID: 3, Name: "Velistra", Race: 10, Class: 8, Gender: 1, Level: 76, Zone: 4395, Online: false, TotalTime: 1912200, Guild: "Silver Covenant", GuildID: 2},
	{GUID: 4, Name: "Grimward", Race: 5, Class: 6, Level: 80, Zone: 210, Online: false, TotalTime: 5882400, Guild: "Ashen Vanguard", GuildID: 3},
	{GUID: 5, Name: "Quickarrow", Race: 4, Class: 3, Gender: 1, Level: 71, Zone: 65, Online: false, TotalTime: 1105200},
	{GUID: 6, Name: "Emberhex", Race: 2, Class: 9, Level: 63, Zone: 3483, Online: false, TotalTime: 748800, Guild: "Ashen Vanguard", GuildID: 3},
	{GUID: 7, Name: "Ironward", Race: 3, Class: 1, Level: 80, Zone: 1519, Online: false, TotalTime: 2750400, Guild: "Keepers of Dawn", GuildID: 1},
	{GUID: 8, Name: "Nightshiv", Race: 5, Class: 4, Level: 78, Zone: 4395, Online: false, TotalTime: 1641600, Guild: "Ashen Vanguard", GuildID: 3},
	{GUID: 9, Name: "Dawnprayer", Race: 11, Class: 5, Gender: 1, Level: 74, Zone: 65, Online: false, TotalTime: 1296000, Guild: "Silver Covenant", GuildID: 2},
	{GUID: 10, Name: "Stormcaller", Race: 8, Class: 7, Level: 69, Zone: 3483, Online: false, TotalTime: 907200, Guild: ""},
}

func buildMockProducts() []product {
	boostIncludes := []string{"Level 80 boost", "All class trainer spell ranks", "All class weapon proficiencies at 400", "Artisan Riding and Cold Weather Flying", "Four Frostweave Bags", "Faction-appropriate ground and flying mounts", "10,000 gold"}
	products := []product{
		{ID: 1, ItemID: 49284, Quantity: 1, Price: 120, Name: "Reins of the Swift Spectral Tiger", Description: "The WotLK mount item, delivered directly through in-game mail.", Category: "Mounts"},
		{ID: 2, ItemID: 49623, Quantity: 1, Price: 85, Name: "Shadowmourne", Description: "A legendary two-handed axe for the realm's mightiest champions.", Category: "Weapons"},
		{ID: 3, ItemID: 51809, Quantity: 1, Price: 45, Name: "Portable Hole", Description: "A spacious 24-slot bag for long expeditions across Northrend.", Category: "Utility"},
		{ID: 4, ItemID: 23713, Quantity: 1, Price: 60, Name: "Hippogryph Hatchling", Description: "A loyal companion from the forests of Feralas.", Category: "Companions"},
		{ID: 5, ItemID: 37719, Quantity: 5, Price: 15, Name: "Adventurer Supply Bundle", Description: "Useful supplies for your next adventure.", Category: "Utility"},
		{ID: 6, ItemID: 50818, Quantity: 1, Price: 75, Name: "Invincible's Reins", Description: "The famed steed of the fallen prince awaits a new rider.", Category: "Mounts"},
		{ID: 7, Price: 40, Name: "Complete Level 80 Boost", Description: "Raise one existing character to level 80 with training, travel essentials, bags, mounts, and spending gold.", Category: "Services", Tier: "Level 80", ServiceLevel: 80, Gold: 10000, Includes: append([]string(nil), boostIncludes...)},
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
			includes = append(includes, boostIncludes...)
			products = append(products, product{ID: id, Price: season.Price, Name: set.Class + " " + set.Spec + " " + season.Name + " Package", Description: "WotLK " + gladiator + " starter loadout for " + set.Spec + ".", Category: "PvP", ClassID: set.ID, ClassName: set.Class, Tier: season.Name, ServiceLevel: 80, Gold: 10000, Includes: includes})
			id++
		}
		includes := []string{"Complete 5-piece Conqueror's " + set.T8Set + " set", "Ulduar neck, cloak, wrists, belt & boots", "2 Ulduar rings and 2 trinkets", "Spec weapon set + ranged weapon or relic"}
		includes = append(includes, kits[set.Role]...)
		includes = append(includes, boostIncludes...)
		products = append(products, product{ID: id, Price: 135, Name: set.Class + " " + set.Spec + " T8 Package", Description: "Ulduar raid-ready WotLK tier 8 loadout for " + set.Spec + ".", Category: "PvE", ClassID: set.ID, ClassName: set.Class, Tier: "T8", ServiceLevel: 80, Gold: 10000, Includes: includes})
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
			products[i].VariantRequired = true
			products[i].Variants = []productVariant{{ID: 1, Name: "Swift spectral tiger", SKU: "spectral-swift", Active: true, Items: []bundleItem{{ItemID: 49284, Quantity: 1}}}, {ID: 2, Name: "Spectral tiger", SKU: "spectral-standard", PriceAdjustment: -20, Active: true, SortOrder: 1, Items: []bundleItem{{ItemID: 33225, Quantity: 1}}}}
			products[i].Collections = []string{"featured"}
		} else if products[i].ID == 2 {
			products[i].Collections = []string{"featured"}
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
	m.HandleFunc("GET /api/auth/discord/start", s.discordOAuthStart)
	m.HandleFunc("GET /api/auth/discord/callback", s.discordOAuthCallback)
	m.HandleFunc("GET /api/auth/google/start", s.googleOAuthStart)
	m.HandleFunc("GET /api/auth/google/callback", s.googleOAuthCallback)
	m.HandleFunc("POST /api/auth/passkey/options", s.rate(20, 10*time.Minute, s.passkeyAuthenticationOptions))
	m.HandleFunc("POST /api/auth/passkey", s.rate(20, 10*time.Minute, s.passkeyAuthenticationFinish))
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
	m.HandleFunc("GET /api/media/{id}/{name}", s.mediaServe)
	m.HandleFunc("GET /api/news", s.newsList)
	m.HandleFunc("GET /api/news/{slug}", s.newsDetail)
	m.HandleFunc("GET /api/pages", s.publicPages)
	m.HandleFunc("GET /api/pages/{slug}", s.publicPage)
	m.HandleFunc("GET /api/events", s.publicEvents)
	m.HandleFunc("POST /api/events/{id}/registration", s.rate(10, time.Hour, s.eventRegistrationAction))
	m.HandleFunc("DELETE /api/events/{id}/registration", s.rate(10, time.Hour, s.eventRegistrationAction))
	m.HandleFunc("GET /api/community/discord", s.discordStatus)
	m.HandleFunc("GET /api/community/issues", s.communityIssues)
	m.HandleFunc("POST /api/community/issues", s.rate(5, time.Hour, s.createCommunityIssue))
	m.HandleFunc("GET /api/community/issues/{id}", s.communityIssueDetail)
	m.HandleFunc("POST /api/community/issues/{id}/vote", s.rate(30, time.Minute, s.communityIssueVote))
	m.HandleFunc("POST /api/community/issues/{id}/comments", s.rate(20, time.Hour, s.communityIssueComment))
	m.HandleFunc("GET /api/downloads", s.mockDownloads)
	m.HandleFunc("GET /api/launcher/manifest", s.mockLauncherManifest)
	m.HandleFunc("GET /api/tools/resources", s.publicTools)
	m.HandleFunc("GET /api/tools/items", s.itemDatabase)
	m.HandleFunc("GET /api/tools/talents", s.talentCalculator)
	m.HandleFunc("GET /api/me", s.mockMe)
	m.HandleFunc("GET /api/identity/accounts", s.identityAccounts)
	m.HandleFunc("POST /api/identity/accounts", s.identityLinkAccount)
	m.HandleFunc("POST /api/identity/accounts/{id}/switch", s.identitySwitchAccount)
	m.HandleFunc("PATCH /api/identity/accounts/{id}", s.identityRenameAccount)
	m.HandleFunc("POST /api/identity/accounts/{id}/primary", s.identityPromoteAccount)
	m.HandleFunc("DELETE /api/identity/accounts/{id}", s.identityUnlinkAccount)
	m.HandleFunc("DELETE /api/identity/providers/{provider}", s.identityUnlinkProvider)
	m.HandleFunc("GET /api/security/passkeys", s.passkeyList)
	m.HandleFunc("POST /api/security/passkeys/register/options", s.passkeyRegistrationOptions)
	m.HandleFunc("POST /api/security/passkeys/register", s.passkeyRegistrationFinish)
	m.HandleFunc("DELETE /api/security/passkeys/{id}", s.passkeyDelete)
	m.HandleFunc("GET /api/characters", s.mockOwnCharacters)
	m.HandleFunc("GET /api/armory", s.feature(s.c.EnableArmory, "Armory", s.mockArmory))
	m.HandleFunc("GET /api/armory/{name}", s.feature(s.c.EnableArmory, "Armory", s.mockCharacter))
	m.HandleFunc("GET /api/armory/{name}/insights", s.feature(s.c.EnableArmory, "Armory", s.armoryInsights))
	m.HandleFunc("GET /api/arena", s.feature(s.c.EnableRankings, "Rankings", s.mockArena))
	m.HandleFunc("GET /api/rankings", s.feature(s.c.EnableRankings, "Rankings", s.mockExpandedRankings))
	m.HandleFunc("GET /api/rankings/capabilities", s.feature(s.c.EnableRankings, "Rankings", s.rankingCapabilities))
	m.HandleFunc("GET /api/rankings/raids", s.feature(s.c.EnableRankings, "Rankings", s.raidRankings))
	m.HandleFunc("GET /api/progression/{name}", s.feature(s.c.EnableArmory, "Armory", s.mockProgression))
	m.HandleFunc("GET /api/realm", s.feature(s.c.EnableRealmStatus, "Realm status", s.mockRealm))
	m.HandleFunc("GET /api/guilds", s.feature(s.c.EnableGuilds, "Guilds", s.mockGuilds))
	m.HandleFunc("GET /api/guilds/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.mockGuild))
	m.HandleFunc("GET /api/guilds/{id}/recruitment", s.feature(s.c.EnableGuilds, "Guilds", s.guildRecruitmentProfile))
	m.HandleFunc("POST /api/guilds/{id}/applications", s.feature(s.c.EnableGuilds, "Guilds", s.rate(5, 24*time.Hour, s.createGuildApplication)))
	m.HandleFunc("GET /api/guild-applications", s.feature(s.c.EnableGuilds, "Guilds", s.guildApplications))
	m.HandleFunc("DELETE /api/guild-applications/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.withdrawGuildApplication))
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
	m.HandleFunc("GET /api/shop/{id}", s.feature(s.c.EnableShop, "Shop", s.shopProductDetail))
	m.HandleFunc("GET /api/shop/collections", s.feature(s.c.EnableShop, "Shop", s.shopCollections))
	m.HandleFunc("GET /api/shop/{id}/eligibility", s.feature(s.c.EnableShop, "Shop", s.shopProductEligibility))
	m.HandleFunc("GET /api/wishlist", s.feature(s.c.EnableShop, "Shop", s.wishlist))
	m.HandleFunc("PUT /api/wishlist/{id}", s.feature(s.c.EnableShop, "Shop", s.wishlistItem))
	m.HandleFunc("DELETE /api/wishlist/{id}", s.feature(s.c.EnableShop, "Shop", s.wishlistItem))
	m.HandleFunc("POST /api/shop/purchase", s.feature(s.c.EnableShop, "Shop", s.mockPurchase))
	m.HandleFunc("GET /api/characters/deleted", s.mockDeletedCharacters)
	m.HandleFunc("POST /api/characters/{guid}/service", s.mockCharacterService)
	m.HandleFunc("GET /api/characters/{guid}/privacy", s.characterPrivacySettings)
	m.HandleFunc("PUT /api/characters/{guid}/privacy", s.characterPrivacySettings)
	m.HandleFunc("GET /api/orders", s.mockOrders)
	m.HandleFunc("GET /api/wallet", s.mockWallet)
	m.HandleFunc("GET /api/notifications", s.mockNotifications)
	m.HandleFunc("POST /api/notifications/{id}/read", s.mockNotificationRead)
	m.HandleFunc("GET /api/dashboard", s.dashboard)
	m.HandleFunc("POST /api/rewards/daily", s.rate(3, time.Hour, s.claimDailyReward))
	m.HandleFunc("POST /api/rewards/referrals/{id}/claim", s.rate(10, time.Hour, s.claimReferralMilestone))
	m.HandleFunc("POST /api/rewards/missions/{id}/claim", s.rate(10, time.Hour, s.claimPlayerMission))
	m.HandleFunc("POST /api/rewards/vote/callback", s.rate(60, time.Minute, s.voteRewardCallback))
	m.HandleFunc("POST /api/integrations/discord/rewards", s.rate(120, time.Minute, s.discordRewardCallback))
	m.HandleFunc("GET /api/votes", func(w http.ResponseWriter, r *http.Request) {
		_, signedIn := s.mockUser(r)
		jsonOut(w, 200, map[string]any{"authenticated": signedIn, "sites": []map[string]any{{"id": 1, "slug": "top100arena", "name": "Top 100 Arena", "url": "https://example.com/vote", "description": "Support the realm once every 12 hours.", "rewardCredits": 5, "cooldownMinutes": 720, "available": signedIn}}})
	})
	m.HandleFunc("GET /api/votes/history", s.voteHistory)
	m.HandleFunc("GET /api/votes/leaderboard", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"period": time.Now().UTC().Format("2006-01"), "leaders": []map[string]any{{"rank": 1, "username": "DEMO", "votes": 18, "credits": 90}}})
	})
	m.HandleFunc("GET /api/votes/campaigns", s.voteCampaigns)
	m.HandleFunc("POST /api/votes/{id}/visit", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.mockUser(r); !ok {
			problem(w, 401, "Sign in required")
			return
		}
		jsonOut(w, 200, map[string]string{"url": "https://example.com/vote"})
	})
	m.HandleFunc("POST /api/integrations/votes/{slug}", s.rate(60, time.Minute, s.voteRewardCallback))
	m.HandleFunc("GET /api/tickets", s.feature(s.c.EnableSupport, "Support", s.mockTickets))
	m.HandleFunc("POST /api/tickets", s.feature(s.c.EnableSupport, "Support", s.mockCreateTicket))
	m.HandleFunc("POST /api/tickets/{id}/messages", s.feature(s.c.EnableSupport, "Support", s.mockTicketMessage))
	m.HandleFunc("GET /api/moderation/sanctions", s.playerSanctions)
	m.HandleFunc("POST /api/moderation/sanctions/{id}/appeal", s.createSanctionAppeal)
	m.HandleFunc("GET /api/transfers", s.transfers)
	m.HandleFunc("POST /api/transfers", s.rate(3, 24*time.Hour, s.createTransfer))
	m.HandleFunc("POST /api/admin/credits", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.mockCredits)))
	m.HandleFunc("GET /api/admin/orders", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrders))
	m.HandleFunc("GET /api/admin/orders/{id}/steps", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]any{"steps": []any{map[string]any{"key": "items:1", "kind": "items", "status": "failed", "attempts": 1, "response": "Demonstration partial delivery failure"}}})
	}))
	m.HandleFunc("POST /api/admin/orders/{id}/steps/{key}", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, 200, map[string]bool{"ok": true})
	}))
	m.HandleFunc("GET /api/admin/status", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminStatus))
	m.HandleFunc("POST /api/admin/delivery-diagnostic", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(3, time.Hour, s.adminDeliveryDiagnostic)))
	m.HandleFunc("GET /api/admin/analytics", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAnalytics))
	m.HandleFunc("GET /api/admin/ledger", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, r *http.Request) {
		page, perPage, _ := requestPage(r, 25, 100)
		entries, meta := slicePage([]map[string]any{{"id": 1, "Actor": "DEMO", "Target": "DEMO", "Amount": 500, "Reason": "Demo starting balance", "Created": time.Now().Add(-48 * time.Hour)}}, page, perPage)
		jsonOut(w, 200, map[string]any{"entries": entries, "pagination": meta})
	}))
	m.HandleFunc("GET /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, r *http.Request) {
		s.mock.mu.Lock()
		out := append([]product(nil), s.mock.products...)
		s.mock.mu.Unlock()
		search, status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))), strings.TrimSpace(r.URL.Query().Get("status"))
		filtered := out[:0]
		for _, item := range out {
			if search != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Category+" "+item.Tier+" "+item.Tags), search) || status == "active" && !item.Active || status == "archived" && item.Active {
				continue
			}
			filtered = append(filtered, item)
		}
		out = filtered
		sortKey, descending := r.URL.Query().Get("sort"), strings.EqualFold(r.URL.Query().Get("direction"), "desc")
		if sortKey != "" {
			sort.SliceStable(out, func(i, j int) bool {
				comparison := 0
				switch sortKey {
				case "id":
					comparison = int(out[i].ID) - int(out[j].ID)
				case "name":
					comparison = strings.Compare(out[i].Name, out[j].Name)
				case "category":
					comparison = strings.Compare(out[i].Category, out[j].Category)
				case "price":
					comparison = int(out[i].Price) - int(out[j].Price)
				case "active":
					if out[i].Active != out[j].Active {
						if out[i].Active {
							comparison = 1
						} else {
							comparison = -1
						}
					}
				default:
					return false
				}
				if descending {
					return comparison > 0
				}
				return comparison < 0
			})
		}
		page, perPage, _ := requestPage(r, 50, 200)
		out, meta := slicePage(out, page, perPage)
		jsonOut(w, 200, map[string]any{"products": out, "pagination": meta})
	}))
	m.HandleFunc("GET /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDetail))
	m.HandleFunc("GET /api/admin/items", s.feature(s.c.EnableAdminPanel, "Administration", s.adminItemSearch))
	m.HandleFunc("GET /api/admin/accounts", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminAccounts))
	m.HandleFunc("POST /api/admin/accounts/{id}/revoke-sessions", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Hour, s.mockAdminRevokeAccountSessions)))
	m.HandleFunc("POST /api/admin/accounts/{id}/require-password-reset", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(10, time.Hour, s.mockAdminRequirePasswordReset)))
	m.HandleFunc("POST /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.mockAdminModeration)))
	m.HandleFunc("GET /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminModerationLog))
	m.HandleFunc("GET /api/admin/sanction-appeals", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSanctionAppeals))
	m.HandleFunc("PUT /api/admin/sanction-appeals/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSanctionAppeal))
	m.HandleFunc("GET /api/admin/investigations/policy", s.feature(s.c.EnableAdminPanel, "Administration", s.adminInvestigationPolicy))
	m.HandleFunc("POST /api/admin/investigations/search", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(20, time.Hour, s.adminInvestigationSearch)))
	m.HandleFunc("POST /api/admin/investigations/evidence", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Hour, s.adminInvestigationEvidence)))
	m.HandleFunc("GET /api/admin/audit", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAudit))
	m.HandleFunc("GET /api/admin/media", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMedia))
	m.HandleFunc("POST /api/admin/media", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaUpload))
	m.HandleFunc("PATCH /api/admin/media/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaUpdate))
	m.HandleFunc("DELETE /api/admin/media/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaDelete))
	m.HandleFunc("GET /api/admin/navigation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigation))
	m.HandleFunc("POST /api/admin/navigation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationCreate))
	m.HandleFunc("PUT /api/admin/navigation/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationUpdate))
	m.HandleFunc("DELETE /api/admin/navigation/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationDelete))
	m.HandleFunc("GET /api/admin/audit/export", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditExport))
	m.HandleFunc("GET /api/admin/audit/filters", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilters))
	m.HandleFunc("POST /api/admin/audit/filters", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilters))
	m.HandleFunc("DELETE /api/admin/audit/filters/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilterDelete))
	m.HandleFunc("GET /api/admin/staff", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminStaff))
	m.HandleFunc("POST /api/admin/staff", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminStaff))
	m.HandleFunc("DELETE /api/admin/staff/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminStaffDelete))
	m.HandleFunc("GET /api/admin/realm-config", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRealmConfig))
	m.HandleFunc("POST /api/admin/realm-config/apply", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRealmConfig))
	m.HandleFunc("GET /api/admin/downloads", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminDownloads))
	m.HandleFunc("POST /api/admin/downloads", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminDownloads))
	m.HandleFunc("DELETE /api/admin/downloads/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminDownloadDelete))
	m.HandleFunc("GET /api/admin/launcher-patches", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminLauncherPatches))
	m.HandleFunc("POST /api/admin/launcher-patches", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminLauncherPatches))
	m.HandleFunc("DELETE /api/admin/launcher-patches/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminLauncherPatchDelete))
	m.HandleFunc("GET /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.mockAdminConsoleHistory))
	m.HandleFunc("POST /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.rate(20, time.Minute, s.mockAdminConsoleExecute)))
	m.HandleFunc("GET /api/admin/tickets", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.mockAdminTickets))
	m.HandleFunc("POST /api/admin/tickets/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.mockAdminTicketUpdate))
	m.HandleFunc("GET /api/admin/transfers", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTransfers))
	m.HandleFunc("POST /api/admin/transfers/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTransferUpdate))
	m.HandleFunc("GET /api/admin/tickets/{id}/events", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTicketEvents))
	m.HandleFunc("GET /api/admin/canned-replies", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplies))
	m.HandleFunc("POST /api/admin/canned-replies", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplies))
	m.HandleFunc("DELETE /api/admin/canned-replies/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplyDelete))
	m.HandleFunc("POST /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProduct))
	m.HandleFunc("PUT /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProductUpdate))
	m.HandleFunc("DELETE /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminProductDelete))
	m.HandleFunc("POST /api/admin/products/{id}/validate", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductValidation))
	m.HandleFunc("POST /api/admin/products/import", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCatalogImport))
	m.HandleFunc("GET /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("POST /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("DELETE /api/admin/coupons/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponDelete))
	m.HandleFunc("GET /api/admin/coupons/{id}/history", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponHistory))
	m.HandleFunc("GET /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("GET /api/admin/news/{id}/revisions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsRevisions))
	m.HandleFunc("GET /api/admin/vote-sites", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, 200, map[string]any{"sites": []any{}}) }))
	m.HandleFunc("POST /api/admin/vote-sites", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, 201, map[string]any{"ok": true, "id": 1}) }))
	m.HandleFunc("PUT /api/admin/vote-sites/{id}", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, 200, map[string]bool{"ok": true}) }))
	m.HandleFunc("DELETE /api/admin/vote-sites/{id}", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, 200, map[string]bool{"ok": true}) }))
	m.HandleFunc("GET /api/admin/vote-campaigns", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteCampaigns))
	m.HandleFunc("POST /api/admin/vote-campaigns", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteCampaigns))
	m.HandleFunc("POST /api/admin/vote-campaigns/{id}/draw", s.feature(s.c.EnableAdminPanel, "Administration", s.drawVoteCampaign))
	m.HandleFunc("GET /api/admin/missions", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, http.StatusOK, map[string]any{"missions": s.loadPlayerMissions(r.Context(), 1)})
	}))
	m.HandleFunc("POST /api/admin/missions", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": 5})
	}))
	m.HandleFunc("PUT /api/admin/missions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, http.StatusOK, map[string]bool{"ok": true}) }))
	m.HandleFunc("DELETE /api/admin/missions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", func(w http.ResponseWriter, _ *http.Request) { jsonOut(w, http.StatusOK, map[string]bool{"ok": true}) }))
	m.HandleFunc("POST /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("PUT /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("DELETE /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("GET /api/admin/pages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPages))
	m.HandleFunc("POST /api/admin/pages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPages))
	m.HandleFunc("PUT /api/admin/pages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPageItem))
	m.HandleFunc("DELETE /api/admin/pages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPageItem))
	m.HandleFunc("GET /api/admin/events", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEvents))
	m.HandleFunc("POST /api/admin/events", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEvents))
	m.HandleFunc("PUT /api/admin/events/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventItem))
	m.HandleFunc("DELETE /api/admin/events/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventItem))
	m.HandleFunc("GET /api/admin/events/{id}/participants", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventParticipants))
	m.HandleFunc("PUT /api/admin/events/{id}/participants/{account}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventParticipantStatus))
	m.HandleFunc("POST /api/admin/events/{id}/rewards", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventRewards))
	m.HandleFunc("GET /api/admin/community/issues", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCommunityIssues))
	m.HandleFunc("PUT /api/admin/community/issues/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCommunityIssue))
	m.HandleFunc("GET /api/admin/guild-recruitment", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildRecruitment))
	m.HandleFunc("POST /api/admin/guild-recruitment", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildRecruitment))
	m.HandleFunc("GET /api/admin/guild-applications", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildApplications))
	m.HandleFunc("PUT /api/admin/guild-applications/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildApplication))
	m.HandleFunc("GET /api/admin/arena-seasons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminArenaSeasons))
	m.HandleFunc("POST /api/admin/arena-seasons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminArenaSeasons))
	m.HandleFunc("GET /api/admin/ranking-exclusions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusions))
	m.HandleFunc("POST /api/admin/ranking-exclusions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusions))
	m.HandleFunc("DELETE /api/admin/ranking-exclusions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusionDelete))
	m.HandleFunc("GET /api/admin/shop/collections", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollections))
	m.HandleFunc("POST /api/admin/shop/collections", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollections))
	m.HandleFunc("PUT /api/admin/shop/collections/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollection))
	m.HandleFunc("DELETE /api/admin/shop/collections/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollection))
	m.HandleFunc("GET /api/admin/shop/stock", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStock))
	m.HandleFunc("POST /api/admin/shop/stock", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStock))
	m.HandleFunc("GET /api/admin/shop/bundles", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplates))
	m.HandleFunc("POST /api/admin/shop/bundles", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplates))
	m.HandleFunc("PUT /api/admin/shop/bundles/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplateUpdate))
	m.HandleFunc("DELETE /api/admin/shop/bundles/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplateDelete))
	m.HandleFunc("GET /api/admin/raid-eligibility", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRaidEligibilityRules))
	m.HandleFunc("PUT /api/admin/raid-eligibility", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRaidEligibilityRules))
	m.HandleFunc("GET /api/admin/resources", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTools))
	m.HandleFunc("POST /api/admin/resources", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTools))
	m.HandleFunc("PUT /api/admin/resources/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminToolItem))
	m.HandleFunc("DELETE /api/admin/resources/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminToolItem))
	m.HandleFunc("GET /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("PUT /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("POST /api/admin/orders/{id}/retry", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrderAction))
	m.HandleFunc("POST /api/admin/orders/bulk-retry", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(10, time.Minute, s.adminBulkRetryOrders)))
	m.HandleFunc("POST /api/admin/orders/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.mockAdminOrderAction))
	m.HandleFunc("POST /api/billing/checkout", s.feature(s.c.EnableShop, "Shop", s.mockBillingCheckout))
	m.HandleFunc("POST /api/gift-codes/redeem", s.rate(10, time.Hour, s.redeemGiftCode))
	m.HandleFunc("GET /api/billing/packages", s.feature(s.c.EnableShop, "Shop", s.billingPackages))
	m.HandleFunc("GET /api/billing/transactions", s.feature(s.c.EnableShop, "Shop", s.paymentTransactions))
	m.HandleFunc("GET /api/admin/credit-packages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackages))
	m.HandleFunc("POST /api/admin/credit-packages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackages))
	m.HandleFunc("DELETE /api/admin/credit-packages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackageDelete))
	m.HandleFunc("GET /api/admin/payments", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPayments))
	m.HandleFunc("POST /api/admin/payments/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPaymentRefund))
	m.HandleFunc("GET /api/admin/gift-codes", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodes))
	m.HandleFunc("POST /api/admin/gift-codes", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodes))
	m.HandleFunc("DELETE /api/admin/gift-codes/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodeDelete))
	m.HandleFunc("GET /api/security/sessions", s.mockSecuritySessions)
	m.HandleFunc("DELETE /api/security/sessions/{id}", s.mockSecurityRevoke)
	m.HandleFunc("POST /api/security/password", s.mockSecurityPassword)
	m.HandleFunc("POST /api/security/email", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in required")
			return
		}
		var in struct{ CurrentPassword, Email string }
		if !decode(w, r, &in) {
			return
		}
		if in.CurrentPassword != "demo1234" || !validEmailAddress(strings.ToUpper(strings.TrimSpace(in.Email))) {
			problem(w, http.StatusUnprocessableEntity, "Check the password and email address")
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "message": "Demo verification email queued."})
	})
	m.HandleFunc("POST /api/security/totp/setup", s.mockTOTPSetup)
	m.HandleFunc("POST /api/security/totp/enable", s.mockTOTPEnable)
	m.HandleFunc("POST /api/security/totp/disable", s.mockTOTPDisable)
	m.HandleFunc("GET /api/security/totp/status", s.mockTOTPStatus)
	m.HandleFunc("POST /api/security/step-up", s.rate(10, 10*time.Minute, s.mockSecurityStepUp))
	m.HandleFunc("GET /api/privacy/export", s.privacyExport)
	m.HandleFunc("GET /api/privacy/requests", s.privacyRequests)
	m.HandleFunc("POST /api/privacy/deletion", s.rate(3, 24*time.Hour, s.privacyDeletionRequest))
	m.HandleFunc("DELETE /api/privacy/requests/{id}", s.privacyRequestCancel)
	m.HandleFunc("GET /api/admin/privacy-requests", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPrivacyRequests))
	m.HandleFunc("POST /api/admin/privacy-requests/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPrivacyRequestUpdate))
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
	var in struct{ Username, Password, Email, TurnstileToken, ReferralCode string }
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
	totpSecret, totpEnabled := s.mock.totpSecret, s.mock.totpEnabled
	recovery := append([]string(nil), s.mock.totpRecovery...)
	s.mock.mu.Unlock()
	if !ok || password != in.Password {
		problem(w, 401, "Invalid username or password. Try DEMO / demo1234")
		return
	}
	if totpEnabled && !validTOTP(totpSecret, in.OTP, time.Now()) {
		matched := -1
		for index, code := range recovery {
			if subtle.ConstantTimeCompare([]byte(normalizeRecoveryCode(code)), []byte(normalizeRecoveryCode(in.OTP))) == 1 {
				matched = index
				break
			}
		}
		if matched < 0 {
			problem(w, http.StatusUnauthorized, "A valid authenticator or recovery code is required")
			return
		}
		s.mock.mu.Lock()
		if matched < len(s.mock.totpRecovery) {
			s.mock.totpRecovery = append(s.mock.totpRecovery[:matched], s.mock.totpRecovery[matched+1:]...)
		}
		s.mock.mu.Unlock()
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
	hidden := s.hiddenCharacterGUIDs(r)
	for _, c := range mockCharacters {
		if strings.Contains(strings.ToLower(c.Name), q) && !hidden[c.GUID] {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}
func (s *Server) mockCharacter(w http.ResponseWriter, r *http.Request) {
	for _, c := range mockCharacters {
		if strings.EqualFold(c.Name, r.PathValue("name")) {
			privacy, visible := s.armoryPrivacy(r, c.GUID, 1)
			if !visible {
				problem(w, http.StatusNotFound, "Character not found")
				return
			}
			items := []map[string]any{
				{"slot": 0, "entry": 51272, "name": "Sanctified Lightsworn Headpiece", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2245, "setId": 1211, "durability": 100, "maxDurability": 100, "icon": "inv_helmet_96", "stats": []string{"+124 Strength", "+172 Stamina", "Meta Socket · Red Socket"}, "enhancements": []itemEnhancement{{Slot: 0, Kind: "enchant", EnchantmentID: 3817, Name: "Arcanum of Torment"}, {Slot: 2, Kind: "gem", EnchantmentID: 3637, ItemID: 41398, Name: "Relentless Earthsiege Diamond"}, {Slot: 3, Kind: "gem", EnchantmentID: 3525, ItemID: 40111, Name: "Bold Cardinal Ruby"}}},
				{"slot": 1, "entry": 50763, "name": "Marrowgar's Scratching Choker", "quality": 4, "itemLevel": 264, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_necklace_53", "stats": []string{"+90 Strength", "+90 Stamina", "+60 Critical Strike"}},
				{"slot": 2, "entry": 51274, "name": "Sanctified Lightsworn Shoulderplates", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2072, "setId": 1211, "icon": "inv_shoulder_117", "stats": []string{"+108 Strength", "+146 Stamina", "+82 Haste"}},
				{"slot": 3, "entry": 43348, "name": "Tabard of the Explorer", "quality": 3, "itemLevel": 1, "requiredLevel": 1, "armor": 0, "icon": "inv_shirt_guildtabard_01", "stats": []string{"Soulbound"}},
				{"slot": 4, "entry": 51270, "name": "Sanctified Lightsworn Battleplate", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2760, "setId": 1211, "icon": "inv_chest_plate_26", "stats": []string{"+148 Strength", "+196 Stamina", "Red Socket · Blue Socket"}},
				{"slot": 5, "entry": 50620, "name": "Coldwraith Links", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1210, "icon": "inv_belt_60", "stats": []string{"+104 Strength", "+139 Stamina", "+80 Expertise"}},
				{"slot": 6, "entry": 51271, "name": "Sanctified Lightsworn Legplates", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 2417, "setId": 1211, "icon": "inv_pants_plate_35", "stats": []string{"+148 Strength", "+196 Stamina", "+92 Critical Strike"}},
				{"slot": 7, "entry": 50639, "name": "Blood-Soaked Saronite Stompers", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1478, "icon": "inv_boots_plate_06", "stats": []string{"+104 Strength", "+139 Stamina", "+72 Haste"}},
				{"slot": 8, "entry": 50659, "name": "Polar Bear Claw Bracers", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 975, "icon": "inv_bracer_45", "stats": []string{"+80 Strength", "+108 Stamina", "+56 Critical Strike"}},
				{"slot": 9, "entry": 51269, "name": "Sanctified Lightsworn Gauntlets", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 1726, "setId": 1211, "icon": "inv_gauntlets_92", "stats": []string{"+108 Strength", "+146 Stamina", "+74 Haste"}},
				{"slot": 10, "entry": 50402, "name": "Ashen Band of Endless Might", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_ring_84", "stats": []string{"+99 Strength", "+107 Stamina", "+59 Critical Strike"}},
				{"slot": 11, "entry": 50693, "name": "Might of Blight", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_ring_83", "stats": []string{"+99 Strength", "+107 Stamina", "+59 Haste"}},
				{"slot": 12, "entry": 50363, "name": "Deathbringer's Will", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 0, "icon": "inv_jewelry_trinket_04", "stats": []string{"+167 Armor Penetration", "Chance on hit: transform your power"}},
				{"slot": 13, "entry": 54590, "name": "Sharpened Twilight Scale", "quality": 4, "itemLevel": 284, "requiredLevel": 80, "armor": 0, "icon": "inv_misc_monsterscales_15", "stats": []string{"+184 Armor Penetration", "Chance on hit: +1472 Attack Power"}},
				{"slot": 14, "entry": 50677, "name": "Winding Sheet", "quality": 4, "itemLevel": 277, "requiredLevel": 80, "armor": 183, "icon": "inv_misc_cape_16", "stats": []string{"+90 Strength", "+90 Stamina", "+60 Haste"}},
				{"slot": 15, "entry": 50730, "name": "Glorenzelg, High-Blade of the Silver Hand", "quality": 4, "itemLevel": 271, "requiredLevel": 80, "armor": 0, "icon": "inv_sword_140", "stats": []string{"Two-Hand Sword", "954–1,432 Damage · Speed 3.60", "+164 Strength · +183 Stamina"}},
				{"slot": 18, "entry": 50455, "name": "Warsong Gulch Mark of Honor", "quality": 3, "itemLevel": 80, "requiredLevel": 80, "armor": 0, "icon": "inv_bannerpvp_02", "stats": []string{"Relic"}},
			}
			if !privacy.ShowGear {
				items = []map[string]any{}
			}
			personalArena := []characterArenaTeam{}
			if c.Name == "Arthoria" {
				personalArena = []characterArenaTeam{{ID: 1, Rank: 1, Name: "Relentless", Bracket: 2, Rating: 2478, SeasonGames: 184, SeasonWins: 137, PersonalRating: 2491, PersonalGames: 172, PersonalWins: 129}, {ID: 4, Rank: 4, Name: "Ice Block Heroes", Bracket: 3, Rating: 2155, SeasonGames: 128, SeasonWins: 82, PersonalRating: 2142, PersonalGames: 119, PersonalWins: 75}}
			}
			sets := []equippedItemSet{{ID: 1211, Name: "Lightsworn Battlegear", Equipped: 5, Metadata: true, Bonuses: []itemSetBonus{{Pieces: 2, SpellID: 70755, Name: "Divine Storm damage increased", Active: true}, {Pieces: 4, SpellID: 70756, Name: "Seal and Judgement damage increased", Active: true}}}}
			jsonOut(w, 200, map[string]any{"character": c, "equipment": items, "itemSets": sets, "profile": characterProfile{Achievements: 164, Exalted: 12, TalentSpecs: 2, TalentSpells: 44, Glyphs: 6, Professions: []profession{{ID: 164, Name: "Blacksmithing", Value: 450, Maximum: 450}, {ID: 186, Name: "Mining", Value: 450, Maximum: 450}}}, "arenaTeams": personalArena, "privacy": privacy})
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
	season := strings.TrimSpace(r.URL.Query().Get("season"))
	if season == "" {
		season = "current"
	}
	seasonName := map[string]string{"current": "Current season", "season-7": "Season 7", "season-6": "Season 6"}[season]
	if seasonName == "" {
		problem(w, http.StatusNotFound, "Arena season snapshot not found")
		return
	}
	if season != "current" {
		for index := range teams {
			teams[index].Rating -= uint16(70 + index*13)
		}
	}
	jsonOut(w, 200, map[string]any{"bracket": bracket, "teams": teams, "page": 1, "hasMore": false, "season": season, "seasonName": seasonName, "source": map[bool]string{true: "Immutable portal season snapshot", false: "Live AzerothCore arena tables"}[season != "current"], "seasons": s.arenaSeasons(r)})
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
		VariantID                uint64 `json:"variantId"`
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
	if in.VariantID > 0 {
		foundVariant := false
		for _, variant := range p.Variants {
			if variant.ID == in.VariantID && variant.Active {
				adjusted := int64(total) + int64(variant.PriceAdjustment)
				if adjusted < 0 {
					problem(w, 422, "Variant price is invalid")
					return
				}
				total, foundVariant = uint32(adjusted), true
			}
		}
		if !foundVariant {
			problem(w, 422, "Choose a valid product variant")
			return
		}
	} else if p.VariantRequired {
		problem(w, 422, "Choose a product variant")
		return
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
		if p.SalePrice > 0 && !found.AllowSale {
			problem(w, 422, "Coupon cannot be combined with a sale price")
			return
		}
		if total < found.MinSubtotal {
			problem(w, 422, "Coupon minimum subtotal was not reached")
			return
		}
		if found.Category != "" && !strings.EqualFold(found.Category, p.Category) {
			problem(w, 422, "Coupon is not valid for this category")
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
		s.mock.stockMovements = append([]stockMovement{{ID: uint64(len(s.mock.stockMovements) + 1), ProductID: p.ID, Delta: -1, Type: "sale", Reference: strconv.FormatUint(uint64(1000+len(s.mock.orders)+1), 10), Reason: "Order reserved", ActorID: 1, CreatedAt: time.Now()}}, s.mock.stockMovements...)
	}
	if couponApplied {
		s.mock.couponUses[code]++
	}
	id := 1043 + len(s.mock.orders)
	s.mock.orders = append([]map[string]any{{"id": id, "itemId": p.ItemID, "quantity": p.Quantity, "total": total, "coupon": code, "status": "delivered", "created": time.Now()}}, s.mock.orders...)
	s.mock.ledger = append([]map[string]any{{"id": len(s.mock.ledger) + 1, "amount": -int64(total), "reason": fmt.Sprintf("Order %d purchase", id), "created": time.Now()}}, s.mock.ledger...)
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

func (s *Server) mockWallet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	balance := s.mock.balance
	entries := append([]map[string]any(nil), s.mock.ledger...)
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]any{"balance": balance, "transactions": entries})
}

func (s *Server) mockNotifications(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	items := append([]notification(nil), s.mock.notifications...)
	unread := 0
	for _, item := range items {
		if item.ReadAt == nil {
			unread++
		}
	}
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

func (s *Server) mockNotificationRead(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	now := time.Now()
	for i := range s.mock.notifications {
		if r.PathValue("id") == "all" || strconv.FormatUint(s.mock.notifications[i].ID, 10) == r.PathValue("id") {
			s.mock.notifications[i].ReadAt = &now
		}
	}
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) mockAdminStaff(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "admin"); !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	if r.Method == http.MethodGet {
		jsonOut(w, http.StatusOK, map[string]any{"staff": s.mock.staff})
		return
	}
	var in struct {
		Username    string
		Role        string
		RealmKey    string
		ExpiresAt   *time.Time
		Permissions []string
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.RealmKey = strings.TrimSpace(in.RealmKey)
	if in.RealmKey == "" {
		in.RealmKey = s.c.RealmKey
	}
	if _, valid := rolePermissions[in.Role]; !valid || in.Username == "" {
		problem(w, http.StatusUnprocessableEntity, "Choose a valid account and staff role")
		return
	}
	if in.RealmKey != "*" && in.RealmKey != s.c.RealmKey {
		problem(w, http.StatusUnprocessableEntity, "Role scope must be this realm or all realms")
		return
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		problem(w, http.StatusUnprocessableEntity, "Temporary access must expire in the future")
		return
	}
	for i := range s.mock.staff {
		if s.mock.staff[i]["username"] == in.Username && s.mock.staff[i]["realmKey"] == in.RealmKey {
			s.mock.staff[i]["role"] = in.Role
			s.mock.staff[i]["realmKey"], s.mock.staff[i]["expiresAt"], s.mock.staff[i]["permissions"] = in.RealmKey, in.ExpiresAt, validStaffPermissions(in.Permissions)
			s.mock.staff[i]["updated"] = time.Now()
			jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	}
	accountID := len(s.mock.staff) + 1
	for _, member := range s.mock.staff {
		if member["username"] == in.Username {
			accountID = member["accountId"].(int)
			break
		}
	}
	s.mock.staff = append(s.mock.staff, map[string]any{"accountId": accountID, "username": in.Username, "role": in.Role, "realmKey": in.RealmKey, "expiresAt": in.ExpiresAt, "permissions": validStaffPermissions(in.Permissions), "grantedBy": "DEMO", "updated": time.Now()})
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) mockAdminStaffDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "admin"); !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id == 1 {
		problem(w, http.StatusUnprocessableEntity, "You cannot remove your own role")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	realmKey := strings.TrimSpace(r.URL.Query().Get("realm"))
	if realmKey == "" {
		realmKey = s.c.RealmKey
	}
	for i, member := range s.mock.staff {
		if member["accountId"] == id && member["realmKey"] == realmKey {
			s.mock.staff = append(s.mock.staff[:i], s.mock.staff[i+1:]...)
			jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, http.StatusNotFound, "Staff role not found")
}

func (s *Server) ensureMockDownloads() {
	if len(s.mock.downloads) == 0 {
		s.mock.downloads = []portalDownload{{ID: 1, Name: "Windows full client", Platform: "Windows", URL: "https://example.com/client.zip", Mirrors: []downloadMirror{{Label: "Europe mirror", URL: "https://mirror.example.com/client.zip"}}, Version: "3.3.5a · 12340", FileSize: "17 GB", SHA256: strings.Repeat("a", 64), SignatureURL: "https://example.com/client.zip.sig", VirusTotalURL: "https://www.virustotal.com/gui/home/url", ChangelogURL: "https://example.com/client/changelog", ReleasedAt: "2026-08-01", Requirements: "Windows 10 or newer, 4 GB RAM, 25 GB free storage, and a DirectX 9-compatible GPU.", Notes: "Clean enUS client", Active: true}}
	}
	if len(s.mock.launcherPatches) == 0 {
		s.mock.launcherPatches = []launcherPatch{{ID: 1, Platform: "Windows", FromVersion: "1.0.0", ToVersion: "1.1.0", URL: "https://example.com/patch-1.1.0.zip", Mirrors: []downloadMirror{{Label: "Europe mirror", URL: "https://mirror.example.com/patch-1.1.0.zip"}}, FileSize: "84 MB", SHA256: strings.Repeat("b", 64), SignatureURL: "https://example.com/patch-1.1.0.zip.sig", ReleasedAt: "2026-08-15", Notes: "Launcher and realm data update", Active: true}}
	}
}

func (s *Server) mockDownloads(w http.ResponseWriter, _ *http.Request) {
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	s.ensureMockDownloads()
	items := []portalDownload{}
	for _, item := range s.mock.downloads {
		if item.Active {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"downloads": items})
}

func (s *Server) mockLauncherManifest(w http.ResponseWriter, r *http.Request) {
	s.mock.mu.Lock()
	s.ensureMockDownloads()
	packages := []portalDownload{}
	for _, item := range s.mock.downloads {
		if item.Active {
			packages = append(packages, item)
		}
	}
	patches := []launcherPatch{}
	for _, item := range s.mock.launcherPatches {
		if item.Active {
			patches = append(patches, item)
		}
	}
	s.mock.mu.Unlock()
	settings := s.runtimeSettings(r)
	jsonOut(w, http.StatusOK, map[string]any{
		"schemaVersion": 2,
		"generatedAt":   time.Now().UTC(),
		"realm": map[string]any{
			"key": s.c.RealmKey, "name": settings.RealmName,
			"address": settings.RealmAddress,
		},
		"client": map[string]any{
			"expansion": s.c.ExpansionName, "version": s.c.ClientVersion,
			"build": s.c.ClientBuild,
		},
		"packages": packages,
		"patches":  patches,
	})
}

func (s *Server) mockAdminLauncherPatches(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	s.ensureMockDownloads()
	if r.Method == http.MethodGet {
		jsonOut(w, http.StatusOK, map[string]any{"patches": s.mock.launcherPatches})
		return
	}
	var item launcherPatch
	if !decode(w, r, &item) {
		return
	}
	if err := validateLauncherPatch(item); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	item.ID = uint64(len(s.mock.launcherPatches) + 1)
	item.Active = true
	s.mock.launcherPatches = append(s.mock.launcherPatches, item)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": item.ID})
}

func (s *Server) mockAdminLauncherPatchDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	s.ensureMockDownloads()
	for index := range s.mock.launcherPatches {
		if s.mock.launcherPatches[index].ID == id {
			s.mock.launcherPatches[index].Active = false
			jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, http.StatusNotFound, "Launcher patch not found")
}

func (s *Server) mockAdminDownloads(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, 403, "Content access required")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	s.ensureMockDownloads()
	if r.Method == http.MethodGet {
		jsonOut(w, 200, map[string]any{"downloads": s.mock.downloads})
		return
	}
	var item portalDownload
	if !decode(w, r, &item) {
		return
	}
	if err := validateDownload(item); err != nil {
		problem(w, 422, err.Error())
		return
	}
	item.ID = uint32(len(s.mock.downloads) + 1)
	s.mock.downloads = append(s.mock.downloads, item)
	jsonOut(w, 201, map[string]any{"ok": true, "id": item.ID})
}

func (s *Server) mockAdminDownloadDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, 403, "Content access required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 32)
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	s.ensureMockDownloads()
	for i := range s.mock.downloads {
		if s.mock.downloads[i].ID == uint32(id) {
			s.mock.downloads[i].Active = false
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, "Download not found")
}

func (s *Server) mockAdminStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "monitoring"); !ok {
		problem(w, http.StatusForbidden, "GM access required")
		return
	}
	cfg := s.runtimeSettings(r)
	active, message := s.maintenanceActive(r)
	dependencies := []map[string]any{
		{"name": "Portal", "configured": true, "reachable": true, "authorized": true, "latencyMs": 1, "detail": "Mock HTTP service"},
		{"name": "Authentication database", "configured": true, "reachable": true, "authorized": true, "latencyMs": 2},
		{"name": "Character database", "configured": true, "reachable": true, "authorized": true, "latencyMs": 3},
		{"name": "World database", "configured": true, "reachable": true, "authorized": true, "latencyMs": 2},
		{"name": "AzerothCore SOAP", "configured": true, "reachable": nil, "authorized": nil, "detail": "Observed during delivery; demo commands are simulated"},
		{"name": "SMTP", "configured": false, "reachable": nil, "authorized": nil},
		{"name": "Stripe", "configured": false, "reachable": nil, "authorized": nil},
		{"name": "Competitive ingestion", "configured": false, "reachable": nil, "authorized": nil},
	}
	jsonOut(w, http.StatusOK, map[string]any{"online": true, "realm": cfg.RealmName, "address": cfg.RealmAddress, "shopDelivery": true, "portal": true, "database": true, "soapConfigured": true, "deliveryDiagnostic": map[string]any{"configured": strings.TrimSpace(s.c.DeliveryDiagnosticCharacter) != "", "character": s.c.DeliveryDiagnosticCharacter, "itemId": deliveryDiagnosticItemID, "requiresOffline": true}, "dependencies": dependencies, "maintenance": active, "maintenanceMessage": message, "checkedAt": time.Now(), "demo": true})
}

func (s *Server) mockTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	tickets := append([]supportTicket(nil), s.mock.tickets...)
	s.mock.mu.Unlock()
	status, priority, search := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("priority")), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	filtered := tickets[:0]
	for _, ticket := range tickets {
		if status != "" && ticket.Status != status || priority != "" && ticket.Priority != priority || search != "" && !strings.Contains(strings.ToLower(ticket.Subject+" "+ticket.Username), search) {
			continue
		}
		filtered = append(filtered, ticket)
	}
	page, perPage, _ := requestPage(r, 25, 100)
	filtered, meta := slicePage(filtered, page, perPage)
	jsonOut(w, 200, map[string]any{"tickets": filtered, "pagination": meta})
}

func (s *Server) mockCreateTicket(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		CharacterGUID    uint32
		Subject, Message string
		Category         string
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
	if !validTicketCategory(in.Category) {
		in.Category = "general"
	}
	due := now.Add(72 * time.Hour)
	s.mock.tickets = append([]supportTicket{{ID: id, AccountID: 1, Username: "DEMO", CharacterGUID: in.CharacterGUID, Subject: in.Subject, Message: in.Message, Status: "pending_staff", Category: in.Category, Priority: "normal", DueAt: &due, Created: now, Updated: now, Messages: []ticketMessage{{ID: 1, AuthorID: 1, AuthorRole: "player", Message: in.Message, Created: now}}}}, s.mock.tickets...)
	s.mock.mu.Unlock()
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) mockAdminTickets(w http.ResponseWriter, r *http.Request) { s.mockTickets(w, r) }

func (s *Server) mockTicketMessage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var in struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Message) < 2 || len(in.Message) > 4000 {
		problem(w, http.StatusUnprocessableEntity, "Message must be 2–4000 characters")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for i := range s.mock.tickets {
		if s.mock.tickets[i].ID == id && s.mock.tickets[i].Status != "closed" {
			now := time.Now()
			s.mock.tickets[i].Messages = append(s.mock.tickets[i].Messages, ticketMessage{ID: uint64(len(s.mock.tickets[i].Messages) + 1), AuthorID: 1, AuthorRole: "player", Message: in.Message, Created: now})
			s.mock.tickets[i].Status, s.mock.tickets[i].Updated = "pending_staff", now
			jsonOut(w, http.StatusCreated, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, http.StatusNotFound, "Open ticket not found")
}

func (s *Server) mockAdminTicketUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var in struct {
		Status, Response, InternalNote, Category, Priority, Tags string
		AssignToSelf, Unassign                                   bool
	}
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for i := range s.mock.tickets {
		if s.mock.tickets[i].ID == id {
			if in.Status != "" {
				s.mock.tickets[i].Status = in.Status
			}
			s.mock.tickets[i].Response = in.Response
			s.mock.tickets[i].GM = "DEMO"
			if in.Category != "" {
				s.mock.tickets[i].Category = in.Category
			}
			if in.Priority != "" {
				s.mock.tickets[i].Priority = in.Priority
			}
			if in.Tags != "" {
				s.mock.tickets[i].Tags = in.Tags
			}
			if in.AssignToSelf {
				s.mock.tickets[i].AssignedTo, s.mock.tickets[i].AssignedName = 1, "DEMO"
			}
			if in.Unassign {
				s.mock.tickets[i].AssignedTo, s.mock.tickets[i].AssignedName = 0, ""
			}
			s.mock.tickets[i].Updated = time.Now()
			if in.Response != "" {
				s.mock.tickets[i].Messages = append(s.mock.tickets[i].Messages, ticketMessage{ID: uint64(len(s.mock.tickets[i].Messages) + 1), AuthorID: 1, AuthorRole: "staff", Message: in.Response, Created: time.Now()})
			}
			if in.InternalNote != "" {
				s.mock.tickets[i].Messages = append(s.mock.tickets[i].Messages, ticketMessage{ID: uint64(len(s.mock.tickets[i].Messages) + 1), AuthorID: 1, AuthorRole: "internal", Message: in.InternalNote, Created: time.Now()})
			}
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
	page, perPage, _ := requestPage(r, 25, 100)
	accounts, meta := slicePage(accounts, page, perPage)
	jsonOut(w, 200, map[string]any{"accounts": accounts, "pagination": meta})
}

func (s *Server) mockAdminRevokeAccountSessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "moderation"); !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || (id != 1 && id != 2) {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	username := "DEMO"
	if id == 2 {
		username = "FROSTBYTE"
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "accountId": id, "username": username, "revoked": 2, "requestId": RequestID(r.Context())})
}

func (s *Server) mockAdminRequirePasswordReset(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "admin"); !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || (id != 1 && id != 2) {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Reason)) < 3 {
		problem(w, http.StatusUnprocessableEntity, "Reason is required")
		return
	}
	username := "DEMO"
	if id == 2 {
		username = "FROSTBYTE"
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "accountId": id, "username": username, "revoked": 2, "requestId": RequestID(r.Context())})
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
	page, perPage, _ := requestPage(r, 25, 100)
	entries, meta := slicePage(entries, page, perPage)
	jsonOut(w, 200, map[string]any{"entries": entries, "pagination": meta})
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
		Package, RecipientUsername, Message string
	}
	if !decode(w, r, &in) {
		return
	}
	credits := map[string]uint32{"small": 100, "medium": 550, "large": 1200}[in.Package]
	for _, pack := range s.availableCreditPackages(r, false) {
		if pack.Slug == in.Package {
			credits = pack.Credits
		}
	}
	if credits == 0 {
		problem(w, 422, "Unknown credit package")
		return
	}
	if in.RecipientUsername != "" && !strings.EqualFold(in.RecipientUsername, "DEMO") {
		problem(w, 422, "Gift recipient was not found")
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
	s.mock.totpRecovery = nil
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
	codes, _, err := generateRecoveryCodes(10)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not generate recovery codes")
		return
	}
	s.mock.totpRecovery = codes
	jsonOut(w, 200, map[string]any{"ok": true, "recoveryCodes": codes})
}
func (s *Server) mockTOTPDisable(w http.ResponseWriter, r *http.Request) {
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
	if s.mock.totpEnabled && !validTOTP(s.mock.totpSecret, in.Code, time.Now()) {
		matched := -1
		for index, code := range s.mock.totpRecovery {
			if subtle.ConstantTimeCompare([]byte(normalizeRecoveryCode(code)), []byte(normalizeRecoveryCode(in.Code))) == 1 {
				matched = index
				break
			}
		}
		if matched < 0 {
			s.mock.mu.Unlock()
			problem(w, http.StatusUnprocessableEntity, "Invalid authenticator or recovery code")
			return
		}
	}
	s.mock.totpEnabled = false
	s.mock.totpSecret = ""
	s.mock.totpRecovery = nil
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) mockTOTPStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	enabled, remaining := s.mock.totpEnabled, len(s.mock.totpRecovery)
	s.mock.mu.Unlock()
	jsonOut(w, http.StatusOK, map[string]any{"enabled": enabled, "recoveryCodesRemaining": remaining, "enrollmentAvailable": true})
}

func (s *Server) mockSecurityStepUp(w http.ResponseWriter, r *http.Request) {
	username, ok := s.mockUser(r)
	if !ok {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var in struct{ Password, Code string }
	if !decode(w, r, &in) {
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	if s.mock.users[username] != in.Password || s.mock.totpEnabled && !validTOTP(s.mock.totpSecret, in.Code, time.Now()) {
		problem(w, http.StatusUnauthorized, "Password or authenticator code is incorrect")
		return
	}
	s.mock.stepUpUntil = time.Now().Add(10 * time.Minute)
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "expiresIn": 600})
}
func (s *Server) mockAdminOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	page, perPage, _ := requestPage(r, 25, 100)
	orders, meta := slicePage(s.mock.orders, page, perPage)
	jsonOut(w, 200, map[string]any{"orders": orders, "pagination": meta})
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
