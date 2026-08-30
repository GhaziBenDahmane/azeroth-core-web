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
	PortalName                string     `json:"portalName"`
	RealmName                 string     `json:"realmName"`
	BrandMark                 string     `json:"brandMark"`
	Tagline                   string     `json:"tagline"`
	HomeHeadline              string     `json:"homeHeadline"`
	HomeEyebrow               string     `json:"homeEyebrow"`
	HomePrimaryCTA            string     `json:"homePrimaryCta"`
	HomeConnectTitle          string     `json:"homeConnectTitle"`
	HomeGuideText             string     `json:"homeGuideText"`
	HomeRules                 string     `json:"homeRules"`
	DiscordStatus             string     `json:"discordStatus"`
	HomeChangelog             string     `json:"homeChangelog"`
	HomeFeatures              string     `json:"homeFeatures"`
	HomeProgression           string     `json:"homeProgression"`
	FeaturedNewsID            uint64     `json:"featuredNewsId"`
	RealmAddress              string     `json:"realmAddress"`
	ExperienceRate            string     `json:"experienceRate"`
	RealmType                 string     `json:"realmType"`
	RealmTimezone             string     `json:"realmTimezone"`
	RealmDescription          string     `json:"realmDescription"`
	SeasonName                string     `json:"seasonName"`
	ArenaRewardPolicy         string     `json:"arenaRewardPolicy"`
	QuestExperienceRate       string     `json:"questExperienceRate"`
	KillExperienceRate        string     `json:"killExperienceRate"`
	ExplorationExperienceRate string     `json:"explorationExperienceRate"`
	DropRate                  string     `json:"dropRate"`
	ReputationRate            string     `json:"reputationRate"`
	HonorRate                 string     `json:"honorRate"`
	ProfessionRate            string     `json:"professionRate"`
	FactionPolicy             string     `json:"factionPolicy"`
	StartLevel                uint8      `json:"startLevel"`
	MaxLevel                  uint8      `json:"maxLevel"`
	PopulationCap             uint32     `json:"populationCap"`
	TransferSLAHours          uint32     `json:"transferSlaHours"`
	CrossFaction              *bool      `json:"crossFaction"`
	CrossFactionAccounts      *bool      `json:"crossFactionAccounts"`
	CrossFactionCalendar      *bool      `json:"crossFactionCalendar"`
	CrossFactionChannels      *bool      `json:"crossFactionChannels"`
	CrossFactionGroups        *bool      `json:"crossFactionGroups"`
	CrossFactionGuilds        *bool      `json:"crossFactionGuilds"`
	CrossFactionAuctions      *bool      `json:"crossFactionAuctions"`
	CrossFactionMail          *bool      `json:"crossFactionMail"`
	CrossFactionWho           *bool      `json:"crossFactionWho"`
	CrossFactionFriends       *bool      `json:"crossFactionFriends"`
	CrossFactionTrade         *bool      `json:"crossFactionTrade"`
	DownloadURL               string     `json:"downloadUrl"`
	CommunityURL              string     `json:"communityUrl"`
	TermsURL                  string     `json:"termsUrl"`
	PrivacyURL                string     `json:"privacyUrl"`
	LogoURL                   string     `json:"logoUrl"`
	HeroImageURL              string     `json:"heroImageUrl"`
	FaviconURL                string     `json:"faviconUrl"`
	ThemePrimary              string     `json:"themePrimary"`
	ThemeSecondary            string     `json:"themeSecondary"`
	ThemeAccent               string     `json:"themeAccent"`
	ThemeBackground           string     `json:"themeBackground"`
	MaintenanceEnabled        bool       `json:"maintenanceEnabled"`
	MaintenanceMessage        string     `json:"maintenanceMessage"`
	MaintenanceStarts         *time.Time `json:"maintenanceStarts,omitempty"`
	MaintenanceEnds           *time.Time `json:"maintenanceEnds,omitempty"`
	Registration              *bool      `json:"registration"`
	Armory                    *bool      `json:"armory"`
	Rankings                  *bool      `json:"rankings"`
	Guilds                    *bool      `json:"guilds"`
	Realm                     *bool      `json:"realm"`
	Shop                      *bool      `json:"shop"`
	Support                   *bool      `json:"support"`
	Admin                     *bool      `json:"admin"`
	GMConsole                 *bool      `json:"gmConsole"`
}

