package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type guildRecruitment struct {
	GuildID     uint32    `json:"guildId"`
	GuildName   string    `json:"guildName"`
	Headline    string    `json:"headline"`
	Description string    `json:"description"`
	LookingFor  string    `json:"lookingFor"`
	Schedule    string    `json:"schedule"`
	Contact     string    `json:"contact"`
	DiscordURL  string    `json:"discordUrl,omitempty"`
	Active      bool      `json:"active"`
	UpdatedBy   uint32    `json:"updatedBy,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type guildApplication struct {
	ID            uint64    `json:"id"`
	GuildID       uint32    `json:"guildId"`
	GuildName     string    `json:"guildName"`
	AccountID     uint32    `json:"accountId,omitempty"`
	Username      string    `json:"username,omitempty"`
	CharacterGUID uint32    `json:"characterGuid"`
	CharacterName string    `json:"characterName"`
	Message       string    `json:"message"`
	Status        string    `json:"status"`
	Response      string    `json:"response,omitempty"`
	StaffNote     string    `json:"staffNote,omitempty"`
	HandledBy     uint32    `json:"handledBy,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

var guildApplicationStatuses = map[string]bool{"submitted": true, "reviewing": true, "accepted": true, "declined": true, "withdrawn": true}

func ensureMockRecruitmentLocked(state *mockState) {
	if len(state.guildRecruitment) != 0 {
		return
	}
	state.guildRecruitment = []guildRecruitment{{GuildID: 1, GuildName: "Keepers of Dawn", Headline: "Progress through Northrend with a steady team", Description: "A friendly progression guild building consistent 10-player and 25-player rosters.", LookingFor: "Healers, ranged damage, and reliable off-tanks", Schedule: "Thursday and Sunday · 20:00–23:00 realm time", Contact: "Speak with Arthoria in game", DiscordURL: "https://discord.gg/example", Active: true, UpdatedAt: time.Now().Add(-12 * time.Hour)}}
}

func validateGuildRecruitment(profile *guildRecruitment) error {
	profile.Headline = strings.TrimSpace(profile.Headline)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.LookingFor = strings.TrimSpace(profile.LookingFor)
	profile.Schedule = strings.TrimSpace(profile.Schedule)
	profile.Contact = strings.TrimSpace(profile.Contact)
	profile.DiscordURL = strings.TrimSpace(profile.DiscordURL)
	if profile.GuildID == 0 || len(profile.Headline) < 5 || len(profile.Headline) > 160 || len(profile.Description) < 20 || len(profile.Description) > 10000 || len(profile.LookingFor) > 500 || len(profile.Schedule) > 300 || len(profile.Contact) > 300 {
		return fmt.Errorf("choose a guild and provide a headline plus a 20–10000 character description")
	}
	if profile.Contact != "" {
		if parsed, err := url.Parse(profile.Contact); err == nil && parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return fmt.Errorf("contact links must use HTTP or HTTPS")
		}
	}
	if profile.DiscordURL != "" {
		parsed, err := url.ParseRequestURI(profile.DiscordURL)
		host := ""
		if parsed != nil {
			host = strings.ToLower(parsed.Hostname())
		}
		if err != nil || parsed.Scheme != "https" || (host != "discord.gg" && host != "discord.com" && host != "www.discord.com") {
			return fmt.Errorf("Discord invite must be an HTTPS discord.gg or discord.com URL")
		}
	}
	return nil
}

