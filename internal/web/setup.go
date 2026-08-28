package web

import (
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/example/azeroth-portal/internal/srp"
)

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	if !s.c.EnableSetup {
		jsonOut(w, http.StatusOK, map[string]any{"enabled": false, "required": false, "complete": true})
		return
	}
	complete, err := s.isSetupComplete(r)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Could not determine setup status")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"enabled": true, "required": !complete, "complete": complete})
}

func (s *Server) isSetupComplete(r *http.Request) (bool, error) {
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		return s.mock.setupDone, nil
	}
	var value string
	err := s.s.Auth.QueryRowContext(r.Context(), "SELECT setting_value FROM portal_settings WHERE setting_key='setup_complete'").Scan(&value)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if value == "1" {
		return true, nil
	}
	var hasGM bool
	if err = s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s`.account_access WHERE gmlevel>0)", s.c.AuthDB)).Scan(&hasGM); err != nil {
		return false, err
	}
	return hasGM, nil
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	if !s.c.EnableSetup {
		problem(w, http.StatusNotFound, "Setup is disabled")
		return
	}
	var in struct {
		Token, Username, Password, Email string
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.Token) != len(s.c.SetupToken) || subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.c.SetupToken)) != 1 {
		problem(w, http.StatusUnauthorized, "Invalid setup token")
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Email = strings.ToUpper(strings.TrimSpace(in.Email))
	if err := srp.Validate(in.Username, in.Password); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if len(in.Email) > 255 || !strings.Contains(in.Email, "@") {
		problem(w, http.StatusUnprocessableEntity, "Enter a valid email address")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if s.mock.setupDone {
			problem(w, http.StatusConflict, "Setup has already been completed")
			return
		}
		s.mock.users[in.Username] = in.Password
		s.mock.setupDone = true
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "username": in.Username, "gmLevel": s.c.SetupGMLevel})
		return
	}
	s.setupDatabase(w, r, in.Username, in.Password, in.Email)
}

func (s *Server) setupDatabase(w http.ResponseWriter, r *http.Request, username, password, email string) {
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var complete string
	if err = tx.QueryRowContext(r.Context(), "SELECT setting_value FROM portal_settings WHERE setting_key='setup_complete' FOR UPDATE").Scan(&complete); err != nil {
		problem(w, http.StatusInternalServerError, "Could not lock setup state")
		return
	}
	var hasGM bool
	if err = tx.QueryRowContext(r.Context(), fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s`.account_access WHERE gmlevel>0)", s.c.AuthDB)).Scan(&hasGM); err != nil {
		problem(w, http.StatusInternalServerError, "Could not inspect administrator accounts")
		return
	}
	if complete == "1" || hasGM {
		problem(w, http.StatusConflict, "Setup has already been completed")
		return
	}
	salt, verifier, err := srp.Registration(username, password)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not secure account")
		return
	}
	accountQuery := fmt.Sprintf("INSERT INTO `%s`.account (username,salt,verifier,email,reg_mail,expansion) VALUES (?,?,?,?,?,?)", s.c.AuthDB)
	result, err := tx.ExecContext(r.Context(), accountQuery, username, salt, verifier, email, email, s.c.Expansion)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			problem(w, http.StatusConflict, "That username is already taken")
		} else {
			problem(w, http.StatusInternalServerError, "Could not create administrator account")
		}
		return
	}
	accountID, _ := result.LastInsertId()
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?)", accountID, s.c.StartingCredits); err != nil {
		problem(w, http.StatusInternalServerError, "Could not initialize administrator wallet")
		return
	}
	realms := fmt.Sprintf("INSERT IGNORE INTO `%s`.realmcharacters (realmid,acctid,numchars) SELECT id,?,0 FROM `%s`.realmlist", s.c.AuthDB, s.c.AuthDB)
	if _, err = tx.ExecContext(r.Context(), realms, accountID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not initialize realm access")
		return
	}
	access := fmt.Sprintf("INSERT INTO `%s`.account_access(id,gmlevel,RealmID) VALUES(?,?,?)", s.c.AuthDB)
	if _, err = tx.ExecContext(r.Context(), access, accountID, s.c.SetupGMLevel, s.c.SetupGMRealmID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not grant GM access")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE portal_settings SET setting_value='1',updated_at=CURRENT_TIMESTAMP WHERE setting_key='setup_complete'"); err != nil {
		problem(w, http.StatusInternalServerError, "Could not finalize setup")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not commit setup")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "username": username, "gmLevel": s.c.SetupGMLevel})
}
