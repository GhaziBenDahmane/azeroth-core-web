package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/srp"
)

func (s *Server) publicConfig(w http.ResponseWriter, r *http.Request) {
	cfg := s.runtimeSettings(r)
	navigationConfigured, navigation := s.loadNavigation(r, false)
	type publicRealm struct {
		Key            string `json:"key"`
		Name           string `json:"name"`
		Address        string `json:"address"`
		ExperienceRate string `json:"experienceRate"`
	}
	realms := make([]publicRealm, 0, len(s.c.Realms))
	for _, realm := range s.c.Realms {
		if realm.Key == s.c.RealmKey {
			realm.Name, realm.Address, realm.ExperienceRate = cfg.RealmName, cfg.RealmAddress, cfg.ExperienceRate
		}
		realms = append(realms, publicRealm{realm.Key, realm.Name, realm.Address, realm.ExperienceRate})
	}
	news := s.publicNews(r)
	if len(news) == 0 {
		for i, item := range s.c.News {
			news = append(news, newsEntry{ID: uint64(i + 1), Title: item.Title, Summary: item.Summary, URL: item.URL, Kind: "news", Active: true})
		}
	}
	for i := range news {
		news[i].Featured = cfg.FeaturedNewsID > 0 && news[i].ID == cfg.FeaturedNewsID
	}
	now := time.Now()
	maintenance := cfg.MaintenanceEnabled && (cfg.MaintenanceStarts == nil || !now.Before(*cfg.MaintenanceStarts)) && (cfg.MaintenanceEnds == nil || now.Before(*cfg.MaintenanceEnds))
	jsonOut(w, 200, map[string]any{
		"portalName": cfg.PortalName, "realmName": cfg.RealmName, "brandMark": cfg.BrandMark,
		"realmKey": s.c.RealmKey, "realms": realms,
		"tagline": cfg.Tagline, "homeHeadline": cfg.HomeHeadline, "homeEyebrow": cfg.HomeEyebrow,
		"homePrimaryCta": cfg.HomePrimaryCTA, "homeConnectTitle": cfg.HomeConnectTitle, "expansionName": s.c.ExpansionName,
		"homeGuideText": cfg.HomeGuideText, "homeRules": cfg.HomeRules, "discordStatus": cfg.DiscordStatus, "homeChangelog": cfg.HomeChangelog,
		"homeFeatures": cfg.HomeFeatures, "homeProgression": cfg.HomeProgression,
		"clientVersion": s.c.ClientVersion, "clientBuild": s.c.ClientBuild,
		"experienceRate": cfg.ExperienceRate, "uptimeLabel": s.c.UptimeLabel,
		"realmProfile": map[string]any{"type": cfg.RealmType, "timezone": cfg.RealmTimezone, "description": cfg.RealmDescription, "season": cfg.SeasonName, "arenaRewardPolicy": cfg.ArenaRewardPolicy, "startLevel": cfg.StartLevel, "maxLevel": cfg.MaxLevel, "populationCap": cfg.PopulationCap, "transferSlaHours": cfg.TransferSLAHours, "factionPolicy": cfg.FactionPolicy, "crossFaction": settingBool(cfg.CrossFactionGroups, settingBool(cfg.CrossFaction, false)),
			"crossFactionFeatures": map[string]bool{"accounts": settingBool(cfg.CrossFactionAccounts, false), "calendar": settingBool(cfg.CrossFactionCalendar, false), "channels": settingBool(cfg.CrossFactionChannels, false), "groups": settingBool(cfg.CrossFactionGroups, false), "guilds": settingBool(cfg.CrossFactionGuilds, false), "auctions": settingBool(cfg.CrossFactionAuctions, false), "mail": settingBool(cfg.CrossFactionMail, false), "who": settingBool(cfg.CrossFactionWho, false), "friends": settingBool(cfg.CrossFactionFriends, false), "trade": settingBool(cfg.CrossFactionTrade, false)},
			"rates":                map[string]string{"questXp": cfg.QuestExperienceRate, "killXp": cfg.KillExperienceRate, "explorationXp": cfg.ExplorationExperienceRate, "drop": cfg.DropRate, "reputation": cfg.ReputationRate, "honor": cfg.HonorRate, "profession": cfg.ProfessionRate}},
		"footerText": s.c.FooterText, "realmAddress": cfg.RealmAddress,
		"downloadUrl": cfg.DownloadURL, "communityUrl": cfg.CommunityURL,
		"logoUrl": cfg.LogoURL, "heroImageUrl": cfg.HeroImageURL, "faviconUrl": cfg.FaviconURL,
		"themePrimary": cfg.ThemePrimary, "themeSecondary": cfg.ThemeSecondary,
		"themeAccent": cfg.ThemeAccent, "themeBackground": cfg.ThemeBackground,
		"locale": s.c.Locale, "translations": s.c.UIText, "news": news,
		"termsUrl": cfg.TermsURL, "privacyUrl": cfg.PrivacyURL, "securityContactUrl": s.c.SecurityContactURL,
		"analytics":   map[string]string{"scriptUrl": s.c.AnalyticsScriptURL, "domain": s.c.AnalyticsDomain},
		"navigation":  map[string]any{"configured": navigationConfigured, "items": navigation},
		"maintenance": map[string]any{"active": maintenance, "enabled": cfg.MaintenanceEnabled, "message": cfg.MaintenanceMessage, "starts": cfg.MaintenanceStarts, "ends": cfg.MaintenanceEnds},
		"features": map[string]bool{
			"registration": s.c.EnableRegistration && settingBool(cfg.Registration, true), "armory": s.c.EnableArmory && settingBool(cfg.Armory, true),
			"rankings": s.c.EnableRankings && settingBool(cfg.Rankings, true), "guilds": s.c.EnableGuilds && settingBool(cfg.Guilds, true),
			"realm": s.c.EnableRealmStatus && settingBool(cfg.Realm, true), "shop": s.c.EnableShop && settingBool(cfg.Shop, true),
			"support": s.c.EnableSupport && settingBool(cfg.Support, true), "admin": s.c.EnableAdminPanel && settingBool(cfg.Admin, true),
			"gmConsole": s.c.EnableAdminPanel && s.c.EnableGMConsole && settingBool(cfg.Admin, true) && settingBool(cfg.GMConsole, true),
			"billing":   s.c.MockMode || (s.c.StripeSecret != "" && s.c.StripeWebhookSecret != "" && len(s.availableCreditPackages(r, false)) > 0),
		},
		"capabilities": map[string]bool{
			"arenaSeasonArchives":   true,
			"specializationFilters": s.c.MockMode,
			"pvpMatchHistory":       s.c.MockMode || s.c.CompetitiveIngestSecret != "",
			"raidEventIngestion":    s.c.MockMode || s.c.CompetitiveIngestSecret != "",
			"discordOAuth":          s.c.DiscordClientID != "" && s.c.DiscordClientSecret != "",
			"googleOAuth":           s.c.GoogleClientID != "" && s.c.GoogleClientSecret != "",
			"passkeys":              !s.c.MockMode,
		},
		"turnstileSiteKey":          s.c.TurnstileSiteKey,
		"passwordResetEnabled":      s.c.MockMode || (s.c.SMTPAddr != "" && s.c.SMTPFrom != ""),
		"emailVerificationRequired": s.c.RequireEmailVerification,
	})
}

