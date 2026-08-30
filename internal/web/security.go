package web

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/srp"
)

const totpAAD = "azeroth-portal:totp:v1"

func (s *Server) encryptTOTP(secret string) ([]byte, error) {
	if len(s.c.TOTPEncryptionKey) != 32 {
		return nil, fmt.Errorf("TOTP encryption is not configured")
	}
	block, err := aes.NewCipher(s.c.TOTPEncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(secret), []byte(totpAAD))
	return []byte("v1:" + base64.RawURLEncoding.EncodeToString(sealed)), nil
}

func (s *Server) decryptTOTP(stored []byte) (string, error) {
	value := string(stored)
	if !strings.HasPrefix(value, "v1:") { // Legacy value; encrypted after the next successful login.
		return value, nil
	}
	if len(s.c.TOTPEncryptionKey) != 32 {
		return "", fmt.Errorf("TOTP encryption key is unavailable")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.c.TOTPEncryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(payload) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted TOTP secret")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(totpAAD))
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func recoveryCodeHash(code string) [32]byte {
	return sha256.Sum256([]byte(normalizeRecoveryCode(code)))
}

func generateRecoveryCodes(count int) ([]string, [][32]byte, error) {
	codes := make([]string, 0, count)
	hashes := make([][32]byte, 0, count)
	for range count {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		value := strings.ToUpper(hex.EncodeToString(raw))
		code := value[:4] + "-" + value[4:8] + "-" + value[8:12] + "-" + value[12:]
		codes = append(codes, code)
		hashes = append(hashes, recoveryCodeHash(code))
	}
	return codes, hashes, nil
}

func (s *Server) consumeRecoveryCode(ctx context.Context, accountID uint32, code string) bool {
	if len(normalizeRecoveryCode(code)) != 16 || s.s == nil {
		return false
	}
	hash := recoveryCodeHash(code)
	result, err := s.s.Auth.ExecContext(ctx, "DELETE FROM portal_totp_recovery_codes WHERE account_id=? AND code_hash=?", accountID, hash[:])
	if err != nil {
		return false
	}
	rows, _ := result.RowsAffected()
	return rows == 1
}

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
	if len(s.c.TOTPEncryptionKey) != 32 {
		problem(w, http.StatusServiceUnavailable, "Authenticator enrollment requires TOTP_ENCRYPTION_KEY")
		return
	}
	raw := make([]byte, 20)
	if _, e = rand.Read(raw); e != nil {
		problem(w, 500, "Could not generate secret")
		return
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	encrypted, e := s.encryptTOTP(secret)
	if e == nil {
		_, e = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_account_security(account_id,totp_secret,totp_enabled) VALUES(?,?,0) ON DUPLICATE KEY UPDATE totp_secret=VALUES(totp_secret),totp_enabled=0", a.ID, encrypted)
	}
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
	var stored []byte
	if s.s.Auth.QueryRowContext(r.Context(), "SELECT totp_secret FROM portal_account_security WHERE account_id=?", a.ID).Scan(&stored) != nil {
		problem(w, 422, "Invalid authenticator code")
		return
	}
	secret, err := s.decryptTOTP(stored)
	if err != nil || !validTOTP(secret, in.Code, time.Now()) {
		problem(w, 422, "Invalid authenticator code")
		return
	}
	codes, hashes, e := generateRecoveryCodes(10)
	if e != nil {
		problem(w, 500, "Could not generate recovery codes")
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), "UPDATE portal_account_security SET totp_enabled=1 WHERE account_id=?", a.ID)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), "DELETE FROM portal_totp_recovery_codes WHERE account_id=?", a.ID)
	}
	for _, hash := range hashes {
		if e == nil {
			_, e = tx.ExecContext(r.Context(), "INSERT INTO portal_totp_recovery_codes(account_id,code_hash) VALUES(?,?)", a.ID, hash[:])
		}
	}
	if e != nil || tx.Commit() != nil {
		problem(w, 500, "Could not enable authenticator")
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "recoveryCodes": codes})
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
	var stored []byte
	if s.s.Auth.QueryRowContext(r.Context(), "SELECT totp_secret FROM portal_account_security WHERE account_id=? AND totp_enabled=1", a.ID).Scan(&stored) != nil {
		problem(w, 422, "Invalid authenticator or recovery code")
		return
	}
	secret, decryptErr := s.decryptTOTP(stored)
	totpValid := decryptErr == nil && validTOTP(secret, in.Code, time.Now())
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	if !totpValid {
		normalized := normalizeRecoveryCode(in.Code)
		if len(normalized) != 16 {
			problem(w, 422, "Invalid authenticator or recovery code")
			return
		}
		hash := recoveryCodeHash(normalized)
		result, deleteErr := tx.ExecContext(r.Context(), "DELETE FROM portal_totp_recovery_codes WHERE account_id=? AND code_hash=?", a.ID, hash[:])
		if deleteErr != nil {
			problem(w, 422, "Invalid authenticator or recovery code")
			return
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			problem(w, 422, "Invalid authenticator or recovery code")
			return
		}
	}
	_, e = tx.ExecContext(r.Context(), "UPDATE portal_account_security SET totp_secret=NULL,totp_enabled=0 WHERE account_id=?", a.ID)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), "DELETE FROM portal_totp_recovery_codes WHERE account_id=?", a.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		problem(w, 500, "Could not disable authenticator")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) securityTOTPStatus(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var enabled bool
	var remaining int
	err = s.s.Auth.QueryRowContext(r.Context(), `SELECT COALESCE(sec.totp_enabled,0),(SELECT COUNT(*) FROM portal_totp_recovery_codes rc WHERE rc.account_id=?) FROM portal_account_security sec WHERE sec.account_id=?`, a.ID, a.ID).Scan(&enabled, &remaining)
	if err != nil {
		enabled, remaining = false, 0
	}
	jsonOut(w, http.StatusOK, map[string]any{"enabled": enabled, "recoveryCodesRemaining": remaining, "enrollmentAvailable": len(s.c.TOTPEncryptionKey) == 32})
}