func (s *Server) guildRecruitmentProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid guild")
		return
	}
	viewer, _ := s.trackerAccount(r)
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockRecruitmentLocked(s.mock)
		for _, profile := range s.mock.guildRecruitment {
			if profile.GuildID != uint32(id) || !profile.Active {
				continue
			}
			var application *guildApplication
			if viewer.ID != 0 {
				for index := range s.mock.guildApplications {
					if s.mock.guildApplications[index].GuildID == uint32(id) && s.mock.guildApplications[index].AccountID == viewer.ID && s.mock.guildApplications[index].Status != "withdrawn" {
						copy := s.mock.guildApplications[index]
						copy.StaffNote = ""
						application = &copy
						break
					}
				}
			}
			jsonOut(w, http.StatusOK, map[string]any{"recruitment": profile, "application": application})
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"recruitment": nil, "application": nil})
		return
	}
	var profile guildRecruitment
	err = s.s.Auth.QueryRowContext(r.Context(), `SELECT guild_id,headline,description,looking_for,schedule,contact,discord_url,active,updated_by,updated_at FROM portal_guild_recruitment WHERE realm_key=? AND guild_id=? AND active=1`, s.c.RealmKey, id).Scan(&profile.GuildID, &profile.Headline, &profile.Description, &profile.LookingFor, &profile.Schedule, &profile.Contact, &profile.DiscordURL, &profile.Active, &profile.UpdatedBy, &profile.UpdatedAt)
	if err != nil {
		jsonOut(w, http.StatusOK, map[string]any{"recruitment": nil, "application": nil})
		return
	}
	_ = s.s.Characters.QueryRowContext(r.Context(), fmt.Sprintf("SELECT name FROM %s.guild WHERE guildid=?", s.c.CharactersDB), id).Scan(&profile.GuildName)
	var application *guildApplication
	if viewer.ID != 0 {
		var item guildApplication
		err = s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf(`SELECT ga.id,ga.guild_id,COALESCE(g.name,''),ga.account_id,COALESCE(a.username,''),ga.character_guid,COALESCE(c.name,''),ga.message,ga.status,ga.response,'',ga.handled_by,ga.created_at,ga.updated_at FROM portal_guild_applications ga LEFT JOIN %s.account a ON a.id=ga.account_id LEFT JOIN %s.characters c ON c.guid=ga.character_guid LEFT JOIN %s.guild g ON g.guildid=ga.guild_id WHERE ga.realm_key=? AND ga.guild_id=? AND ga.account_id=? AND ga.status<>'withdrawn' ORDER BY ga.id DESC LIMIT 1`, s.c.AuthDB, s.c.CharactersDB, s.c.CharactersDB), s.c.RealmKey, id, viewer.ID).Scan(&item.ID, &item.GuildID, &item.GuildName, &item.AccountID, &item.Username, &item.CharacterGUID, &item.CharacterName, &item.Message, &item.Status, &item.Response, &item.StaffNote, &item.HandledBy, &item.CreatedAt, &item.UpdatedAt)
		if err == nil {
			application = &item
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"recruitment": profile, "application": application})
}

