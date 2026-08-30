package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

const emailVerificationLifetime = 24 * time.Hour

func validEmailAddress(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && strings.Contains(value, "@") && len(value) <= 255
}

func createEmailVerification(ctx context.Context, tx *sql.Tx, accountID uint32) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	_, err := tx.ExecContext(ctx, `INSERT INTO portal_email_verifications(token_hash,account_id,expires_at)
		VALUES(?,?,?) ON DUPLICATE KEY UPDATE token_hash=VALUES(token_hash),expires_at=VALUES(expires_at),created_at=CURRENT_TIMESTAMP`, hash[:], accountID, time.Now().Add(emailVerificationLifetime))
	return token, err
}

func (s *Server) sendVerificationEmail(email, username, token string) error {
	link := s.c.PublicURL + "/verify-email?token=" + url.QueryEscape(token)
	subject := strings.NewReplacer("\r", " ", "\n", " ").Replace(s.c.PortalName) + " account verification"
	body := "Welcome, " + username + ".\r\n\r\nActivate your game account using this link:\r\n\r\n" + link + "\r\n\r\nThis link expires in 24 hours. If you did not create this account, ignore this email."
	return s.sendMail(email, subject, body)
}

func (s *Server) emailVerificationConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	raw, err := hex.DecodeString(in.Token)
	if err != nil || len(raw) != 32 {
		problem(w, 422, "Invalid or expired verification link")
		return
	}
	hash := sha256.Sum256([]byte(in.Token))
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var accountID uint32
	var pendingEmail string
	if err = tx.QueryRowContext(r.Context(), "SELECT account_id,pending_email FROM portal_email_verifications WHERE token_hash=? AND expires_at>NOW() FOR UPDATE", hash[:]).Scan(&accountID, &pendingEmail); err != nil {
		problem(w, 422, "Invalid or expired verification link")
		return
	}
	var q string
	if pendingEmail != "" {
		q = fmt.Sprintf("UPDATE `%s`.account SET email=?,reg_mail=? WHERE id=?", s.c.AuthDB)
		_, err = tx.ExecContext(r.Context(), q, pendingEmail, pendingEmail, accountID)
	} else {
		q = fmt.Sprintf("UPDATE `%s`.account SET locked=0 WHERE id=?", s.c.AuthDB)
		_, err = tx.ExecContext(r.Context(), q, accountID)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM portal_email_verifications WHERE account_id=?", accountID)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, 500, "Could not verify account")
		return
	}
	message := "Email verified. You can now sign in."
	if pendingEmail != "" {
		message = "Your email address has been updated."
	}
	jsonOut(w, 200, map[string]any{"ok": true, "message": message})
}

func (s *Server) emailVerificationResend(w http.ResponseWriter, r *http.Request) {
	generic := map[string]string{"message": "If that address is awaiting verification, a new link has been sent."}
	if !s.c.RequireEmailVerification {
		jsonOut(w, 200, generic)
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if !decode(w, r, &in) {
		return
	}
	email := strings.ToUpper(strings.TrimSpace(in.Email))
	var accountID uint32
	var username, storedEmail string
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		jsonOut(w, 200, generic)
		return
	}
	defer tx.Rollback()
	q := fmt.Sprintf("SELECT a.id,a.username,a.email FROM `%s`.account a JOIN portal_email_verifications v ON v.account_id=a.id WHERE a.email=? AND a.locked=1 LIMIT 1", s.c.AuthDB)
	q += " FOR UPDATE"
	if tx.QueryRowContext(r.Context(), q, email).Scan(&accountID, &username, &storedEmail) != nil {
		jsonOut(w, 200, generic)
		return
	}
	token, err := createEmailVerification(r.Context(), tx, accountID)
	if err != nil || tx.Commit() != nil {
		jsonOut(w, 200, generic)
		return
	}
	go func() {
		if err := s.sendVerificationEmail(storedEmail, username, token); err != nil {
			slog.Error("resend email verification", "error", err)
		}
	}()
	jsonOut(w, 200, generic)
}
