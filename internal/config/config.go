package config

import (
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Addr, AuthDSN, CharactersDSN, WorldDSN     string
	AuthDB, CharactersDB, WorldDB              string
	PublicURL, RealmName, RealmAddress         string
	RealmKey                                   string
	DefaultRealmKey                            string
	PortalName, BrandMark, PortalTagline       string
	HomeHeadline, HomeEyebrow                  string
	HomePrimaryCTA, HomeConnectTitle           string
	HomeGuideText, HomeRules                   string
	DiscordStatus, HomeChangelog               string
	ExpansionName, ClientVersion               string
	ClientBuild, ExperienceRate                string
	RealmType, RealmTimezone                   string
	RealmDescription, SeasonName               string
	QuestExperienceRate, KillExperienceRate    string
	ExplorationExperienceRate, DropRate        string
	ReputationRate, HonorRate, ProfessionRate  string
	FactionPolicy                              string
	StartLevel, MaxLevel, PopulationCap        int
	CrossFaction                               bool
	CrossFactionAccounts, CrossFactionCalendar bool
	CrossFactionChannels, CrossFactionGroups   bool
	CrossFactionGuilds, CrossFactionAuctions   bool
	CrossFactionMail, CrossFactionWho          bool
	CrossFactionFriends, CrossFactionTrade     bool
	UptimeLabel, FooterText                    string
	DownloadURL, CommunityURL                  string
	LogoURL, HeroImageURL, FaviconURL          string
	ThemePrimary, ThemeSecondary               string
	ThemeAccent, ThemeBackground               string
	Locale, TermsURL, PrivacyURL               string
	UIText                                     map[string]string
	News                                       []NewsItem
	Realms                                     []RealmConfig
	SOAPURL, SOAPUser, SOAPPassword            string
	CookieSecure                               bool
	TrustProxy                                 bool
	Expansion                                  int
	StartingCredits                            int
	AdminToken                                 string
	MockMode                                   bool
	RealmID                                    int
	GMLevel                                    int
	SupportGMLevel, ModeratorGMLevel           int
	StaffShopManagers                          map[string]bool
	StripeSecret                               string
	StripeWebhookSecret                        string
	StripePriceSmall                           string
	StripePriceMedium                          string
	StripePriceLarge                           string
	TurnstileSiteKey, TurnstileSecret          string
	SMTPAddr, SMTPUser, SMTPPassword           string
	SMTPFrom                                   string
	DiscordWebhookURL                          string
	VoteURL                                    string
	VoteRewardCredits                          int
	VoteCallbackSecret                         string
	RequireEmailVerification                   bool
	RealmStartWebhook, RealmControlToken       string
	EnableRegistration, EnableArmory           bool
	EnableRankings, EnableGuilds               bool
	EnableRealmStatus, EnableShop              bool
	EnableSupport, EnableAdminPanel            bool
	EnableGMConsole, GMConsoleAllowAll         bool
	GMConsoleLevel                             int
	GMConsoleAllowed                           []string
	EnableSetup                                bool
	SetupToken                                 string
	SetupGMLevel, SetupGMRealmID               int
}

type NewsItem struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Date    string `json:"date"`
	URL     string `json:"url"`
}

// RealmConfig describes one AzerothCore realm served by this portal process.
// Connection fields are never included in the public configuration response.
type RealmConfig struct {
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	Address              string `json:"address"`
	ExperienceRate       string `json:"experienceRate"`
	RealmType            string `json:"type"`
	Timezone             string `json:"timezone"`
	Description          string `json:"description"`
	SeasonName           string `json:"seasonName"`
	QuestXPRate          string `json:"questXpRate"`
	KillXPRate           string `json:"killXpRate"`
	ExplorationXPRate    string `json:"explorationXpRate"`
	DropRate             string `json:"dropRate"`
	ReputationRate       string `json:"reputationRate"`
	HonorRate            string `json:"honorRate"`
	ProfessionRate       string `json:"professionRate"`
	FactionPolicy        string `json:"factionPolicy"`
	StartLevel           int    `json:"startLevel"`
	MaxLevel             int    `json:"maxLevel"`
	PopulationCap        int    `json:"populationCap"`
	CrossFaction         *bool  `json:"crossFaction"`
	CrossFactionAccounts *bool  `json:"crossFactionAccounts"`
	CrossFactionCalendar *bool  `json:"crossFactionCalendar"`
	CrossFactionChannels *bool  `json:"crossFactionChannels"`
	CrossFactionGroups   *bool  `json:"crossFactionGroups"`
	CrossFactionGuilds   *bool  `json:"crossFactionGuilds"`
	CrossFactionAuctions *bool  `json:"crossFactionAuctions"`
	CrossFactionMail     *bool  `json:"crossFactionMail"`
	CrossFactionWho      *bool  `json:"crossFactionWho"`
	CrossFactionFriends  *bool  `json:"crossFactionFriends"`
	CrossFactionTrade    *bool  `json:"crossFactionTrade"`
	CharactersDSN        string `json:"charactersDsn"`
	WorldDSN             string `json:"worldDsn"`
	CharactersDB         string `json:"charactersDb"`
	WorldDB              string `json:"worldDb"`
	SOAPURL              string `json:"soapUrl"`
	SOAPUser             string `json:"soapUser"`
	SOAPPassword         string `json:"soapPassword"`
	StartWebhook         string `json:"startWebhook"`
	ControlToken         string `json:"controlToken"`
	ID                   int    `json:"id"`
}

