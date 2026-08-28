package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type siteSettings struct {
	PortalName         string     `json:"portalName"`
	RealmName          string     `json:"realmName"`
	BrandMark          string     `json:"brandMark"`
	Tagline            string     `json:"tagline"`
	RealmAddress       string     `json:"realmAddress"`
	ExperienceRate     string     `json:"experienceRate"`
	DownloadURL        string     `json:"downloadUrl"`
	CommunityURL       string     `json:"communityUrl"`
	TermsURL           string     `json:"termsUrl"`
	PrivacyURL         string     `json:"privacyUrl"`
	ThemePrimary       string     `json:"themePrimary"`
	ThemeSecondary     string     `json:"themeSecondary"`
	ThemeAccent        string     `json:"themeAccent"`
	ThemeBackground    string     `json:"themeBackground"`
	MaintenanceEnabled bool       `json:"maintenanceEnabled"`
	MaintenanceMessage string     `json:"maintenanceMessage"`
	MaintenanceStarts  *time.Time `json:"maintenanceStarts,omitempty"`
	MaintenanceEnds    *time.Time `json:"maintenanceEnds,omitempty"`
	Registration       *bool      `json:"registration"`
	Armory             *bool      `json:"armory"`
	Rankings           *bool      `json:"rankings"`
	Guilds             *bool      `json:"guilds"`
	Realm              *bool      `json:"realm"`
	Shop               *bool      `json:"shop"`
	Support            *bool      `json:"support"`
	Admin              *bool      `json:"admin"`
	GMConsole          *bool      `json:"gmConsole"`
}

type newsEntry struct {
	ID        uint64     `json:"id"`
	Title     string     `json:"title"`
	Summary   string     `json:"summary"`
	URL       string     `json:"url"`
	Kind      string     `json:"kind"`
	PublishAt *time.Time `json:"publishAt,omitempty"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Active    bool       `json:"active"`
}

type coupon struct {
	ID              uint64     `json:"id"`
	Code            string     `json:"code"`
	DiscountPercent uint8      `json:"discountPercent"`
	DiscountCredits uint32     `json:"discountCredits"`
	MaxUses         uint32     `json:"maxUses"`
	PerAccountLimit uint32     `json:"perAccountLimit"`
	StartsAt        *time.Time `json:"startsAt,omitempty"`
	EndsAt          *time.Time `json:"endsAt,omitempty"`
	Active          bool       `json:"active"`
}

var couponCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,39}$`)

func boolPtr(v bool) *bool { return &v }

func (s *Server) siteSettingsKey() string { return "site_config:" + s.c.RealmKey }

func (s *Server) defaultSiteSettings() siteSettings {
	return siteSettings{PortalName: s.c.PortalName, RealmName: s.c.RealmName, BrandMark: s.c.BrandMark, Tagline: s.c.PortalTagline,
		RealmAddress: s.c.RealmAddress, ExperienceRate: s.c.ExperienceRate, DownloadURL: s.c.DownloadURL, CommunityURL: s.c.CommunityURL, TermsURL: s.c.TermsURL, PrivacyURL: s.c.PrivacyURL,
		ThemePrimary: s.c.ThemePrimary, ThemeSecondary: s.c.ThemeSecondary, ThemeAccent: s.c.ThemeAccent, ThemeBackground: s.c.ThemeBackground,
		Registration: boolPtr(s.c.EnableRegistration), Armory: boolPtr(s.c.EnableArmory), Rankings: boolPtr(s.c.EnableRankings), Guilds: boolPtr(s.c.EnableGuilds),
		Realm: boolPtr(s.c.EnableRealmStatus), Shop: boolPtr(s.c.EnableShop), Support: boolPtr(s.c.EnableSupport), Admin: boolPtr(s.c.EnableAdminPanel), GMConsole: boolPtr(s.c.EnableGMConsole)}
}

func (s *Server) runtimeSettings(ctx *http.Request) siteSettings {
	base := s.defaultSiteSettings()
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if s.mock.settings.PortalName != "" {
			return s.mock.settings
		}
		return base
	}
	var raw string
	if s.s.Auth.QueryRowContext(ctx.Context(), "SELECT setting_value FROM portal_settings WHERE setting_key=?", s.siteSettingsKey()).Scan(&raw) != nil && s.c.RealmKey == "default" {
		_ = s.s.Auth.QueryRowContext(ctx.Context(), "SELECT setting_value FROM portal_settings WHERE setting_key='site_config'").Scan(&raw)
	}
	if raw != "" {
		var stored siteSettings
		if json.Unmarshal([]byte(raw), &stored) == nil && stored.PortalName != "" {
			// Preserve compatibility with site_config JSON saved before this setting existed.
			if strings.TrimSpace(stored.ExperienceRate) == "" {
				stored.ExperienceRate = base.ExperienceRate
			}
			return stored
		}
	}
	return base
}

