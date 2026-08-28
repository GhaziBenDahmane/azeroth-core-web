package config

import "testing"

func TestLoadCustomization(t *testing.T) {
	t.Setenv("UI_TEXT_JSON", `{"nav.home":"Accueil"}`)
	t.Setenv("NEWS_JSON", `[{"title":"Launch","summary":"Welcome","date":"2026-08-28","url":"https://example.com/news"}]`)
	c := Config{ThemePrimary: "#112233", ThemeSecondary: "#223344", ThemeAccent: "#334455", ThemeBackground: "#445566", Locale: "fr-FR"}
	if err := loadCustomization(&c); err != nil {
		t.Fatalf("valid customization rejected: %v", err)
	}
	if c.UIText["nav.home"] != "Accueil" || len(c.News) != 1 || c.News[0].Title != "Launch" {
		t.Fatalf("customization was not decoded: %#v %#v", c.UIText, c.News)
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