func (s *Server) securityStepUp(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	var salt, verifier, stored []byte
	var enabled bool
	q := fmt.Sprintf("SELECT a.salt,a.verifier,COALESCE(sec.totp_secret,''),COALESCE(sec.totp_enabled,0) FROM `%s`.account a LEFT JOIN portal_account_security sec ON sec.account_id=a.id WHERE a.id=?", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), q, a.ID).Scan(&salt, &verifier, &stored, &enabled) != nil || !srp.Verify(a.Username, in.Password, salt, verifier) {
		problem(w, http.StatusUnauthorized, "Password is incorrect")
		return
	}
	if enabled {
		secret, decryptErr := s.decryptTOTP(stored)
		if (decryptErr != nil || !validTOTP(secret, in.Code, time.Now())) && !s.consumeRecoveryCode(r.Context(), a.ID, in.Code) {
			problem(w, http.StatusUnauthorized, "Authenticator or recovery code is incorrect")
			return
		}
	}
	hash, err := currentSessionHash(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_sessions SET step_up_until=DATE_ADD(NOW(),INTERVAL 10 MINUTE) WHERE token_hash=? AND account_id=?", hash[:], a.ID)
	rows := int64(0)
	if err == nil {
		rows, _ = result.RowsAffected()
	}
	if err != nil || rows != 1 {
		problem(w, http.StatusInternalServerError, "Could not confirm this session")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "expiresIn": 600})
}

func (s *Server) stepUpValid(r *http.Request) bool {
	if s.c.MockMode {
		s.mock.mu.Lock()
		valid := !s.mock.stepUpUntil.Before(time.Now())
		s.mock.mu.Unlock()
		return valid
	}
	hash, err := currentSessionHash(r)
	if err != nil {
		return false
	}
	var valid bool
	err = s.s.Auth.QueryRowContext(r.Context(), "SELECT step_up_until IS NOT NULL AND step_up_until>NOW() FROM portal_sessions WHERE token_hash=?", hash[:]).Scan(&valid)
	return err == nil && valid
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

func (s *Server) securityEmail(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.SMTPAddr == "" || s.c.SMTPFrom == "" {
		problem(w, http.StatusServiceUnavailable, "Email delivery is not configured")
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		Email           string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	newEmail := strings.ToUpper(strings.TrimSpace(in.Email))
	if !validEmailAddress(newEmail) || strings.EqualFold(newEmail, a.Email) {
		problem(w, http.StatusUnprocessableEntity, "Enter a different valid email address")
		return
	}
	var salt, verifier []byte
	q := fmt.Sprintf("SELECT salt,verifier FROM `%s`.account WHERE id=?", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), q, a.ID).Scan(&salt, &verifier) != nil || !srp.Verify(a.Username, in.CurrentPassword, salt, verifier) {
		problem(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	token, err := createEmailVerification(r.Context(), tx, a.ID)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE portal_email_verifications SET pending_email=? WHERE account_id=?", newEmail, a.ID)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not request email change")
		return
	}
	go func() {
		if mailErr := s.sendVerificationEmail(newEmail, a.Username, token); mailErr != nil {
			slog.Error("send email change verification", "error", mailErr)
		}
	}()
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "message": "Check the new address to confirm the change."})
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