func (s *Server) verifyTurnstile(ctx context.Context, token, remoteIP string) bool {
	if s.c.TurnstileSecret == "" {
		return true
	}
	form := url.Values{"secret": {s.c.TurnstileSecret}, "response": {token}, "remoteip": {remoteIP}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.soap.HTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var result struct {
		Success bool `json:"success"`
	}
	return resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&result) == nil && result.Success
}

func (s *Server) passwordResetRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	generic := map[string]string{"message": "If that address belongs to an account, a reset link has been sent."}
	if s.c.SMTPAddr == "" || s.c.SMTPFrom == "" {
		problem(w, 503, "Password recovery is not configured")
		return
	}
	var id uint32
	var username, email string
	q := fmt.Sprintf("SELECT id,username,email FROM `%s`.account WHERE email=? AND locked=0 LIMIT 1", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), q, strings.ToUpper(strings.TrimSpace(in.Email))).Scan(&id, &username, &email) != nil {
		jsonOut(w, 200, generic)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		problem(w, 500, "Could not create reset request")
		return
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(r.Context(), "DELETE FROM portal_password_resets WHERE account_id=? OR expires_at<NOW()", id)
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_password_resets(token_hash,account_id,expires_at) VALUES(?,?,?)", hash[:], id, time.Now().Add(time.Hour)); err != nil {
		problem(w, 500, "Could not create reset request")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not create reset request")
		return
	}
	link := s.c.PublicURL + "/reset-password?token=" + url.QueryEscape(token)
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(s.c.PortalName) + " portal password reset"
	body := "A password reset was requested for " + username + ".\r\n\r\n" + link + "\r\n\r\nThis link expires in one hour. If you did not request this, ignore this email."
	go func() {
		if mailErr := s.sendMail(email, subject, body); mailErr != nil {
			slog.Error("send password reset", "error", mailErr)
		}
	}()
	jsonOut(w, 200, generic)
}

