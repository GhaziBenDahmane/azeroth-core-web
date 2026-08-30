package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const discordAPIBase = "https://discord.com/api/v10"
const googleOAuthBase = "https://accounts.google.com/o/oauth2/v2/auth"
const googleTokenURL = "https://oauth2.googleapis.com/token"
const googleUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

type externalIdentity struct {
	Provider string `json:"provider"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Avatar   string `json:"avatarUrl,omitempty"`
}

func oauthSecret(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func (s *Server) discordOAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.c.DiscordClientID == "" || s.c.DiscordClientSecret == "" {
		problem(w, http.StatusNotFound, "Discord sign-in is not configured")
		return
	}
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Discord OAuth requires a configured database-backed portal")
		return
	}
	mode := "login"
	identityID := uint64(0)
	if strings.EqualFold(r.URL.Query().Get("mode"), "link") {
		active, err := s.auth(r)
		if err != nil {
			problem(w, http.StatusUnauthorized, "Sign in before linking Discord")
			return
		}
		if !s.stepUpValid(r) {
			problem(w, http.StatusPreconditionRequired, "Confirm your password and authenticator code to continue")
			return
		}
		identityID, err = s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load master account")
			return
		}
		mode = "link"
	}
	state, err := oauthSecret(32)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Discord authentication")
		return
	}
	verifier, err := oauthSecret(48)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Discord authentication")
		return
	}
	stateHash := sha256.Sum256([]byte(state))
	_, _ = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_oauth_states WHERE expires_at<NOW()`)
	if _, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_oauth_states(state_hash,provider,flow_mode,identity_id,redirect_path,code_verifier,expires_at) VALUES(?,'discord',?,?,?, ?,DATE_ADD(NOW(),INTERVAL 10 MINUTE))`, stateHash[:], mode, identityID, oauthRedirectPath(mode), verifier); err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Discord authentication")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_oauth_state", Value: state, Path: "/api/auth/discord/callback", MaxAge: 600, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"client_id":             {s.c.DiscordClientID},
		"redirect_uri":          {s.c.DiscordRedirectURL},
		"response_type":         {"code"},
		"scope":                 {"identify email"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"prompt":                {"consent"},
	}
	http.Redirect(w, r, "https://discord.com/oauth2/authorize?"+values.Encode(), http.StatusFound)
}

func oauthRedirectPath(mode string) string {
	if mode == "link" {
		return "/account/security?provider=discord"
	}
	return "/account?provider=discord"
}

func (s *Server) discordOAuthCallback(w http.ResponseWriter, r *http.Request) {
	failPath := "/login?oauth=failed"
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateCookie, cookieErr := r.Cookie("portal_oauth_state")
	stateBound := cookieErr == nil && len(stateCookie.Value) == len(state) && subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) == 1
	http.SetCookie(w, &http.Cookie{Name: "portal_oauth_state", Path: "/api/auth/discord/callback", MaxAge: -1, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	if state == "" || code == "" || !stateBound || r.URL.Query().Get("error") != "" || s.c.DiscordClientID == "" {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	hash := sha256.Sum256([]byte(state))
	var mode, redirectPath, verifier string
	var identityID uint64
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(r.Context(), `SELECT flow_mode,identity_id,redirect_path,code_verifier FROM portal_oauth_states WHERE state_hash=? AND provider='discord' AND expires_at>NOW() FOR UPDATE`, hash[:]).Scan(&mode, &identityID, &redirectPath, &verifier)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_oauth_states WHERE state_hash=?`, hash[:]); err != nil || tx.Commit() != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	profile, err := s.exchangeDiscordCode(r, code, verifier)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if mode == "link" {
		active, authErr := s.auth(r)
		if authErr != nil || !s.stepUpValid(r) {
			http.Redirect(w, r, "/login?oauth=reauth", http.StatusFound)
			return
		}
		currentIdentity, identityErr := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
		if identityErr != nil || currentIdentity != identityID {
			http.Redirect(w, r, failPath, http.StatusFound)
			return
		}
		if err = s.linkExternalIdentity(r, identityID, profile); err != nil {
			http.Redirect(w, r, "/account/security?provider=conflict", http.StatusFound)
			return
		}
		s.auditIdentity(r, active.ID, "identity.provider.link", identityID, "Discord account linked")
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	}
	if err = s.s.Auth.QueryRowContext(r.Context(), `SELECT identity_id FROM portal_identity_providers WHERE provider='discord' AND provider_user_id=?`, profile.UserID).Scan(&identityID); err != nil {
		http.Redirect(w, r, "/login?oauth=unlinked", http.StatusFound)
		return
	}
	var active account
	query := fmt.Sprintf(`SELECT a.id,a.username,a.email FROM portal_identity_accounts ia JOIN %s.account a ON a.id=ia.account_id WHERE ia.identity_id=? AND ia.is_primary=1 AND a.locked=0 AND NOT EXISTS (SELECT 1 FROM %s.account_banned b WHERE b.id=a.id AND b.active=1) LIMIT 1`, s.c.AuthDB, s.c.AuthDB)
	if err = s.s.Auth.QueryRowContext(r.Context(), query, identityID).Scan(&active.ID, &active.Username, &active.Email); err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if err = s.issuePortalSession(w, r, active, identityID); err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	s.auditIdentity(r, active.ID, "identity.provider.login", identityID, "Signed in with Discord")
	http.Redirect(w, r, redirectPath, http.StatusFound)
}