func (c Config) ForRealm(realm RealmConfig) Config {
	c.RealmKey, c.RealmName, c.RealmAddress, c.RealmID = realm.Key, realm.Name, realm.Address, realm.ID
	c.ExperienceRate = realm.ExperienceRate
	if realm.RealmType != "" {
		c.RealmType = realm.RealmType
	}
	if realm.Timezone != "" {
		c.RealmTimezone = realm.Timezone
	}
	if realm.Description != "" {
		c.RealmDescription = realm.Description
	}
	if realm.SeasonName != "" {
		c.SeasonName = realm.SeasonName
	}
	if realm.QuestXPRate != "" {
		c.QuestExperienceRate = realm.QuestXPRate
	}
	if realm.KillXPRate != "" {
		c.KillExperienceRate = realm.KillXPRate
	}
	if realm.ExplorationXPRate != "" {
		c.ExplorationExperienceRate = realm.ExplorationXPRate
	}
	if realm.DropRate != "" {
		c.DropRate = realm.DropRate
	}
	if realm.ReputationRate != "" {
		c.ReputationRate = realm.ReputationRate
	}
	if realm.HonorRate != "" {
		c.HonorRate = realm.HonorRate
	}
	if realm.ProfessionRate != "" {
		c.ProfessionRate = realm.ProfessionRate
	}
	if realm.FactionPolicy != "" {
		c.FactionPolicy = realm.FactionPolicy
	}
	if realm.StartLevel > 0 {
		c.StartLevel = realm.StartLevel
	}
	if realm.MaxLevel > 0 {
		c.MaxLevel = realm.MaxLevel
	}
	if realm.PopulationCap > 0 {
		c.PopulationCap = realm.PopulationCap
	}
	if realm.CrossFaction != nil {
		c.CrossFaction = *realm.CrossFaction
	}
	for source, target := range map[*bool]*bool{
		realm.CrossFactionAccounts: &c.CrossFactionAccounts, realm.CrossFactionCalendar: &c.CrossFactionCalendar,
		realm.CrossFactionChannels: &c.CrossFactionChannels, realm.CrossFactionGroups: &c.CrossFactionGroups,
		realm.CrossFactionGuilds: &c.CrossFactionGuilds, realm.CrossFactionAuctions: &c.CrossFactionAuctions,
		realm.CrossFactionMail: &c.CrossFactionMail, realm.CrossFactionWho: &c.CrossFactionWho,
		realm.CrossFactionFriends: &c.CrossFactionFriends, realm.CrossFactionTrade: &c.CrossFactionTrade,
	} {
		if source != nil {
			*target = *source
		}
	}
	c.CharactersDSN, c.WorldDSN = realm.CharactersDSN, realm.WorldDSN
	c.CharactersDB, c.WorldDB = realm.CharactersDB, realm.WorldDB
	c.SOAPURL, c.SOAPUser, c.SOAPPassword = realm.SOAPURL, realm.SOAPUser, realm.SOAPPassword
	c.RealmStartWebhook, c.RealmControlToken = realm.StartWebhook, realm.ControlToken
	return c
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func Load() (Config, error) {
	c := Config{
		Addr:    env("PORTAL_ADDR", ":8080"),
		AuthDSN: os.Getenv("AUTH_DSN"), CharactersDSN: os.Getenv("CHARACTERS_DSN"), WorldDSN: os.Getenv("WORLD_DSN"),
		AuthDB: env("AUTH_DB", "acore_auth"), CharactersDB: env("CHARACTERS_DB", "acore_characters"), WorldDB: env("WORLD_DB", "acore_world"),
		PublicURL: env("PUBLIC_URL", "http://localhost:8080"), RealmName: env("REALM_NAME", "Azeroth"), RealmAddress: env("REALM_ADDRESS", "logon.example.com"), RealmKey: env("REALM_KEY", "default"),
		PortalName: env("PORTAL_NAME", env("REALM_NAME", "Azeroth")), BrandMark: env("BRAND_MARK", "A"),
		PortalTagline: env("PORTAL_TAGLINE", "A timeless realm, shaped by its community. Forge alliances, conquer raids, and write your story."),
		HomeHeadline:  strings.TrimSpace(os.Getenv("HOME_HEADLINE")), HomeEyebrow: env("HOME_EYEBROW", "Realm status"),
		HomePrimaryCTA: env("HOME_PRIMARY_CTA", "Create account"), HomeConnectTitle: env("HOME_CONNECT_TITLE", "Connect in three steps"),
		HomeGuideText: env("HOME_GUIDE_TEXT", "Everything you need to join the server."), HomeRules: strings.TrimSpace(os.Getenv("HOME_RULES")),
		DiscordStatus: strings.TrimSpace(os.Getenv("DISCORD_STATUS")), HomeChangelog: strings.TrimSpace(os.Getenv("HOME_CHANGELOG")),
		ExpansionName: env("EXPANSION_NAME", "Wrath of the Lich King"), ClientVersion: env("CLIENT_VERSION", "3.3.5a"), ClientBuild: env("CLIENT_BUILD", "12340"),
		ExperienceRate: env("EXPERIENCE_RATE", "2×"), UptimeLabel: env("UPTIME_LABEL", "24/7"),
		RealmType: env("REALM_TYPE", "PvE"), RealmTimezone: env("REALM_TIMEZONE", "UTC"), RealmDescription: strings.TrimSpace(os.Getenv("REALM_DESCRIPTION")), SeasonName: strings.TrimSpace(os.Getenv("SEASON_NAME")),
		QuestExperienceRate: env("XP_QUEST_RATE", env("EXPERIENCE_RATE", "2×")), KillExperienceRate: env("XP_KILL_RATE", env("EXPERIENCE_RATE", "2×")), ExplorationExperienceRate: env("XP_EXPLORATION_RATE", env("EXPERIENCE_RATE", "2×")),
		DropRate: env("DROP_RATE", "1×"), ReputationRate: env("REPUTATION_RATE", "1×"), HonorRate: env("HONOR_RATE", "1×"), ProfessionRate: env("PROFESSION_RATE", "1×"),
		FactionPolicy: env("FACTION_POLICY", "both"), StartLevel: envInt("START_LEVEL", 1), MaxLevel: envInt("MAX_LEVEL", 80), PopulationCap: envInt("POPULATION_CAP", 0), CrossFaction: envBool("CROSS_FACTION", false),
		CrossFactionAccounts: envBool("CROSS_FACTION_ACCOUNTS", envBool("CROSS_FACTION", false)), CrossFactionCalendar: envBool("CROSS_FACTION_CALENDAR", envBool("CROSS_FACTION", false)),
		CrossFactionChannels: envBool("CROSS_FACTION_CHANNELS", envBool("CROSS_FACTION", false)), CrossFactionGroups: envBool("CROSS_FACTION_GROUPS", envBool("CROSS_FACTION", false)),
		CrossFactionGuilds: envBool("CROSS_FACTION_GUILDS", envBool("CROSS_FACTION", false)), CrossFactionAuctions: envBool("CROSS_FACTION_AUCTIONS", envBool("CROSS_FACTION", false)),
		CrossFactionMail: envBool("CROSS_FACTION_MAIL", envBool("CROSS_FACTION", false)), CrossFactionWho: envBool("CROSS_FACTION_WHO", envBool("CROSS_FACTION", false)),
		CrossFactionFriends: envBool("CROSS_FACTION_FRIENDS", envBool("CROSS_FACTION", false)), CrossFactionTrade: envBool("CROSS_FACTION_TRADE", envBool("CROSS_FACTION", false)),
		FooterText:  env("FOOTER_TEXT", "Independent community realm portal. Not affiliated with Blizzard Entertainment."),
		DownloadURL: strings.TrimSpace(os.Getenv("DOWNLOAD_URL")), CommunityURL: strings.TrimSpace(os.Getenv("COMMUNITY_URL")),
		LogoURL: strings.TrimSpace(os.Getenv("LOGO_URL")), HeroImageURL: strings.TrimSpace(os.Getenv("HERO_IMAGE_URL")), FaviconURL: strings.TrimSpace(os.Getenv("FAVICON_URL")),
		ThemePrimary: env("THEME_PRIMARY", "#d3ae68"), ThemeSecondary: env("THEME_SECONDARY", "#f3d89c"), ThemeAccent: env("THEME_ACCENT", "#3fd0be"), ThemeBackground: env("THEME_BACKGROUND", "#07110f"),
		Locale: env("PORTAL_LOCALE", "en"), TermsURL: strings.TrimSpace(os.Getenv("TERMS_URL")), PrivacyURL: strings.TrimSpace(os.Getenv("PRIVACY_URL")),
		SOAPURL: os.Getenv("SOAP_URL"), SOAPUser: os.Getenv("SOAP_USER"), SOAPPassword: os.Getenv("SOAP_PASSWORD"),
		CookieSecure: envBool("COOKIE_SECURE", false), TrustProxy: envBool("TRUST_PROXY", false), Expansion: envInt("ACCOUNT_EXPANSION", 2),
		StartingCredits: envInt("STARTING_CREDITS", 0), AdminToken: os.Getenv("ADMIN_TOKEN"),
		MockMode: envBool("MOCK_MODE", false),
		RealmID:  envInt("REALM_ID", 1), GMLevel: envInt("GM_LEVEL", 3),
		StripeSecret: os.Getenv("STRIPE_SECRET_KEY"), StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceSmall: os.Getenv("STRIPE_PRICE_SMALL"), StripePriceMedium: os.Getenv("STRIPE_PRICE_MEDIUM"), StripePriceLarge: os.Getenv("STRIPE_PRICE_LARGE"),
		TurnstileSiteKey: os.Getenv("TURNSTILE_SITE_KEY"), TurnstileSecret: os.Getenv("TURNSTILE_SECRET"),
		SMTPAddr: os.Getenv("SMTP_ADDR"), SMTPUser: os.Getenv("SMTP_USER"), SMTPPassword: os.Getenv("SMTP_PASSWORD"), SMTPFrom: os.Getenv("SMTP_FROM"),
		DiscordWebhookURL:        strings.TrimSpace(os.Getenv("DISCORD_WEBHOOK_URL")),
		VoteURL:                  strings.TrimSpace(os.Getenv("VOTE_URL")),
		VoteRewardCredits:        envInt("VOTE_REWARD_CREDITS", 0),
		VoteCallbackSecret:       os.Getenv("VOTE_CALLBACK_SECRET"),
		RequireEmailVerification: envBool("REQUIRE_EMAIL_VERIFICATION", false),
		RealmStartWebhook:        os.Getenv("REALM_START_WEBHOOK"), RealmControlToken: os.Getenv("REALM_CONTROL_TOKEN"),
		EnableRegistration: envBool("ENABLE_REGISTRATION", true), EnableArmory: envBool("ENABLE_ARMORY", true),
		EnableRankings: envBool("ENABLE_RANKINGS", true), EnableGuilds: envBool("ENABLE_GUILDS", true),
		EnableRealmStatus: envBool("ENABLE_REALM_STATUS", true), EnableShop: envBool("ENABLE_SHOP", true),
		EnableSupport: envBool("ENABLE_SUPPORT", true), EnableAdminPanel: envBool("ENABLE_ADMIN_PANEL", true),
		EnableGMConsole: envBool("ENABLE_GM_CONSOLE", false), GMConsoleAllowAll: envBool("GM_CONSOLE_ALLOW_ALL", false),
		GMConsoleLevel: envInt("GM_CONSOLE_LEVEL", 3),
		EnableSetup:    envBool("ENABLE_SETUP", false), SetupToken: os.Getenv("SETUP_TOKEN"),
		SetupGMLevel: envInt("SETUP_GM_LEVEL", 3), SetupGMRealmID: envInt("SETUP_GM_REALM_ID", -1),
	}
	c.SupportGMLevel = envInt("STAFF_SUPPORT_GM_LEVEL", c.GMLevel)
	c.ModeratorGMLevel = envInt("STAFF_MODERATOR_GM_LEVEL", c.GMLevel)
	c.StaffShopManagers = map[string]bool{}
	for _, username := range strings.Split(os.Getenv("STAFF_SHOP_MANAGERS"), ",") {
		username = strings.ToUpper(strings.TrimSpace(username))
		if username == "" {
			continue
		}
		if len(username) > 32 || !regexp.MustCompile(`^[A-Z0-9_]+$`).MatchString(username) {
			return c, fmt.Errorf("STAFF_SHOP_MANAGERS contains an invalid account name")
		}
		c.StaffShopManagers[username] = true
	}
	if c.AuthDSN == "" && !c.MockMode {
		return c, fmt.Errorf("AUTH_DSN is required")
	}
	publicURL, err := url.ParseRequestURI(c.PublicURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		return c, fmt.Errorf("PUBLIC_URL must be an absolute HTTP or HTTPS URL")
	}
	if publicURL.Scheme == "https" && !c.CookieSecure {
		return c, fmt.Errorf("COOKIE_SECURE must be true when PUBLIC_URL uses HTTPS")
	}
	if c.CharactersDSN == "" {
		c.CharactersDSN = c.AuthDSN
	}
	if c.WorldDSN == "" {
		c.WorldDSN = c.AuthDSN
	}
	for _, name := range []string{c.AuthDB, c.CharactersDB, c.WorldDB} {
		if !identifier.MatchString(name) {
			return c, fmt.Errorf("invalid database name %q", name)
		}
	}
	if err := loadCustomization(&c); err != nil {
		return c, err
	}
	c.DefaultRealmKey = c.RealmKey
	if c.EnableSetup && (len(c.SetupToken) < 16 || len(c.SetupToken) > 256) {
		return c, fmt.Errorf("SETUP_TOKEN must contain 16–256 characters when setup is enabled")
	}
	if c.SetupGMLevel < 1 || c.SetupGMLevel > 3 || c.SetupGMRealmID < -1 {
		return c, fmt.Errorf("invalid setup GM level or realm ID")
	}
	if c.GMConsoleLevel < 1 || c.GMConsoleLevel > 3 {
		return c, fmt.Errorf("GM_CONSOLE_LEVEL must be between 1 and 3")
	}
	if c.SupportGMLevel < 1 || c.ModeratorGMLevel < c.SupportGMLevel || c.GMLevel < c.ModeratorGMLevel || c.GMLevel > 3 {
		return c, fmt.Errorf("staff GM levels must satisfy support <= moderator <= administrator <= 3")
	}
	if c.StartLevel < 1 || c.StartLevel > 80 || c.MaxLevel < c.StartLevel || c.MaxLevel > 80 || c.PopulationCap < 0 {
		return c, fmt.Errorf("START_LEVEL, MAX_LEVEL, or POPULATION_CAP is invalid")
	}
	if c.EnableRegistration && c.RequireEmailVerification && !c.MockMode {
		if _, _, err := net.SplitHostPort(c.SMTPAddr); err != nil {
			return c, fmt.Errorf("SMTP_ADDR must be a host:port when email verification is required")
		}
		from, err := mail.ParseAddress(c.SMTPFrom)
		if err != nil || from.Address != c.SMTPFrom {
			return c, fmt.Errorf("SMTP_FROM must be a plain email address when email verification is required")
		}
	}
	if c.DiscordWebhookURL != "" {
		u, err := url.ParseRequestURI(c.DiscordWebhookURL)
		if err != nil || u.Scheme != "https" || (u.Host != "discord.com" && u.Host != "discordapp.com") || !strings.HasPrefix(u.Path, "/api/webhooks/") {
			return c, fmt.Errorf("DISCORD_WEBHOOK_URL must be an HTTPS Discord webhook URL")
		}
	}
	if !validPublicURL(c.VoteURL, true) || c.VoteRewardCredits < 0 || c.VoteRewardCredits > 100000 {
		return c, fmt.Errorf("VOTE_URL or VOTE_REWARD_CREDITS is invalid")
	}
	for _, prefix := range strings.Split(env("GM_CONSOLE_ALLOWED_PREFIXES", "help,server info,server motd,account online list,lookup,player info"), ",") {
		prefix = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(prefix, ".")))
		if prefix == "" || len(prefix) > 80 || strings.ContainsAny(prefix, "\r\n\x00") {
			return c, fmt.Errorf("GM_CONSOLE_ALLOWED_PREFIXES contains an invalid command prefix")
		}
		c.GMConsoleAllowed = append(c.GMConsoleAllowed, prefix)
	}
	return c, nil
}
func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func envBool(k string, d bool) bool {
	v, e := strconv.ParseBool(os.Getenv(k))
	if e == nil {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	v, e := strconv.Atoi(os.Getenv(k))
	if e == nil {
		return v
	}
	return d
}