func settingBool(v *bool, fallback bool) bool {
	if v == nil {
		return fallback
	}
	return *v
}

func (s *Server) maintenanceActive(r *http.Request) (bool, string) {
	cfg := s.runtimeSettings(r)
	now := time.Now()
	active := cfg.MaintenanceEnabled && (cfg.MaintenanceStarts == nil || !now.Before(*cfg.MaintenanceStarts)) && (cfg.MaintenanceEnds == nil || now.Before(*cfg.MaintenanceEnds))
	return active, cfg.MaintenanceMessage
}

func (s *Server) featureEnabled(r *http.Request, name string, fallback bool) bool {
	c := s.runtimeSettings(r)
	switch name {
	case "Registration":
		return fallback && settingBool(c.Registration, true)
	case "Armory":
		return fallback && settingBool(c.Armory, true)
	case "Rankings":
		return fallback && settingBool(c.Rankings, true)
	case "Guilds":
		return fallback && settingBool(c.Guilds, true)
	case "Realm status":
		return fallback && settingBool(c.Realm, true)
	case "Shop":
		return fallback && settingBool(c.Shop, true)
	case "Support":
		return fallback && settingBool(c.Support, true)
	case "Administration":
		return fallback && settingBool(c.Admin, true)
	case "GM console":
		return s.c.EnableAdminPanel && fallback && settingBool(c.Admin, true) && settingBool(c.GMConsole, true)
	}
	return fallback
}

func (s *Server) publicNews(r *http.Request) []newsEntry {
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		return append([]newsEntry(nil), s.mock.news...)
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,title,summary,url,kind,publish_at,expires_at,active FROM portal_news WHERE active=1 AND (publish_at IS NULL OR publish_at<=NOW()) AND (expires_at IS NULL OR expires_at>NOW()) ORDER BY COALESCE(publish_at,created_at) DESC LIMIT 24`)
	if err != nil {
		return []newsEntry{}
	}
	defer rows.Close()
	out := []newsEntry{}
	for rows.Next() {
		var n newsEntry
		if rows.Scan(&n.ID, &n.Title, &n.Summary, &n.URL, &n.Kind, &n.PublishAt, &n.ExpiresAt, &n.Active) == nil {
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	if r.Method == http.MethodGet {
		jsonOut(w, 200, map[string]any{"settings": s.runtimeSettings(r)})
		return
	}
	var in siteSettings
	if !decode(w, r, &in) {
		return
	}
	if err := validateSiteSettings(in); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		s.mock.settings = in
		s.mock.mu.Unlock()
	} else {
		raw, _ := json.Marshal(in)
		if _, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_settings(setting_key,setting_value) VALUES(?,?) ON DUPLICATE KEY UPDATE setting_value=VALUES(setting_value)`, s.siteSettingsKey(), string(raw)); err != nil {
			problem(w, 500, "Could not save settings")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'settings.update',?,?)", a.ID, s.siteSettingsKey(), "Runtime portal configuration updated")
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func validateSiteSettings(in siteSettings) error {
	for _, v := range []string{in.PortalName, in.RealmName, in.Tagline, in.RealmAddress, in.ExperienceRate, in.MaintenanceMessage} {
		if len(v) > 500 {
			return fmt.Errorf("configuration text is too long")
		}
	}
	if strings.TrimSpace(in.PortalName) == "" || strings.TrimSpace(in.RealmName) == "" || strings.TrimSpace(in.RealmAddress) == "" {
		return fmt.Errorf("portal name, realm name, and realm address are required")
	}
	if strings.TrimSpace(in.ExperienceRate) == "" || len(in.ExperienceRate) > 30 {
		return fmt.Errorf("experience rate is required and must be at most 30 characters")
	}
	if len(in.BrandMark) < 1 || len(in.BrandMark) > 3 {
		return fmt.Errorf("brand mark must contain 1–3 characters")
	}
	for _, v := range []string{in.ThemePrimary, in.ThemeSecondary, in.ThemeAccent, in.ThemeBackground} {
		if ok, _ := regexp.MatchString(`^#[0-9A-Fa-f]{6}$`, v); !ok {
			return fmt.Errorf("theme colors must use #RRGGBB")
		}
	}
	for _, v := range []string{in.DownloadURL, in.CommunityURL, in.TermsURL, in.PrivacyURL} {
		if v != "" {
			u, e := url.ParseRequestURI(v)
			if e != nil || (strings.HasPrefix(v, "//")) || (!strings.HasPrefix(v, "/") && (u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"))) {
				return fmt.Errorf("links must be root-relative or HTTP(S) URLs")
			}
		}
	}
	if in.MaintenanceStarts != nil && in.MaintenanceEnds != nil && !in.MaintenanceEnds.After(*in.MaintenanceStarts) {
		return fmt.Errorf("maintenance end must be after its start")
	}
	return nil
}

func (s *Server) adminNews(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			out := append([]newsEntry(nil), s.mock.news...)
			s.mock.mu.Unlock()
			jsonOut(w, 200, map[string]any{"news": out})
			return
		}
		rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,title,summary,url,kind,publish_at,expires_at,active FROM portal_news ORDER BY id DESC LIMIT 100")
		if e != nil {
			problem(w, 500, "Could not load news")
			return
		}
		defer rows.Close()
		out := []newsEntry{}
		for rows.Next() {
			var n newsEntry
			if rows.Scan(&n.ID, &n.Title, &n.Summary, &n.URL, &n.Kind, &n.PublishAt, &n.ExpiresAt, &n.Active) == nil {
				out = append(out, n)
			}
		}
		jsonOut(w, 200, map[string]any{"news": out})
		return
	}
	var n newsEntry
	if !decode(w, r, &n) {
		return
	}
	if err := validateNews(n); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		n.ID = uint64(len(s.mock.news) + 1)
		s.mock.news = append([]newsEntry{n}, s.mock.news...)
		s.mock.mu.Unlock()
	} else {
		res, e := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_news(title,summary,url,kind,publish_at,expires_at,active,created_by) VALUES(?,?,?,?,?,?,?,?)", n.Title, n.Summary, n.URL, n.Kind, n.PublishAt, n.ExpiresAt, n.Active, a.ID)
		if e != nil {
			problem(w, 500, "Could not create news")
			return
		}
		id, _ := res.LastInsertId()
		n.ID = uint64(id)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'news.create',?,?)", a.ID, strconv.FormatUint(n.ID, 10), n.Title)
	}
	jsonOut(w, 201, n)
}