func (s *Server) exchangeDiscordCode(r *http.Request, code, verifier string) (externalIdentity, error) {
	values := url.Values{"client_id": {s.c.DiscordClientID}, "client_secret": {s.c.DiscordClientSecret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.c.DiscordRedirectURL}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, discordAPIBase+"/oauth2/token", strings.NewReader(values.Encode()))
	if err != nil {
		return externalIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.soap.HTTP.Do(req)
	if err != nil {
		return externalIdentity{}, err
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" {
		return externalIdentity{}, fmt.Errorf("discord token exchange failed")
	}
	req, err = http.NewRequestWithContext(r.Context(), http.MethodGet, discordAPIBase+"/users/@me", nil)
	if err != nil {
		return externalIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err = s.soap.HTTP.Do(req)
	if err != nil {
		return externalIdentity{}, err
	}
	defer resp.Body.Close()
	var user struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
		Email      string `json:"email"`
		Avatar     string `json:"avatar"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user) != nil || user.ID == "" {
		return externalIdentity{}, fmt.Errorf("discord profile request failed")
	}
	name := strings.TrimSpace(user.GlobalName)
	if name == "" {
		name = user.Username
	}
	avatar := ""
	if user.Avatar != "" {
		avatar = "https://cdn.discordapp.com/avatars/" + url.PathEscape(user.ID) + "/" + url.PathEscape(user.Avatar) + ".png"
	}
	return externalIdentity{Provider: "discord", UserID: user.ID, Username: truncate(name, 100), Email: truncate(user.Email, 255), Avatar: avatar}, nil
}

func (s *Server) googleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.c.GoogleClientID == "" || s.c.GoogleClientSecret == "" {
		problem(w, http.StatusNotFound, "Google sign-in is not configured")
		return
	}
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Google OAuth requires a configured database-backed portal")
		return
	}
	mode := "login"
	identityID := uint64(0)
	if strings.EqualFold(r.URL.Query().Get("mode"), "link") {
		active, err := s.auth(r)
		if err != nil {
			problem(w, http.StatusUnauthorized, "Sign in before linking Google")
			return
		}
		if !s.stepUpValid(r) {
			problem(w, http.StatusPreconditionRequired, "Confirm your password and authenticator code to continue")
			return
		}
		identityID, err = s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load master account")
			return
		}
		mode = "link"
	}
	state, err := oauthSecret(32)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Google authentication")
		return
	}
	verifier, err := oauthSecret(48)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Google authentication")
		return
	}
	stateHash := sha256.Sum256([]byte(state))
	_, _ = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_oauth_states WHERE expires_at<NOW()`)
	redirectPath := oauthProviderRedirectPath(mode, "google")
	if _, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_oauth_states(state_hash,provider,flow_mode,identity_id,redirect_path,code_verifier,expires_at) VALUES(?,'google',?,?,?,?,DATE_ADD(NOW(),INTERVAL 10 MINUTE))`, stateHash[:], mode, identityID, redirectPath, verifier); err != nil {
		problem(w, http.StatusInternalServerError, "Could not start Google authentication")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_google_oauth_state", Value: state, Path: "/api/auth/google/callback", MaxAge: 600, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	challenge := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"client_id":             {s.c.GoogleClientID},
		"redirect_uri":          {s.c.GoogleRedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	http.Redirect(w, r, googleOAuthBase+"?"+values.Encode(), http.StatusFound)
}

func oauthProviderRedirectPath(mode, provider string) string {
	if mode == "link" {
		return "/account/security?provider=" + url.QueryEscape(provider)
	}
	return "/account?provider=" + url.QueryEscape(provider)
}

func (s *Server) googleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	failPath := "/login?oauth=failed&provider=google"
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	stateCookie, cookieErr := r.Cookie("portal_google_oauth_state")
	stateBound := cookieErr == nil && len(stateCookie.Value) == len(state) && subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) == 1
	http.SetCookie(w, &http.Cookie{Name: "portal_google_oauth_state", Path: "/api/auth/google/callback", MaxAge: -1, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	if state == "" || code == "" || !stateBound || r.URL.Query().Get("error") != "" || s.c.GoogleClientID == "" {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	hash := sha256.Sum256([]byte(state))
	var mode, redirectPath, verifier string
	var identityID uint64
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(r.Context(), `SELECT flow_mode,identity_id,redirect_path,code_verifier FROM portal_oauth_states WHERE state_hash=? AND provider='google' AND expires_at>NOW() FOR UPDATE`, hash[:]).Scan(&mode, &identityID, &redirectPath, &verifier)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_oauth_states WHERE state_hash=?`, hash[:]); err != nil || tx.Commit() != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	profile, err := s.exchangeGoogleCode(r, code, verifier)
	if err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if mode == "link" {
		active, authErr := s.auth(r)
		if authErr != nil || !s.stepUpValid(r) {
			http.Redirect(w, r, "/login?oauth=reauth&provider=google", http.StatusFound)
			return
		}
		currentIdentity, identityErr := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
		if identityErr != nil || currentIdentity != identityID {
			http.Redirect(w, r, failPath, http.StatusFound)
			return
		}
		if err = s.linkExternalIdentity(r, identityID, profile); err != nil {
			http.Redirect(w, r, "/account/security?provider=conflict", http.StatusFound)
			return
		}
		s.auditIdentity(r, active.ID, "identity.provider.link", identityID, "Google account linked")
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	}
	if err = s.s.Auth.QueryRowContext(r.Context(), `SELECT identity_id FROM portal_identity_providers WHERE provider='google' AND provider_user_id=?`, profile.UserID).Scan(&identityID); err != nil {
		http.Redirect(w, r, "/login?oauth=unlinked&provider=google", http.StatusFound)
		return
	}
	var active account
	query := fmt.Sprintf(`SELECT a.id,a.username,a.email FROM portal_identity_accounts ia JOIN %s.account a ON a.id=ia.account_id WHERE ia.identity_id=? AND ia.is_primary=1 AND a.locked=0 AND NOT EXISTS (SELECT 1 FROM %s.account_banned b WHERE b.id=a.id AND b.active=1) LIMIT 1`, s.c.AuthDB, s.c.AuthDB)
	if err = s.s.Auth.QueryRowContext(r.Context(), query, identityID).Scan(&active.ID, &active.Username, &active.Email); err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	if err = s.issuePortalSession(w, r, active, identityID); err != nil {
		http.Redirect(w, r, failPath, http.StatusFound)
		return
	}
	s.auditIdentity(r, active.ID, "identity.provider.login", identityID, "Signed in with Google")
	http.Redirect(w, r, redirectPath, http.StatusFound)
}