func (s *Server) createGuildApplication(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in to apply")
		return
	}
	guildID, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid guild")
		return
	}
	var in struct {
		CharacterGUID uint32 `json:"characterGuid"`
		Message       string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if in.CharacterGUID == 0 || len(in.Message) < 20 || len(in.Message) > 2000 {
		problem(w, http.StatusUnprocessableEntity, "Choose a character and provide a 20–2000 character message")
		return
	}
	if s.c.MockMode {
		validCharacter, characterName := false, ""
		for _, character := range mockCharacters {
			if character.GUID == in.CharacterGUID {
				if character.Guild != "" {
					problem(w, http.StatusConflict, "Leave your current guild before applying")
					return
				}
				validCharacter, characterName = true, character.Name
				break
			}
		}
		if !validCharacter {
			problem(w, http.StatusUnprocessableEntity, "Choose one of your characters")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockRecruitmentLocked(s.mock)
		active := false
		guildName := ""
		for _, profile := range s.mock.guildRecruitment {
			if profile.GuildID == uint32(guildID) && profile.Active {
				active, guildName = true, profile.GuildName
			}
		}
		if !active {
			problem(w, http.StatusConflict, "This guild is not accepting applications")
			return
		}
		for _, application := range s.mock.guildApplications {
			if application.GuildID == uint32(guildID) && application.AccountID == a.ID && (application.Status == "submitted" || application.Status == "reviewing") {
				problem(w, http.StatusConflict, "You already have an active application to this guild")
				return
			}
		}
		id := uint64(len(s.mock.guildApplications) + 1)
		now := time.Now()
		s.mock.guildApplications = append([]guildApplication{{ID: id, GuildID: uint32(guildID), GuildName: guildName, AccountID: a.ID, Username: a.Username, CharacterGUID: in.CharacterGUID, CharacterName: characterName, Message: in.Message, Status: "submitted", CreatedAt: now, UpdatedAt: now}}, s.mock.guildApplications...)
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
		return
	}
	var characterName string
	var currentGuild uint32
	query := fmt.Sprintf(`SELECT c.name,COALESCE(gm.guildid,0) FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid WHERE c.guid=? AND c.account=? AND c.deleteDate IS NULL`, s.c.CharactersDB, s.c.CharactersDB)
	if err = s.s.Characters.QueryRowContext(r.Context(), query, in.CharacterGUID, a.ID).Scan(&characterName, &currentGuild); err != nil {
		problem(w, http.StatusUnprocessableEntity, "Choose one of your characters")
		return
	}
	if currentGuild != 0 {
		problem(w, http.StatusConflict, "Leave your current guild before applying")
		return
	}
	var active bool
	if err = s.s.Auth.QueryRowContext(r.Context(), "SELECT active FROM portal_guild_recruitment WHERE realm_key=? AND guild_id=?", s.c.RealmKey, guildID).Scan(&active); err != nil || !active {
		problem(w, http.StatusConflict, "This guild is not accepting applications")
		return
	}
	var existing uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_guild_applications WHERE realm_key=? AND guild_id=? AND account_id=? AND status IN ('submitted','reviewing')", s.c.RealmKey, guildID, a.ID).Scan(&existing)
	if existing > 0 {
		problem(w, http.StatusConflict, "You already have an active application to this guild")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_guild_applications(realm_key,guild_id,account_id,character_guid,message) VALUES(?,?,?,?,?)", s.c.RealmKey, guildID, a.ID, in.CharacterGUID, in.Message)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not submit guild application")
		return
	}
	id, _ := result.LastInsertId()
	s.notifyDiscordAsync("New guild application", "**%s** applied to guild %d with **%s**.", a.Username, guildID, characterName)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Server) guildApplications(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		items := []guildApplication{}
		for _, item := range s.mock.guildApplications {
			if item.AccountID == a.ID {
				item.StaffNote = ""
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"applications": items})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT ga.id,ga.guild_id,COALESCE(g.name,''),ga.account_id,'',ga.character_guid,COALESCE(c.name,''),ga.message,ga.status,ga.response,'',ga.handled_by,ga.created_at,ga.updated_at FROM portal_guild_applications ga LEFT JOIN %s.characters c ON c.guid=ga.character_guid LEFT JOIN %s.guild g ON g.guildid=ga.guild_id WHERE ga.account_id=? AND ga.realm_key=? ORDER BY ga.id DESC LIMIT 100`, s.c.CharactersDB, s.c.CharactersDB), a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load guild applications")
		return
	}
	defer rows.Close()
	items := []guildApplication{}
	for rows.Next() {
		var item guildApplication
		if rows.Scan(&item.ID, &item.GuildID, &item.GuildName, &item.AccountID, &item.Username, &item.CharacterGUID, &item.CharacterName, &item.Message, &item.Status, &item.Response, &item.StaffNote, &item.HandledBy, &item.CreatedAt, &item.UpdatedAt) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"applications": items})
}

func (s *Server) withdrawGuildApplication(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid application")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for index := range s.mock.guildApplications {
			item := &s.mock.guildApplications[index]
			if item.ID == id && item.AccountID == a.ID && (item.Status == "submitted" || item.Status == "reviewing") {
				item.Status = "withdrawn"
				item.UpdatedAt = time.Now()
				jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, http.StatusConflict, "Application cannot be withdrawn")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_guild_applications SET status='withdrawn' WHERE id=? AND account_id=? AND realm_key=? AND status IN ('submitted','reviewing')", id, a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not withdraw application")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusConflict, "Application cannot be withdrawn")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminGuildRecruitment(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			defer s.mock.mu.Unlock()
			ensureMockRecruitmentLocked(s.mock)
			jsonOut(w, http.StatusOK, map[string]any{"profiles": s.mock.guildRecruitment})
			return
		}
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT guild_id,headline,description,looking_for,schedule,contact,discord_url,active,updated_by,updated_at FROM portal_guild_recruitment WHERE realm_key=? ORDER BY updated_at DESC", s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load recruitment profiles")
			return
		}
		defer rows.Close()
		profiles := []guildRecruitment{}
		for rows.Next() {
			var item guildRecruitment
			if rows.Scan(&item.GuildID, &item.Headline, &item.Description, &item.LookingFor, &item.Schedule, &item.Contact, &item.DiscordURL, &item.Active, &item.UpdatedBy, &item.UpdatedAt) == nil {
				_ = s.s.Characters.QueryRowContext(r.Context(), fmt.Sprintf("SELECT name FROM %s.guild WHERE guildid=?", s.c.CharactersDB), item.GuildID).Scan(&item.GuildName)
				profiles = append(profiles, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"profiles": profiles})
		return
	}
	var profile guildRecruitment
	if !decode(w, r, &profile) {
		return
	}
	if err := validateGuildRecruitment(&profile); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	profile.UpdatedBy = actor.ID
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockRecruitmentLocked(s.mock)
		found := false
		for index := range s.mock.guildRecruitment {
			if s.mock.guildRecruitment[index].GuildID == profile.GuildID {
				profile.GuildName = s.mock.guildRecruitment[index].GuildName
				profile.UpdatedAt = time.Now()
				s.mock.guildRecruitment[index] = profile
				found = true
			}
		}
		if !found {
			for _, guild := range mockGuildData() {
				if fmt.Sprint(guild["id"]) == strconv.FormatUint(uint64(profile.GuildID), 10) {
					profile.GuildName = fmt.Sprint(guild["name"])
				}
			}
			if profile.GuildName == "" {
				problem(w, http.StatusNotFound, "Guild not found")
				return
			}
			profile.UpdatedAt = time.Now()
			s.mock.guildRecruitment = append(s.mock.guildRecruitment, profile)
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var guildName string
	if err := s.s.Characters.QueryRowContext(r.Context(), fmt.Sprintf("SELECT name FROM %s.guild WHERE guildid=?", s.c.CharactersDB), profile.GuildID).Scan(&guildName); err != nil {
		problem(w, http.StatusNotFound, "Guild not found")
		return
	}
	_, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_guild_recruitment(realm_key,guild_id,headline,description,looking_for,schedule,contact,discord_url,active,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE headline=VALUES(headline),description=VALUES(description),looking_for=VALUES(looking_for),schedule=VALUES(schedule),contact=VALUES(contact),discord_url=VALUES(discord_url),active=VALUES(active),updated_by=VALUES(updated_by)`, s.c.RealmKey, profile.GuildID, profile.Headline, profile.Description, profile.LookingFor, profile.Schedule, profile.Contact, profile.DiscordURL, profile.Active, actor.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not save recruitment profile")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'guild.recruitment',?,?)", actor.ID, strconv.FormatUint(uint64(profile.GuildID), 10), "active="+strconv.FormatBool(profile.Active))
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminGuildApplications(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "support"); !ok {
		problem(w, http.StatusForbidden, "Support permission required")
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	page, perPage, offset := requestPage(r, 25, 100)
	if status != "" && !guildApplicationStatuses[status] || len(search) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Invalid status")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		items := []guildApplication{}
		for _, item := range s.mock.guildApplications {
			if (status == "" || item.Status == status) && (search == "" || strings.Contains(strings.ToLower(item.Username+" "+item.CharacterName+" "+item.GuildName), strings.ToLower(search))) {
				items = append(items, item)
			}
		}
		items, meta := slicePage(items, page, perPage)
		jsonOut(w, http.StatusOK, map[string]any{"applications": items, "pagination": meta})
		return
	}
	query := fmt.Sprintf(`SELECT ga.id,ga.guild_id,COALESCE(g.name,''),ga.account_id,COALESCE(a.username,''),ga.character_guid,COALESCE(c.name,''),ga.message,ga.status,ga.response,ga.staff_note,ga.handled_by,ga.created_at,ga.updated_at FROM portal_guild_applications ga LEFT JOIN %s.account a ON a.id=ga.account_id LEFT JOIN %s.characters c ON c.guid=ga.character_guid LEFT JOIN %s.guild g ON g.guildid=ga.guild_id WHERE ga.realm_key=?`, s.c.AuthDB, s.c.CharactersDB, s.c.CharactersDB)
	args := []any{s.c.RealmKey}
	if status != "" {
		query += " AND ga.status=?"
		args = append(args, status)
	}
	if search != "" {
		query += " AND (a.username LIKE ? OR c.name LIKE ? OR g.name LIKE ?)"
		pattern := likePattern(search)
		args = append(args, pattern, pattern, pattern)
	}
	countQuery := "SELECT COUNT(*) FROM (" + query + ") portal_guild_application_count"
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), countQuery, args...).Scan(&total); err != nil {
		problem(w, http.StatusInternalServerError, "Could not count guild applications")
		return
	}
	meta := paginationMeta(page, perPage, total)
	offset = (meta.Page - 1) * perPage
	query += " ORDER BY ga.id DESC LIMIT ? OFFSET ?"
	rows, err := s.s.Auth.QueryContext(r.Context(), query, append(args, perPage, offset)...)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load guild applications")
		return
	}
	defer rows.Close()
	items := []guildApplication{}
	for rows.Next() {
		var item guildApplication
		if rows.Scan(&item.ID, &item.GuildID, &item.GuildName, &item.AccountID, &item.Username, &item.CharacterGUID, &item.CharacterName, &item.Message, &item.Status, &item.Response, &item.StaffNote, &item.HandledBy, &item.CreatedAt, &item.UpdatedAt) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"applications": items, "pagination": meta})
}