func (s *Server) adminRequirePasswordReset(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "admin")
	if !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	if s.c.SMTPAddr == "" || s.c.SMTPFrom == "" {
		problem(w, http.StatusServiceUnavailable, "Configure SMTP before requiring a password reset")
		return
	}
	accountID, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || accountID == 0 {
		problem(w, http.StatusBadRequest, "Invalid account")
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) < 3 || len(in.Reason) > 255 || strings.ContainsAny(in.Reason, "\r\n") {
		problem(w, http.StatusUnprocessableEntity, "Reason must be 3–255 characters without line breaks")
		return
	}
	var username, email string
	query := fmt.Sprintf("SELECT username,email FROM `%s`.account WHERE id=?", s.c.AuthDB)
	if err = s.s.Auth.QueryRowContext(r.Context(), query, accountID).Scan(&username, &email); err != nil {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		problem(w, http.StatusInternalServerError, "Could not create reset request")
		return
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(r.Context(), "DELETE FROM portal_password_resets WHERE account_id=? OR expires_at<NOW()", accountID)
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_password_resets(token_hash,account_id,expires_at) VALUES(?,?,?)", hash[:], accountID, time.Now().Add(time.Hour)); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_forced_password_resets(account_id,actor_account_id,reason) VALUES(?,?,?) ON DUPLICATE KEY UPDATE actor_account_id=VALUES(actor_account_id),reason=VALUES(reason),created_at=CURRENT_TIMESTAMP`, accountID, actor.ID, in.Reason)
	}
	var revoked int64
	if err == nil {
		var result sql.Result
		result, err = tx.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE account_id=?", accountID)
		if err == nil {
			revoked, _ = result.RowsAffected()
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'account.password-reset.require',?,?)", actor.ID, username, in.Reason)
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not require password reset")
		return
	}
	link := s.c.PublicURL + "/reset-password?token=" + url.QueryEscape(token)
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(s.c.PortalName) + " required password reset"
	body := "Realm staff required a password reset for " + username + ".\r\nReason: " + in.Reason + "\r\n\r\n" + link + "\r\n\r\nThis link expires in one hour. Your portal sessions have been revoked."
	if err = s.sendMail(email, subject, body); err != nil {
		slog.Error("send staff-required password reset", "account", username, "error", err)
		problem(w, http.StatusBadGateway, "Could not deliver the password reset email; the account was not changed")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not require password reset")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "accountId": accountID, "username": username, "revoked": revoked, "requestId": RequestID(r.Context())})
}

func (s *Server) sendMail(to, subject, body string) error {
	if strings.ContainsAny(s.c.SMTPFrom+to, "\r\n") {
		return fmt.Errorf("invalid mail address")
	}
	host, _, err := net.SplitHostPort(s.c.SMTPAddr)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if s.c.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.c.SMTPUser, s.c.SMTPPassword, host)
	}
	msg := []byte("From: " + s.c.SMTPFrom + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body)
	err = smtp.SendMail(s.c.SMTPAddr, auth, s.c.SMTPFrom, []string{to}, msg)
	if err != nil {
		s.metrics.emailFailures.Add(1)
	}
	return err
}

func (s *Server) passwordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	raw, err := hex.DecodeString(in.Token)
	if err != nil || len(raw) != 32 {
		problem(w, 422, "Invalid or expired reset link")
		return
	}
	hash := sha256.Sum256([]byte(in.Token))
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var id uint32
	var username string
	q := fmt.Sprintf("SELECT r.account_id,a.username FROM portal_password_resets r JOIN `%s`.account a ON a.id=r.account_id WHERE r.token_hash=? AND r.expires_at>NOW() FOR UPDATE", s.c.AuthDB)
	if tx.QueryRowContext(r.Context(), q, hash[:]).Scan(&id, &username) != nil {
		problem(w, 422, "Invalid or expired reset link")
		return
	}
	if err = srp.Validate(username, in.Password); err != nil {
		problem(w, 422, err.Error())
		return
	}
	salt, verifier, err := srp.Registration(username, in.Password)
	if err != nil {
		problem(w, 500, "Could not secure password")
		return
	}
	q = fmt.Sprintf("UPDATE `%s`.account SET salt=?,verifier=? WHERE id=?", s.c.AuthDB)
	if _, err = tx.ExecContext(r.Context(), q, salt, verifier, id); err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE account_id=?", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM portal_password_resets WHERE account_id=?", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM portal_forced_password_resets WHERE account_id=?", id)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, 500, "Could not reset password")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