type newsEntry struct {
	ID         uint64     `json:"id"`
	Title      string     `json:"title"`
	Slug       string     `json:"slug"`
	Summary    string     `json:"summary"`
	Body       string     `json:"body"`
	URL        string     `json:"url"`
	CoverURL   string     `json:"coverUrl"`
	Tags       string     `json:"tags"`
	AuthorName string     `json:"authorName"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	PublishAt  *time.Time `json:"publishAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	Active     bool       `json:"active"`
	Featured   bool       `json:"featured"`
	CreatedBy  uint32     `json:"createdBy,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

type newsRevision struct {
	ID        uint64    `json:"id"`
	NewsID    uint64    `json:"newsId"`
	EditorID  uint32    `json:"editorId"`
	Snapshot  newsEntry `json:"snapshot"`
	CreatedAt time.Time `json:"createdAt"`
}

const newsSelect = `id,title,COALESCE(slug,''),summary,body,url,cover_url,tags,author_name,kind,status,publish_at,expires_at,active,created_by,created_at,updated_at`

type rowScanner interface{ Scan(...any) error }

func scanNews(row rowScanner, n *newsEntry) error {
	return row.Scan(&n.ID, &n.Title, &n.Slug, &n.Summary, &n.Body, &n.URL, &n.CoverURL, &n.Tags, &n.AuthorName, &n.Kind, &n.Status, &n.PublishAt, &n.ExpiresAt, &n.Active, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt)
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
	AllowSale       bool       `json:"allowSale"`
	MinSubtotal     uint32     `json:"minSubtotal"`
	Category        string     `json:"category"`
	Uses            uint32     `json:"uses,omitempty"`
}

var couponCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,39}$`)

func boolPtr(v bool) *bool { return &v }

func (s *Server) siteSettingsKey() string { return "site_config:" + s.c.RealmKey }