func (s *Server) adminGuildApplication(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "support")
	if !ok {
		problem(w, http.StatusForbidden, "Support permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid application")
		return
	}
	var in struct{ Status, Response, StaffNote string }
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.Response = strings.TrimSpace(in.Response)
	in.StaffNote = strings.TrimSpace(in.StaffNote)
	if !guildApplicationStatuses[in.Status] || in.Status == "withdrawn" || len(in.Response) > 2000 || len(in.StaffNote) > 2000 {
		problem(w, http.StatusUnprocessableEntity, "Choose a valid staff status and note")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for index := range s.mock.guildApplications {
			item := &s.mock.guildApplications[index]
			if item.ID == id {
				item.Status = in.Status
				item.Response = in.Response
				item.StaffNote = in.StaffNote
				item.HandledBy = actor.ID
				item.UpdatedAt = time.Now()
				s.mock.notifications = append([]notification{{ID: uint64(len(s.mock.notifications) + 1), Kind: "guild", Title: "Guild application updated", Message: "Your application is now " + in.Status + ".", ActionURL: "/guilds/" + strconv.FormatUint(uint64(item.GuildID), 10), Created: time.Now()}}, s.mock.notifications...)
				jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, http.StatusNotFound, "Application not found")
		return
	}
	var ownerID, guildID uint32
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT account_id,guild_id FROM portal_guild_applications WHERE id=? AND realm_key=?", id, s.c.RealmKey).Scan(&ownerID, &guildID); err != nil {
		problem(w, http.StatusNotFound, "Application not found")
		return
	}
	_, err = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_guild_applications SET status=?,response=?,staff_note=?,handled_by=? WHERE id=? AND realm_key=?", in.Status, in.Response, in.StaffNote, actor.ID, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update application")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'guild.application',?,?)", actor.ID, strconv.FormatUint(id, 10), "status="+in.Status)
	s.notifyAccount(r.Context(), ownerID, "guild", "Guild application updated", "Your application is now "+strings.ReplaceAll(in.Status, "_", " ")+".", "/guilds/"+strconv.FormatUint(uint64(guildID), 10))
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