func (s *Server) exchangeGoogleCode(r *http.Request, code, verifier string) (externalIdentity, error) {
	values := url.Values{"client_id": {s.c.GoogleClientID}, "client_secret": {s.c.GoogleClientSecret}, "grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.c.GoogleRedirectURL}, "code_verifier": {verifier}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, googleTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return externalIdentity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.soap.HTTP.Do(req)
	if err != nil {
		return externalIdentity{}, err
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token) != nil || token.AccessToken == "" {
		return externalIdentity{}, fmt.Errorf("google token exchange failed")
	}
	req, err = http.NewRequestWithContext(r.Context(), http.MethodGet, googleUserInfoURL, nil)
	if err != nil {
		return externalIdentity{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	resp, err = s.soap.HTTP.Do(req)
	if err != nil {
		return externalIdentity{}, err
	}
	defer resp.Body.Close()
	var user struct {
		ID            string `json:"sub"`
		Name          string `json:"name"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Picture       string `json:"picture"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&user) != nil || user.ID == "" {
		return externalIdentity{}, fmt.Errorf("google profile request failed")
	}
	email := ""
	if user.EmailVerified {
		email = truncate(strings.TrimSpace(user.Email), 255)
	}
	name := strings.TrimSpace(user.Name)
	if name == "" {
		name = email
	}
	return externalIdentity{Provider: "google", UserID: truncate(user.ID, 128), Username: truncate(name, 100), Email: email, Avatar: truncate(user.Picture, 500)}, nil
}

func (s *Server) linkExternalIdentity(r *http.Request, identityID uint64, profile externalIdentity) error {
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingIdentity uint64
	err = tx.QueryRowContext(r.Context(), `SELECT identity_id FROM portal_identity_providers WHERE provider=? AND provider_user_id=? FOR UPDATE`, profile.Provider, profile.UserID).Scan(&existingIdentity)
	if err == nil && existingIdentity != identityID {
		return fmt.Errorf("provider account already linked")
	}
	var existingUser string
	err = tx.QueryRowContext(r.Context(), `SELECT provider_user_id FROM portal_identity_providers WHERE identity_id=? AND provider=? FOR UPDATE`, identityID, profile.Provider).Scan(&existingUser)
	if err == nil && existingUser != profile.UserID {
		return fmt.Errorf("identity already has provider")
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_identity_providers(identity_id,provider,provider_user_id,username,email,avatar_url) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE username=VALUES(username),email=VALUES(email),avatar_url=VALUES(avatar_url)`, identityID, profile.Provider, profile.UserID, profile.Username, profile.Email, profile.Avatar)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) issuePortalSession(w http.ResponseWriter, r *http.Request, active account, identityID uint64) error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(7 * 24 * time.Hour)
	ua := truncate(r.UserAgent(), 255)
	ip := s.clientIP(r)
	if _, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_sessions(token_hash,account_id,identity_id,expires_at,ip_address,user_agent) VALUES(?,?,?,?,?,?)`, hash[:], active.ID, identityID, expires, ip, ua); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: token, Path: "/", Expires: expires, MaxAge: 604800, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	return nil
}

func (s *Server) identityUnlinkProvider(w http.ResponseWriter, r *http.Request) {
	active, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
	if provider != "discord" && provider != "google" {
		problem(w, http.StatusBadRequest, "Unsupported identity provider")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"unlinked": true})
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_identity_providers WHERE identity_id=? AND provider=?`, identityID, provider)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not unlink identity provider")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusNotFound, "Identity provider is not linked")
		return
	}
	s.auditIdentity(r, active.ID, "identity.provider.unlink", identityID, strings.ToUpper(provider[:1])+provider[1:]+" account unlinked")
	jsonOut(w, http.StatusOK, map[string]bool{"unlinked": true})
}

func (s *Server) loadIdentityProviders(r *http.Request, identityID uint64) []externalIdentity {
	if s.c.MockMode {
		return []externalIdentity{}
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT provider,provider_user_id,username,email,avatar_url FROM portal_identity_providers WHERE identity_id=? ORDER BY provider`, identityID)
	if err != nil {
		return []externalIdentity{}
	}
	defer rows.Close()
	out := []externalIdentity{}
	for rows.Next() {
		var item externalIdentity
		if rows.Scan(&item.Provider, &item.UserID, &item.Username, &item.Email, &item.Avatar) == nil {
			out = append(out, item)
		}
	}
	return out
}