func (s *Server) defaultSiteSettings() siteSettings {
	realmType, timezone, faction := s.c.RealmType, s.c.RealmTimezone, s.c.FactionPolicy
	if realmType == "" {
		realmType = "PvE"
	}
	if timezone == "" {
		timezone = "UTC"
	}
	if faction == "" {
		faction = "both"
	}
	overallRate := s.c.ExperienceRate
	if overallRate == "" {
		overallRate = "1×"
	}
	rate := func(value string) string {
		if value == "" {
			return overallRate
		}
		return value
	}
	startLevel, maxLevel := s.c.StartLevel, s.c.MaxLevel
	if startLevel == 0 {
		startLevel = 1
	}
	if maxLevel == 0 {
		maxLevel = 80
	}
	transferSLAHours := s.c.TransferSLAHours
	if transferSLAHours <= 0 {
		transferSLAHours = 72
	}
	homeFeatures, homeProgression := s.c.HomeFeatures, s.c.HomeProgression
	if s.c.MockMode && strings.TrimSpace(homeFeatures) == "" {
		homeFeatures = "Faithful 3.3.5a|Wrath systems, items, talents, and progression use the level 80 expansion data.\nPlayer-first operations|Live status, transparent rankings, support, and audited delivery are built into the portal.\nCommunity progression|Guild recruitment, events, voting, and public feedback keep the realm active."
	}
	if s.c.MockMode && strings.TrimSpace(homeProgression) == "" {
		homeProgression = "Naxxramas|Complete|The first Northrend raid tier has been conquered.\nUlduar|Live|Hard modes and raid-speed rankings are active now.\nTrial of the Crusader|Next|The next tier opens after the current progression window.\nIcecrown Citadel|Planned|The Lich King awaits at the end of the season."
	}
	return siteSettings{PortalName: s.c.PortalName, RealmName: s.c.RealmName, BrandMark: s.c.BrandMark, Tagline: s.c.PortalTagline,
		HomeHeadline: s.c.HomeHeadline, HomeEyebrow: s.c.HomeEyebrow, HomePrimaryCTA: s.c.HomePrimaryCTA, HomeConnectTitle: s.c.HomeConnectTitle,
		HomeGuideText: s.c.HomeGuideText, HomeRules: s.c.HomeRules, DiscordStatus: s.c.DiscordStatus, HomeChangelog: s.c.HomeChangelog,
		HomeFeatures: homeFeatures, HomeProgression: homeProgression,
		RealmAddress: s.c.RealmAddress, ExperienceRate: overallRate, RealmType: realmType, RealmTimezone: timezone, RealmDescription: s.c.RealmDescription, SeasonName: s.c.SeasonName, ArenaRewardPolicy: s.c.ArenaRewardPolicy,
		QuestExperienceRate: rate(s.c.QuestExperienceRate), KillExperienceRate: rate(s.c.KillExperienceRate), ExplorationExperienceRate: rate(s.c.ExplorationExperienceRate), DropRate: rate(s.c.DropRate), ReputationRate: rate(s.c.ReputationRate), HonorRate: rate(s.c.HonorRate), ProfessionRate: rate(s.c.ProfessionRate),
		FactionPolicy: faction, StartLevel: uint8(startLevel), MaxLevel: uint8(maxLevel), PopulationCap: uint32(s.c.PopulationCap), TransferSLAHours: uint32(transferSLAHours), CrossFaction: boolPtr(s.c.CrossFaction),
		CrossFactionAccounts: boolPtr(s.c.CrossFactionAccounts), CrossFactionCalendar: boolPtr(s.c.CrossFactionCalendar), CrossFactionChannels: boolPtr(s.c.CrossFactionChannels), CrossFactionGroups: boolPtr(s.c.CrossFactionGroups),
		CrossFactionGuilds: boolPtr(s.c.CrossFactionGuilds), CrossFactionAuctions: boolPtr(s.c.CrossFactionAuctions), CrossFactionMail: boolPtr(s.c.CrossFactionMail), CrossFactionWho: boolPtr(s.c.CrossFactionWho), CrossFactionFriends: boolPtr(s.c.CrossFactionFriends), CrossFactionTrade: boolPtr(s.c.CrossFactionTrade),
		DownloadURL: s.c.DownloadURL, CommunityURL: s.c.CommunityURL, TermsURL: s.c.TermsURL, PrivacyURL: s.c.PrivacyURL,
		LogoURL: s.c.LogoURL, HeroImageURL: s.c.HeroImageURL, FaviconURL: s.c.FaviconURL,
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
			if strings.TrimSpace(stored.HomeEyebrow) == "" {
				stored.HomeEyebrow = base.HomeEyebrow
			}
			if strings.TrimSpace(stored.HomePrimaryCTA) == "" {
				stored.HomePrimaryCTA = base.HomePrimaryCTA
			}
			if strings.TrimSpace(stored.HomeConnectTitle) == "" {
				stored.HomeConnectTitle = base.HomeConnectTitle
			}
			if strings.TrimSpace(stored.HomeGuideText) == "" {
				stored.HomeGuideText = base.HomeGuideText
			}
			if stored.RealmType == "" {
				stored.RealmType, stored.RealmTimezone, stored.QuestExperienceRate, stored.KillExperienceRate = base.RealmType, base.RealmTimezone, base.QuestExperienceRate, base.KillExperienceRate
				stored.ExplorationExperienceRate, stored.DropRate, stored.ReputationRate, stored.HonorRate, stored.ProfessionRate = base.ExplorationExperienceRate, base.DropRate, base.ReputationRate, base.HonorRate, base.ProfessionRate
				stored.FactionPolicy, stored.StartLevel, stored.MaxLevel, stored.PopulationCap, stored.CrossFaction = base.FactionPolicy, base.StartLevel, base.MaxLevel, base.PopulationCap, base.CrossFaction
			}
			if stored.CrossFactionGroups == nil {
				legacy := settingBool(stored.CrossFaction, false)
				stored.CrossFactionAccounts, stored.CrossFactionCalendar, stored.CrossFactionChannels, stored.CrossFactionGroups = boolPtr(legacy), boolPtr(legacy), boolPtr(legacy), boolPtr(legacy)
				stored.CrossFactionGuilds, stored.CrossFactionAuctions, stored.CrossFactionMail, stored.CrossFactionWho = boolPtr(legacy), boolPtr(legacy), boolPtr(legacy), boolPtr(legacy)
				stored.CrossFactionFriends, stored.CrossFactionTrade = boolPtr(legacy), boolPtr(legacy)
			}
			if stored.TransferSLAHours == 0 {
				stored.TransferSLAHours = base.TransferSLAHours
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
		now := time.Now()
		out := []newsEntry{}
		for _, item := range s.mock.news {
			if item.Status == "published" && item.Active && (item.PublishAt == nil || !now.Before(*item.PublishAt)) && (item.ExpiresAt == nil || now.Before(*item.ExpiresAt)) {
				item.CreatedBy = 0
				out = append(out, item)
			}
		}
		return out
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT `+newsSelect+` FROM portal_news WHERE realm_key=? AND status='published' AND active=1 AND (publish_at IS NULL OR publish_at<=NOW()) AND (expires_at IS NULL OR expires_at>NOW()) ORDER BY COALESCE(publish_at,created_at) DESC LIMIT 24`, s.c.RealmKey)
	if err != nil {
		return []newsEntry{}
	}
	defer rows.Close()
	out := []newsEntry{}
	for rows.Next() {
		var n newsEntry
		if scanNews(rows, &n) == nil {
			n.CreatedBy = 0
			out = append(out, n)
		}
	}
	return out
}

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "settings")
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
	current := s.runtimeSettings(r)
	if in.RealmType == "" {
		in.RealmType, in.RealmTimezone, in.RealmDescription, in.SeasonName = current.RealmType, current.RealmTimezone, current.RealmDescription, current.SeasonName
		in.QuestExperienceRate, in.KillExperienceRate, in.ExplorationExperienceRate = current.QuestExperienceRate, current.KillExperienceRate, current.ExplorationExperienceRate
		in.DropRate, in.ReputationRate, in.HonorRate, in.ProfessionRate = current.DropRate, current.ReputationRate, current.HonorRate, current.ProfessionRate
		in.FactionPolicy, in.StartLevel, in.MaxLevel, in.PopulationCap = current.FactionPolicy, current.StartLevel, current.MaxLevel, current.PopulationCap
		in.TransferSLAHours = current.TransferSLAHours
	}
	if in.CrossFaction == nil {
		in.CrossFaction = current.CrossFaction
	}
	if in.CrossFactionGroups == nil {
		in.CrossFactionAccounts, in.CrossFactionCalendar, in.CrossFactionChannels, in.CrossFactionGroups = current.CrossFactionAccounts, current.CrossFactionCalendar, current.CrossFactionChannels, current.CrossFactionGroups
		in.CrossFactionGuilds, in.CrossFactionAuctions, in.CrossFactionMail, in.CrossFactionWho = current.CrossFactionGuilds, current.CrossFactionAuctions, current.CrossFactionMail, current.CrossFactionWho
		in.CrossFactionFriends, in.CrossFactionTrade = current.CrossFactionFriends, current.CrossFactionTrade
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
	if maintenanceSettingsChanged(current, in) {
		title, message := "Realm maintenance cancelled", "The previously announced maintenance window has been cancelled."
		if in.MaintenanceEnabled {
			title, message = "Realm maintenance scheduled", strings.TrimSpace(in.MaintenanceMessage)
			if message == "" {
				message = "Scheduled realm maintenance has been announced."
			}
			if in.MaintenanceStarts != nil {
				message += " Starts " + in.MaintenanceStarts.UTC().Format("2 Jan 2006 15:04 UTC") + "."
			}
			if in.MaintenanceEnds != nil {
				message += " Expected completion " + in.MaintenanceEnds.UTC().Format("2 Jan 2006 15:04 UTC") + "."
			}
		}
		s.notifyAllAccounts(r.Context(), "maintenance", title, message, "/")
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func maintenanceSettingsChanged(before, after siteSettings) bool {
	if before.MaintenanceEnabled != after.MaintenanceEnabled {
		return true
	}
	if !after.MaintenanceEnabled {
		return false
	}
	equalTime := func(left, right *time.Time) bool {
		if left == nil || right == nil {
			return left == nil && right == nil
		}
		return left.Equal(*right)
	}
	return before.MaintenanceMessage != after.MaintenanceMessage || !equalTime(before.MaintenanceStarts, after.MaintenanceStarts) || !equalTime(before.MaintenanceEnds, after.MaintenanceEnds)
}

func validateSiteSettings(in siteSettings) error {
	for _, v := range []string{in.PortalName, in.RealmName, in.Tagline, in.HomeHeadline, in.HomeEyebrow, in.HomePrimaryCTA, in.HomeConnectTitle, in.HomeGuideText, in.HomeRules, in.DiscordStatus, in.HomeChangelog, in.RealmAddress, in.ExperienceRate, in.RealmDescription, in.SeasonName, in.MaintenanceMessage} {
		if len(v) > 500 {
			return fmt.Errorf("configuration text is too long")
		}
	}
	if len(in.HomeFeatures) > 4000 || len(in.HomeProgression) > 4000 {
		return fmt.Errorf("homepage feature and progression content must be at most 4000 characters")
	}
	if len(in.ArenaRewardPolicy) > 2000 {
		return fmt.Errorf("arena reward policy must be at most 2000 characters")
	}
	if strings.TrimSpace(in.PortalName) == "" || strings.TrimSpace(in.RealmName) == "" || strings.TrimSpace(in.RealmAddress) == "" {
		return fmt.Errorf("portal name, realm name, and realm address are required")
	}
	if strings.TrimSpace(in.ExperienceRate) == "" || len(in.ExperienceRate) > 30 {
		return fmt.Errorf("experience rate is required and must be at most 30 characters")
	}
	validRealmTypes := map[string]bool{"PvE": true, "PvP": true, "RP": true, "RP-PvP": true}
	validFactionPolicies := map[string]bool{"both": true, "alliance": true, "horde": true}
	if !validRealmTypes[in.RealmType] || !validFactionPolicies[in.FactionPolicy] || in.StartLevel < 1 || in.MaxLevel < in.StartLevel || in.MaxLevel > 80 || in.PopulationCap > 1_000_000 || in.TransferSLAHours < 1 || in.TransferSLAHours > 8760 || len(in.RealmTimezone) > 80 {
		return fmt.Errorf("realm type, faction policy, level range, population cap, or timezone is invalid")
	}
	for _, rate := range []string{in.QuestExperienceRate, in.KillExperienceRate, in.ExplorationExperienceRate, in.DropRate, in.ReputationRate, in.HonorRate, in.ProfessionRate} {
		if strings.TrimSpace(rate) == "" || len(rate) > 30 {
			return fmt.Errorf("all displayed realm rates are required and must be at most 30 characters")
		}
	}
	if len(in.BrandMark) < 1 || len(in.BrandMark) > 3 {
		return fmt.Errorf("brand mark must contain 1–3 characters")
	}
	for _, v := range []string{in.ThemePrimary, in.ThemeSecondary, in.ThemeAccent, in.ThemeBackground} {
		if ok, _ := regexp.MatchString(`^#[0-9A-Fa-f]{6}$`, v); !ok {
			return fmt.Errorf("theme colors must use #RRGGBB")
		}
	}
	for _, v := range []string{in.DownloadURL, in.CommunityURL, in.TermsURL, in.PrivacyURL, in.LogoURL, in.HeroImageURL, in.FaviconURL} {
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
	a, ok := s.requireStaffPermission(r, "content")
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
		rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT "+newsSelect+" FROM portal_news WHERE realm_key=? ORDER BY id DESC LIMIT 100", s.c.RealmKey)
		if e != nil {
			problem(w, 500, "Could not load news")
			return
		}
		defer rows.Close()
		out := []newsEntry{}
		for rows.Next() {
			var n newsEntry
			if scanNews(rows, &n) == nil {
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
	if err := validateNews(&n); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		n.ID = uint64(len(s.mock.news) + 1)
		s.mock.news = append([]newsEntry{n}, s.mock.news...)
		s.mock.mu.Unlock()
	} else {
		res, e := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_news(title,slug,summary,body,url,cover_url,tags,author_name,kind,status,publish_at,expires_at,active,created_by,realm_key) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", n.Title, nullableSlug(n.Slug), n.Summary, n.Body, n.URL, n.CoverURL, n.Tags, n.AuthorName, n.Kind, n.Status, n.PublishAt, n.ExpiresAt, n.Active, a.ID, s.c.RealmKey)
		if e != nil {
			problem(w, 500, "Could not create news")
			return
		}
		id, _ := res.LastInsertId()
		n.ID = uint64(id)
		_ = s.saveNewsRevision(r, n, a.ID)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'news.create',?,?)", a.ID, strconv.FormatUint(n.ID, 10), n.Title)
	}
	jsonOut(w, 201, n)
}

func validateNews(n *newsEntry) error {
	n.Title = strings.TrimSpace(n.Title)
	n.Slug = strings.Trim(strings.ToLower(strings.TrimSpace(n.Slug)), "-")
	if n.Slug == "" {
		n.Slug = articleSlug(n.Title)
	}
	n.Status = strings.ToLower(strings.TrimSpace(n.Status))
	if n.Status == "" {
		if n.Active {
			n.Status = "published"
		} else {
			n.Status = "draft"
		}
	}
	n.Active = n.Status == "published"
	if n.Title == "" || len(n.Title) > 120 || len(n.Slug) > 160 || len(n.Summary) > 1000 || len(n.Body) > 100000 || len(n.URL) > 500 || len(n.CoverURL) > 500 || len(n.Tags) > 500 || len(n.AuthorName) > 100 {
		return fmt.Errorf("title is required and news fields must fit their limits")
	}
	if n.Slug == "" || !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(n.Slug) {
		return fmt.Errorf("slug must contain lowercase letters, numbers, and hyphens")
	}
	if n.Status != "draft" && n.Status != "published" && n.Status != "archived" {
		return fmt.Errorf("invalid article status")
	}
	if n.Kind != "news" && n.Kind != "announcement" && n.Kind != "maintenance" && n.Kind != "changelog" {
		return fmt.Errorf("invalid news type")
	}
	if n.ExpiresAt != nil && n.PublishAt != nil && !n.ExpiresAt.After(*n.PublishAt) {
		return fmt.Errorf("expiry must be after publication")
	}
	for _, link := range []string{n.URL, n.CoverURL} {
		if link == "" {
			continue
		}
		u, e := url.ParseRequestURI(link)
		if e != nil || strings.HasPrefix(link, "//") || (!strings.HasPrefix(link, "/") && (u.Host == "" || (u.Scheme != "http" && u.Scheme != "https"))) {
			return fmt.Errorf("invalid news URL")
		}
	}
	return nil
}

func (s *Server) adminNewsItem(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
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
					s.mock.news[i].Status = "archived"
				}
			}
			s.mock.mu.Unlock()
		} else {
			res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_news SET active=0,status='archived' WHERE id=? AND realm_key=?", id, s.c.RealmKey)
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
	if err := validateNews(&n); err != nil {
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
		res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_news SET title=?,slug=?,summary=?,body=?,url=?,cover_url=?,tags=?,author_name=?,kind=?,status=?,publish_at=?,expires_at=?,active=? WHERE id=? AND realm_key=?", n.Title, nullableSlug(n.Slug), n.Summary, n.Body, n.URL, n.CoverURL, n.Tags, n.AuthorName, n.Kind, n.Status, n.PublishAt, n.ExpiresAt, n.Active, id, s.c.RealmKey)
		if e != nil {
			problem(w, 500, "Could not update news")
			return
		}
		changed, _ := res.RowsAffected()
		if changed == 0 {
			problem(w, 404, "News item not found")
			return
		}
		n.ID = id
		_ = s.saveNewsRevision(r, n, a.ID)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'news.update',?,?)", a.ID, strconv.FormatUint(id, 10), n.Title)
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func articleSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if len(value) > 150 {
		value = strings.Trim(value[:150], "-")
	}
	return value
}

func nullableSlug(slug string) any {
	if slug == "" {
		return nil
	}
	return slug
}

func (s *Server) saveNewsRevision(r *http.Request, n newsEntry, editorID uint32) error {
	_, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_news_revisions(news_id,realm_key,editor_account_id,title,slug,summary,body,url,cover_url,tags,author_name,kind,status,publish_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, n.ID, s.c.RealmKey, editorID, n.Title, nullableSlug(n.Slug), n.Summary, n.Body, n.URL, n.CoverURL, n.Tags, n.AuthorName, n.Kind, n.Status, n.PublishAt, n.ExpiresAt)
	return err
}
