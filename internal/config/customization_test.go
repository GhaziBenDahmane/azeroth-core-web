package config

import "testing"

func TestLoadCustomization(t *testing.T) {
	t.Setenv("UI_TEXT_JSON", `{"nav.home":"Accueil"}`)
	t.Setenv("NEWS_JSON", `[{"title":"Launch","summary":"Welcome","date":"2026-08-28","url":"https://example.com/news"}]`)
	t.Setenv("REALMS_JSON", `[{"key":"frost","name":"Frosthold","id":1},{"key":"ember","name":"Emberfall","id":2}]`)
	c := Config{ThemePrimary: "#112233", ThemeSecondary: "#223344", ThemeAccent: "#334455", ThemeBackground: "#445566", Locale: "fr-FR", RealmKey: "frost", RealmID: 1, RealmName: "Frosthold", PublicURL: "https://portal.example.com", CharactersDB: "acore_characters", WorldDB: "acore_world"}
	if err := loadCustomization(&c); err != nil {
		t.Fatalf("valid customization rejected: %v", err)
	}
	if c.UIText["nav.home"] != "Accueil" || len(c.News) != 1 || c.News[0].Title != "Launch" || len(c.Realms) != 2 || c.Realms[1].Key != "ember" {
		t.Fatalf("customization was not decoded: %#v %#v %#v", c.UIText, c.News, c.Realms)
	}
}

func TestLoadCustomizationRejectsRealmDirectoryWithoutCurrentRealm(t *testing.T) {
	t.Setenv("REALMS_JSON", `[{"key":"ember","name":"Emberfall","id":2}]`)
	c := Config{ThemePrimary: "#112233", ThemeSecondary: "#223344", ThemeAccent: "#334455", ThemeBackground: "#445566", Locale: "en", RealmKey: "frost", RealmID: 1, CharactersDB: "acore_characters", WorldDB: "acore_world"}
	if err := loadCustomization(&c); err == nil {
		t.Fatal("realm directory without active REALM_KEY was accepted")
	}
}

func TestLoadCustomizationRejectsUnsafeValues(t *testing.T) {
	t.Setenv("UI_TEXT_JSON", `{}`)
	t.Setenv("NEWS_JSON", `[]`)
	c := Config{ThemePrimary: "red; background:url(x)", ThemeSecondary: "#223344", ThemeAccent: "#334455", ThemeBackground: "#445566", Locale: "en"}
	if err := loadCustomization(&c); err == nil {
		t.Fatal("unsafe theme value was accepted")
	}
	c.ThemePrimary = "#112233"
	c.LogoURL = "javascript:alert(1)"
	if err := loadCustomization(&c); err == nil {
		t.Fatal("unsafe asset URL was accepted")
	}
}

func TestSetupRequiresStrongToken(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("ENABLE_SETUP", "true")
	t.Setenv("SETUP_TOKEN", "short")
	if _, err := Load(); err == nil {
		t.Fatal("setup accepted a short token")
	}
}

func TestHTTPSPublicURLRequiresSecureCookies(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "https://portal.example.com")
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := Load(); err == nil {
		t.Fatal("HTTPS public URL accepted insecure cookies")
	}
	t.Setenv("COOKIE_SECURE", "true")
	if _, err := Load(); err != nil {
		t.Fatalf("secure HTTPS configuration rejected: %v", err)
	}
}

func TestRejectsInvalidPublicURL(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("PUBLIC_URL", "portal.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("relative PUBLIC_URL was accepted")
	}
}

func TestGMConsoleConfiguration(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("ENABLE_GM_CONSOLE", "true")
	t.Setenv("GM_CONSOLE_LEVEL", "2")
	t.Setenv("GM_CONSOLE_ALLOWED_PREFIXES", "server info, lookup item")
	c, err := Load()
	if err != nil {
		t.Fatalf("valid console configuration rejected: %v", err)
	}
	if !c.EnableGMConsole || c.GMConsoleLevel != 2 || len(c.GMConsoleAllowed) != 2 || c.GMConsoleAllowed[1] != "lookup item" {
		t.Fatalf("unexpected console configuration: %#v", c)
	}
}

func TestDiscordWebhookValidation(t *testing.T) {
	for _, webhook := range []string{
		"http://discord.com/api/webhooks/123/token",
		"https://example.com/api/webhooks/123/token",
		"https://discord.com/channels/123/456",
		"javascript:alert(1)",
	} {
		t.Run(webhook, func(t *testing.T) {
			t.Setenv("MOCK_MODE", "true")
			t.Setenv("DISCORD_WEBHOOK_URL", webhook)
			if _, err := Load(); err == nil {
				t.Fatalf("unsafe Discord webhook %q was accepted", webhook)
			}
		})
	}

	t.Setenv("MOCK_MODE", "true")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/123/token")
	if _, err := Load(); err != nil {
		t.Fatalf("valid Discord webhook rejected: %v", err)
	}
}

func TestHomepageAndStaffConfiguration(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("HOME_HEADLINE", "Enter Frosthold")
	t.Setenv("HOME_EYEBROW", "Two realms online")
	t.Setenv("HOME_PRIMARY_CTA", "Begin your journey")
	t.Setenv("HOME_CONNECT_TITLE", "Join the realm")
	t.Setenv("STAFF_SUPPORT_GM_LEVEL", "1")
	t.Setenv("STAFF_MODERATOR_GM_LEVEL", "2")
	c, err := Load()
	if err != nil {
		t.Fatalf("valid homepage and staff configuration rejected: %v", err)
	}
	if c.HomeHeadline != "Enter Frosthold" || c.HomeEyebrow != "Two realms online" || c.HomePrimaryCTA != "Begin your journey" || c.HomeConnectTitle != "Join the realm" {
		t.Fatalf("homepage configuration was not loaded: %#v", c)
	}
	if c.SupportGMLevel != 1 || c.ModeratorGMLevel != 2 {
		t.Fatalf("staff thresholds = support %d, moderator %d", c.SupportGMLevel, c.ModeratorGMLevel)
	}
}

func TestRejectsInvertedStaffLevels(t *testing.T) {
	t.Setenv("MOCK_MODE", "true")
	t.Setenv("STAFF_SUPPORT_GM_LEVEL", "2")
	t.Setenv("STAFF_MODERATOR_GM_LEVEL", "1")
	if _, err := Load(); err == nil {
		t.Fatal("inverted staff thresholds were accepted")
	}
}