func validateNews(n newsEntry) error {
	n.Title = strings.TrimSpace(n.Title)
	if n.Title == "" || len(n.Title) > 120 || len(n.Summary) > 1000 || len(n.URL) > 500 {
		return fmt.Errorf("title is required and news fields must fit their limits")
	}
	if n.Kind != "news" && n.Kind != "announcement" && n.Kind != "maintenance" {
		return fmt.Errorf("invalid news type")
	}
	if n.ExpiresAt != nil && n.PublishAt != nil && !n.ExpiresAt.After(*n.PublishAt) {
		return fmt.Errorf("expiry must be after publication")
	}
	if n.URL != "" {
		u, e := url.ParseRequestURI(n.URL)
		if e != nil || strings.HasPrefix(n.URL, "//") || (!strings.HasPrefix(n.URL, "/") && (u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"))) {
			return fmt.Errorf("invalid news URL")
		}
	}
	return nil
}

func (s *Server) adminNewsItem(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if e != nil {
		problem(w, 400, "Invalid news item")
		return
	}
	if r.Method == http.MethodDelete {
		if s.c.MockMode {
			s.mock.mu.Lock()
			for i := range s.mock.news {
				if s.mock.news[i].ID == id {
					s.mock.news[i].Active = false
				}
			}
			s.mock.mu.Unlock()
		} else {
			res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_news SET active=0 WHERE id=?", id)
			if e != nil {
				problem(w, 500, "Could not archive news")
				return
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				problem(w, 404, "News item not found")
				return
			}
			_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'news.archive',?)", a.ID, strconv.FormatUint(id, 10))
		}
		jsonOut(w, 200, map[string]bool{"ok": true})
		return
	}
	var n newsEntry
	if !decode(w, r, &n) {
		return
	}
	if err := validateNews(n); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		for i := range s.mock.news {
			if s.mock.news[i].ID == id {
				n.ID = id
				s.mock.news[i] = n
			}
		}
		s.mock.mu.Unlock()
	} else {
		res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_news SET title=?,summary=?,url=?,kind=?,publish_at=?,expires_at=?,active=? WHERE id=?", n.Title, n.Summary, n.URL, n.Kind, n.PublishAt, n.ExpiresAt, n.Active, id)
		if e != nil {
			problem(w, 500, "Could not update news")
			return
		}
		changed, _ := res.RowsAffected()
		if changed == 0 {
			problem(w, 404, "News item not found")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'news.update',?,?)", a.ID, strconv.FormatUint(id, 10), n.Title)
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
