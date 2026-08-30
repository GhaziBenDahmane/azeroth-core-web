package web

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/srp"
)

type linkedGameAccount struct {
	ID       uint32 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Label    string `json:"label"`
	Primary  bool   `json:"primary"`
	Active   bool   `json:"active"`
}

func (s *Server) ensureIdentity(ctx context.Context, accountID uint32, username, email string) (uint64, error) {
	// Accounts may predate the portal or be created by bootstrap-account. Make
	// their portal wallet available on first sign-in just as it is for accounts
	// created through the registration endpoint.
	if _, err := s.s.Auth.ExecContext(ctx, `INSERT IGNORE INTO portal_wallets(account_id,balance) VALUES(?,0)`, accountID); err != nil {
		return 0, err
	}
	var identityID uint64
	err := s.s.Auth.QueryRowContext(ctx, `SELECT identity_id FROM portal_identity_accounts WHERE account_id=?`, accountID).Scan(&identityID)
	if err == nil {
		return identityID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	tx, err := s.s.Auth.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT identity_id FROM portal_identity_accounts WHERE account_id=? FOR UPDATE`, accountID).Scan(&identityID); err == nil {
		return identityID, tx.Commit()
	} else if err != sql.ErrNoRows {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO portal_identities(email,display_name) VALUES(?,?)`, email, username)
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()
	identityID = uint64(id)
	if _, err = tx.ExecContext(ctx, `INSERT INTO portal_identity_accounts(identity_id,account_id,label,is_primary) VALUES(?,?,?,1)`, identityID, accountID, username); err != nil {
		return 0, err
	}
	return identityID, tx.Commit()
}

func (s *Server) identityAccounts(w http.ResponseWriter, r *http.Request) {
	active, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"identity": map[string]any{"displayName": "DEMO"}, "accounts": []linkedGameAccount{{ID: 1, Username: "DEMO", Email: "demo@example.com", Label: "Main account", Primary: true, Active: true}}, "providers": []externalIdentity{}})
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	s.bindSessionIdentity(r, identityID)
	query := fmt.Sprintf(`SELECT a.id,a.username,a.email,ia.label,ia.is_primary FROM portal_identity_accounts ia JOIN %s.account a ON a.id=ia.account_id WHERE ia.identity_id=? ORDER BY ia.is_primary DESC,a.username`, s.c.AuthDB)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, identityID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load linked game accounts")
		return
	}
	defer rows.Close()
	accounts := []linkedGameAccount{}
	for rows.Next() {
		var item linkedGameAccount
		if rows.Scan(&item.ID, &item.Username, &item.Email, &item.Label, &item.Primary) == nil {
			item.Active = item.ID == active.ID
			accounts = append(accounts, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"identity": map[string]any{"id": identityID, "displayName": active.Username}, "accounts": accounts, "providers": s.loadIdentityProviders(r, identityID)})
}

func (s *Server) identityLinkAccount(w http.ResponseWriter, r *http.Request) {
	active, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]bool{"linked": true})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Label    string `json:"label"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Username, input.Label = strings.ToUpper(strings.TrimSpace(input.Username)), strings.TrimSpace(input.Label)
	var target account
	var salt, verifier []byte
	var storedTOTP []byte
	var totpEnabled bool
	query := fmt.Sprintf(`SELECT a.id,a.username,a.email,a.salt,a.verifier,COALESCE(sec.totp_secret,''),COALESCE(sec.totp_enabled,0) FROM %s.account a LEFT JOIN portal_account_security sec ON sec.account_id=a.id WHERE a.username=? AND a.locked=0`, s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), query, input.Username).Scan(&target.ID, &target.Username, &target.Email, &salt, &verifier, &storedTOTP, &totpEnabled) != nil || !srp.Verify(target.Username, input.Password, salt, verifier) {
		problem(w, http.StatusUnauthorized, "Invalid game-account credentials")
		return
	}
	if totpEnabled {
		secret, decryptErr := s.decryptTOTP(storedTOTP)
		if (decryptErr != nil || !validTOTP(secret, input.Code, time.Now())) && !s.consumeRecoveryCode(r.Context(), target.ID, input.Code) {
			problem(w, http.StatusUnauthorized, "A valid authenticator or recovery code is required for that game account")
			return
		}
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	s.bindSessionIdentity(r, identityID)
	if input.Label == "" {
		input.Label = target.Username
	}
	if len(input.Label) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Label is too long")
		return
	}
	if _, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_identity_accounts(identity_id,account_id,label,is_primary) VALUES(?,?,?,0)`, identityID, target.ID, input.Label); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			problem(w, http.StatusConflict, "That game account is already linked")
		} else {
			problem(w, http.StatusInternalServerError, "Could not link game account")
		}
		return
	}
	s.auditIdentity(r, active.ID, "identity.account.link", uint64(target.ID), input.Label)
	jsonOut(w, http.StatusCreated, map[string]bool{"linked": true})
}

