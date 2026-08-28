package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	localeID = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
	textKey  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

func loadCustomization(c *Config) error {
	for name, value := range map[string]string{
		"THEME_PRIMARY": c.ThemePrimary, "THEME_SECONDARY": c.ThemeSecondary,
		"THEME_ACCENT": c.ThemeAccent, "THEME_BACKGROUND": c.ThemeBackground,
	} {
		if !hexColor.MatchString(value) {
			return fmt.Errorf("%s must be a six-digit hex color", name)
		}
	}
	if !localeID.MatchString(c.Locale) {
		return fmt.Errorf("PORTAL_LOCALE must be a language tag such as en or fr-FR")
	}
	for name, value := range map[string]string{
		"DOWNLOAD_URL": c.DownloadURL, "COMMUNITY_URL": c.CommunityURL,
		"TERMS_URL": c.TermsURL, "PRIVACY_URL": c.PrivacyURL,
	} {
		if !validPublicURL(value, true) {
			return fmt.Errorf("%s must be empty, root-relative, or an absolute HTTP(S) URL", name)
		}
	}
	for name, value := range map[string]string{"LOGO_URL": c.LogoURL, "HERO_IMAGE_URL": c.HeroImageURL, "FAVICON_URL": c.FaviconURL} {
		if !validPublicURL(value, false) {
			return fmt.Errorf("%s must be empty, root-relative, or an absolute HTTPS URL", name)
		}
	}
	c.UIText = map[string]string{}
	if raw := strings.TrimSpace(os.Getenv("UI_TEXT_JSON")); raw != "" {
		if len(raw) > 32*1024 || json.Unmarshal([]byte(raw), &c.UIText) != nil {
			return fmt.Errorf("UI_TEXT_JSON must be a JSON object smaller than 32 KiB")
		}
		if len(c.UIText) > 200 {
			return fmt.Errorf("UI_TEXT_JSON supports at most 200 entries")
		}
		for key, value := range c.UIText {
			if !textKey.MatchString(key) || len(value) > 500 {
				return fmt.Errorf("UI_TEXT_JSON contains an invalid key or value")
			}
		}
	}
	c.News = []NewsItem{}
	if raw := strings.TrimSpace(os.Getenv("NEWS_JSON")); raw != "" {
		if len(raw) > 64*1024 || json.Unmarshal([]byte(raw), &c.News) != nil || len(c.News) > 12 {
			return fmt.Errorf("NEWS_JSON must be an array of at most 12 items and smaller than 64 KiB")
		}
		for _, item := range c.News {
			if strings.TrimSpace(item.Title) == "" || len(item.Title) > 120 || len(item.Summary) > 1000 || len(item.Date) > 40 || !validPublicURL(item.URL, true) {
				return fmt.Errorf("NEWS_JSON contains an invalid news item")
			}
		}
	}
	return nil
}

func validPublicURL(value string, allowHTTP bool) bool {
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	u, err := url.ParseRequestURI(value)
	return err == nil && (u.Scheme == "https" || (allowHTTP && u.Scheme == "http")) && u.Host != ""
}
