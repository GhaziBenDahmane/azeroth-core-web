package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Addr, AuthDSN, CharactersDSN, WorldDSN string
	AuthDB, CharactersDB, WorldDB          string
	PublicURL, RealmName, RealmAddress     string
	PortalName, BrandMark, PortalTagline   string
	ExpansionName, ClientVersion           string
	ClientBuild, ExperienceRate            string
	UptimeLabel, FooterText                string
	DownloadURL, CommunityURL              string
	LogoURL, HeroImageURL, FaviconURL      string
	ThemePrimary, ThemeSecondary           string
	ThemeAccent, ThemeBackground           string
	Locale, TermsURL, PrivacyURL           string
	UIText                                 map[string]string
	News                                   []NewsItem
	SOAPURL, SOAPUser, SOAPPassword        string
	CookieSecure                           bool
	TrustProxy                             bool
	Expansion                              int
	StartingCredits                        int
	AdminToken                             string
	MockMode                               bool
	RealmID                                int
	GMLevel                                int
	StripeSecret                           string
	StripeWebhookSecret                    string
	StripePriceSmall                       string
	StripePriceMedium                      string
	StripePriceLarge                       string
	TurnstileSiteKey, TurnstileSecret      string
	SMTPAddr, SMTPUser, SMTPPassword       string
	SMTPFrom                               string
	RealmStartWebhook, RealmControlToken   string
	EnableRegistration, EnableArmory       bool
	EnableRankings, EnableGuilds           bool
	EnableRealmStatus, EnableShop          bool
	EnableSupport, EnableAdminPanel        bool
	EnableGMConsole, GMConsoleAllowAll     bool
	GMConsoleLevel                         int
	GMConsoleAllowed                       []string
	EnableSetup                            bool
	SetupToken                             string
	SetupGMLevel, SetupGMRealmID           int
}

type NewsItem struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Date    string `json:"date"`
	URL     string `json:"url"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func Load() (Config, error) {
	c := Config{
		Addr:    env("PORTAL_ADDR", ":8080"),
		AuthDSN: os.Getenv("AUTH_DSN"), CharactersDSN: os.Getenv("CHARACTERS_DSN"), WorldDSN: os.Getenv("WORLD_DSN"),
		AuthDB: env("AUTH_DB", "acore_auth"), CharactersDB: env("CHARACTERS_DB", "acore_characters"), WorldDB: env("WORLD_DB", "acore_world"),
		PublicURL: env("PUBLIC_URL", "http://localhost:8080"), RealmName: env("REALM_NAME", "Azeroth"), RealmAddress: env("REALM_ADDRESS", "logon.example.com"),
		PortalName: env("PORTAL_NAME", env("REALM_NAME", "Azeroth")), BrandMark: env("BRAND_MARK", "A"),
		PortalTagline: env("PORTAL_TAGLINE", "A timeless realm, shaped by its community. Forge alliances, conquer raids, and write your story."),
		ExpansionName: env("EXPANSION_NAME", "Wrath of the Lich King"), ClientVersion: env("CLIENT_VERSION", "3.3.5a"), ClientBuild: env("CLIENT_BUILD", "12340"),
		ExperienceRate: env("EXPERIENCE_RATE", "2×"), UptimeLabel: env("UPTIME_LABEL", "24/7"),
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
		RealmStartWebhook: os.Getenv("REALM_START_WEBHOOK"), RealmControlToken: os.Getenv("REALM_CONTROL_TOKEN"),
		EnableRegistration: envBool("ENABLE_REGISTRATION", true), EnableArmory: envBool("ENABLE_ARMORY", true),
		EnableRankings: envBool("ENABLE_RANKINGS", true), EnableGuilds: envBool("ENABLE_GUILDS", true),
		EnableRealmStatus: envBool("ENABLE_REALM_STATUS", true), EnableShop: envBool("ENABLE_SHOP", true),
		EnableSupport: envBool("ENABLE_SUPPORT", true), EnableAdminPanel: envBool("ENABLE_ADMIN_PANEL", true),
		EnableGMConsole: envBool("ENABLE_GM_CONSOLE", false), GMConsoleAllowAll: envBool("GM_CONSOLE_ALLOW_ALL", false),
		GMConsoleLevel: envInt("GM_CONSOLE_LEVEL", 3),
		EnableSetup:    envBool("ENABLE_SETUP", false), SetupToken: os.Getenv("SETUP_TOKEN"),
		SetupGMLevel: envInt("SETUP_GM_LEVEL", 3), SetupGMRealmID: envInt("SETUP_GM_REALM_ID", -1),
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
	if c.EnableSetup && (len(c.SetupToken) < 16 || len(c.SetupToken) > 256) {
		return c, fmt.Errorf("SETUP_TOKEN must contain 16–256 characters when setup is enabled")
	}
	if c.SetupGMLevel < 1 || c.SetupGMLevel > 3 || c.SetupGMRealmID < -1 {
		return c, fmt.Errorf("invalid setup GM level or realm ID")
	}
	if c.GMConsoleLevel < 1 || c.GMConsoleLevel > 3 {
		return c, fmt.Errorf("GM_CONSOLE_LEVEL must be between 1 and 3")
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
