package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/srp"
)

func validTOTP(secret, code string, now time.Time) bool {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(code) != 6 {
		return false
	}
	for offset := -1; offset <= 1; offset++ {
		counter := uint64(now.Unix()/30 + int64(offset))
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, counter)
		mac := hmac.New(sha1.New, raw)
		_, _ = mac.Write(buf)
		sum := mac.Sum(nil)
		i := sum[len(sum)-1] & 15
		value := (uint32(sum[i])&127)<<24 | uint32(sum[i+1])<<16 | uint32(sum[i+2])<<8 | uint32(sum[i+3])
		if fmt.Sprintf("%06d", value%1000000) == code {
			return true
		}
	}
	return false
}

func (s *Server) securityTOTPSetup(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var enabled bool
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT totp_enabled FROM portal_account_security WHERE account_id=?", a.ID).Scan(&enabled)
	if enabled {
		problem(w, http.StatusConflict, "Disable the current authenticator before enrolling a new one")
		return
	}
	raw := make([]byte, 20)
	if _, e = rand.Read(raw); e != nil {
		problem(w, 500, "Could not generate secret")
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	_, e = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_account_security(account_id,totp_secret,totp_enabled) VALUES(?,?,0) ON DUPLICATE KEY UPDATE totp_secret=VALUES(totp_secret),totp_enabled=0", a.ID, secret)
	if e != nil {
		problem(w, 500, "Could not save authenticator setup")
		return
	}
	uri := "otpauth://totp/" + url.PathEscape(s.c.RealmName+":"+a.Username) + "?secret=" + url.QueryEscape(secret) + "&issuer=" + url.QueryEscape(s.c.RealmName) + "&digits=6&period=30"
	jsonOut(w, 200, map[string]string{"secret": secret, "uri": uri})
}
func (s *Server) securityTOTPEnable(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	var secret string
	if s.s.Auth.QueryRowContext(r.Context(), "SELECT totp_secret FROM portal_account_security WHERE account_id=?", a.ID).Scan(&secret) != nil || !validTOTP(secret, in.Code, time.Now()) {
		problem(w, 422, "Invalid authenticator code")
		return
	}
	_, e = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_account_security SET totp_enabled=1 WHERE account_id=?", a.ID)
	if e != nil {
		problem(w, 500, "Could not enable authenticator")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) securityTOTPDisable(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	var secret string
	if s.s.Auth.QueryRowContext(r.Context(), "SELECT totp_secret FROM portal_account_security WHERE account_id=? AND totp_enabled=1", a.ID).Scan(&secret) != nil || !validTOTP(secret, in.Code, time.Now()) {
		problem(w, 422, "Invalid authenticator code")
		return
	}
	_, e = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_account_security SET totp_secret=NULL,totp_enabled=0 WHERE account_id=?", a.ID)
	if e != nil {
		problem(w, 500, "Could not disable authenticator")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) securityPassword(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !decode(w, r, &in) {
		return
	}
	if e = srp.Validate(a.Username, in.NewPassword); e != nil {
		problem(w, 422, e.Error())
		return
	}
	var salt, verifier []byte
	q := fmt.Sprintf("SELECT salt,verifier FROM `%s`.account WHERE id=?", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), q, a.ID).Scan(&salt, &verifier) != nil || !srp.Verify(a.Username, in.CurrentPassword, salt, verifier) {
		problem(w, 401, "Current password is incorrect")
		return
	}
	salt, verifier, e = srp.Registration(a.Username, in.NewPassword)
	if e != nil {
		problem(w, 500, "Could not secure password")
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	q = fmt.Sprintf("UPDATE `%s`.account SET salt=?,verifier=? WHERE id=?", s.c.AuthDB)
	if _, e = tx.ExecContext(r.Context(), q, salt, verifier, a.ID); e == nil {
		_, e = tx.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE account_id=?", a.ID)
	}
	if e != nil || tx.Commit() != nil {
		problem(w, 500, "Could not change password")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func currentSessionHash(r *http.Request) ([32]byte, error) {
	c, e := r.Cookie("portal_session")
	if e != nil {
		return [32]byte{}, e
	}
	return sha256.Sum256([]byte(c.Value)), nil
}
func (s *Server) securitySessions(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	current, _ := currentSessionHash(r)
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT token_hash,created_at,last_seen_at,expires_at,ip_address,user_agent FROM portal_sessions WHERE account_id=? ORDER BY created_at DESC", a.ID)
	if e != nil {
		problem(w, 500, "Could not load sessions")
		return
	}
	defer rows.Close()
	type session struct {
		ID                         string `json:"id"`
		Created, LastSeen, Expires time.Time
		IP, UserAgent              string
		Current                    bool
	}
	out := []session{}
	for rows.Next() {
		var x session
		var hash []byte
		if rows.Scan(&hash, &x.Created, &x.LastSeen, &x.Expires, &x.IP, &x.UserAgent) == nil {
			x.ID = hex.EncodeToString(hash)
			x.Current = hmac.Equal(hash, current[:])
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"sessions": out})
}
func (s *Server) securityRevokeSession(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	hash, e := hex.DecodeString(r.PathValue("id"))
	if e != nil || len(hash) != 32 {
		problem(w, 400, "Invalid session")
		return
	}
	res, e := s.s.Auth.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE token_hash=? AND account_id=?", hash, a.ID)
	if e != nil {
		problem(w, 500, "Could not revoke session")
		return
	}
	n, _ := res.RowsAffected()
	jsonOut(w, 200, map[string]any{"ok": n == 1})
}
