package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/srp"
)

func (s *Server) publicConfig(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, 200, map[string]any{
		"portalName": s.c.PortalName, "realmName": s.c.RealmName, "brandMark": s.c.BrandMark,
		"tagline": s.c.PortalTagline, "expansionName": s.c.ExpansionName,
		"clientVersion": s.c.ClientVersion, "clientBuild": s.c.ClientBuild,
		"experienceRate": s.c.ExperienceRate, "uptimeLabel": s.c.UptimeLabel,
		"footerText": s.c.FooterText, "realmAddress": s.c.RealmAddress,
		"downloadUrl": s.c.DownloadURL, "communityUrl": s.c.CommunityURL,
		"logoUrl": s.c.LogoURL, "heroImageUrl": s.c.HeroImageURL, "faviconUrl": s.c.FaviconURL,
		"themePrimary": s.c.ThemePrimary, "themeSecondary": s.c.ThemeSecondary,
		"themeAccent": s.c.ThemeAccent, "themeBackground": s.c.ThemeBackground,
		"locale": s.c.Locale, "translations": s.c.UIText, "news": s.c.News,
		"termsUrl": s.c.TermsURL, "privacyUrl": s.c.PrivacyURL,
		"features": map[string]bool{
			"registration": s.c.EnableRegistration, "armory": s.c.EnableArmory,
			"rankings": s.c.EnableRankings, "guilds": s.c.EnableGuilds,
			"realm": s.c.EnableRealmStatus, "shop": s.c.EnableShop,
			"support": s.c.EnableSupport, "admin": s.c.EnableAdminPanel,
			"gmConsole": s.c.EnableAdminPanel && s.c.EnableGMConsole,
		},
		"turnstileSiteKey":     s.c.TurnstileSiteKey,
		"passwordResetEnabled": s.c.MockMode || (s.c.SMTPAddr != "" && s.c.SMTPFrom != ""),
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
	return smtp.SendMail(s.c.SMTPAddr, auth, s.c.SMTPFrom, []string{to}, msg)
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
	if err != nil || tx.Commit() != nil {
		problem(w, 500, "Could not reset password")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