func (s *Server) identitySwitchAccount(w http.ResponseWriter, r *http.Request) {
	active, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"switched": true})
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid game account")
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	cookie, err := r.Cookie("portal_session")
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	hash := sha256.Sum256([]byte(cookie.Value))
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_sessions s JOIN portal_identity_accounts ia ON ia.identity_id=s.identity_id AND ia.account_id=? SET s.account_id=?,s.step_up_until=NULL WHERE s.token_hash=? AND s.identity_id=?`, id, id, hash[:], identityID)
	if err != nil {
		problem(w, http.StatusForbidden, "Game account is not linked to this master account")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusForbidden, "Game account is not linked to this master account")
		return
	}
	s.auditIdentity(r, active.ID, "identity.account.switch", id, "Active game account changed")
	jsonOut(w, http.StatusOK, map[string]bool{"switched": true})
}

func (s *Server) identityRenameAccount(w http.ResponseWriter, r *http.Request) {
	active, identityID, targetID, ok := s.identityMutationContext(w, r)
	if !ok {
		return
	}
	var input struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Label = strings.TrimSpace(input.Label)
	if input.Label == "" || len(input.Label) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Label must be between 1 and 100 characters")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"renamed": true})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_identity_accounts SET label=? WHERE identity_id=? AND account_id=?`, input.Label, identityID, targetID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not rename game account")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusNotFound, "Linked game account not found")
		return
	}
	s.auditIdentity(r, active.ID, "identity.account.rename", targetID, input.Label)
	jsonOut(w, http.StatusOK, map[string]bool{"renamed": true})
}

func (s *Server) identityPromoteAccount(w http.ResponseWriter, r *http.Request) {
	active, identityID, targetID, ok := s.identityMutationContext(w, r)
	if !ok {
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"primary": true})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update primary account")
		return
	}
	defer tx.Rollback()
	var linkedID uint64
	if err = tx.QueryRowContext(r.Context(), `SELECT account_id FROM portal_identity_accounts WHERE identity_id=? AND account_id=? FOR UPDATE`, identityID, targetID).Scan(&linkedID); err != nil {
		problem(w, http.StatusNotFound, "Linked game account not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal_identity_accounts SET is_primary=(account_id=?) WHERE identity_id=?`, targetID, identityID); err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not update primary account")
		return
	}
	s.auditIdentity(r, active.ID, "identity.account.primary", targetID, "Primary game account changed")
	jsonOut(w, http.StatusOK, map[string]bool{"primary": true})
}

func (s *Server) identityUnlinkAccount(w http.ResponseWriter, r *http.Request) {
	active, identityID, targetID, ok := s.identityMutationContext(w, r)
	if !ok {
		return
	}
	if targetID == uint64(active.ID) {
		problem(w, http.StatusConflict, "Switch to another game account before unlinking this one")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"unlinked": true})
		return
	}
	var primary bool
	if err := s.s.Auth.QueryRowContext(r.Context(), `SELECT is_primary FROM portal_identity_accounts WHERE identity_id=? AND account_id=?`, identityID, targetID).Scan(&primary); err != nil {
		problem(w, http.StatusNotFound, "Linked game account not found")
		return
	}
	if primary {
		problem(w, http.StatusConflict, "Choose another primary game account before unlinking this one")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_identity_accounts WHERE identity_id=? AND account_id=? AND is_primary=0`, identityID, targetID)
	changed, _ := result.RowsAffected()
	if err != nil || changed != 1 {
		problem(w, http.StatusInternalServerError, "Could not unlink game account")
		return
	}
	s.auditIdentity(r, active.ID, "identity.account.unlink", targetID, "Game account unlinked")
	jsonOut(w, http.StatusOK, map[string]bool{"unlinked": true})
}

func (s *Server) identityMutationContext(w http.ResponseWriter, r *http.Request) (account, uint64, uint64, bool) {
	active, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return account{}, 0, 0, false
	}
	targetID, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || targetID == 0 {
		problem(w, http.StatusBadRequest, "Invalid game account")
		return account{}, 0, 0, false
	}
	if s.c.MockMode {
		return active, 1, targetID, true
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return account{}, 0, 0, false
	}
	s.bindSessionIdentity(r, identityID)
	return active, identityID, targetID, true
}

func (s *Server) auditIdentity(r *http.Request, actorID uint32, action string, targetID uint64, details string) {
	if s.c.MockMode {
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id,ip_address,user_agent) VALUES(?,?,?,?,?,?,?,?)`, actorID, action, strconv.FormatUint(targetID, 10), details, s.c.RealmKey, RequestID(r.Context()), s.clientIP(r), truncate(r.UserAgent(), 500))
}

func (s *Server) bindSessionIdentity(r *http.Request, identityID uint64) {
	if s.c.MockMode {
		return
	}
	if cookie, err := r.Cookie("portal_session"); err == nil {
		hash := sha256.Sum256([]byte(cookie.Value))
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_sessions SET identity_id=? WHERE token_hash=? AND identity_id=0`, identityID, hash[:])
	}
}
