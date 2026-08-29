package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type dashboardService struct {
	Action, Character, Response string
	Success                     bool
	Created                     time.Time
}

func referralCode(username string, accountID uint32) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", strings.ToUpper(username), accountID)))
	return strings.ToUpper(username) + "-" + strings.ToUpper(hex.EncodeToString(sum[:3]))
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		claimed := s.mock.dailyClaim.Equal(time.Now().Local().Truncate(24 * time.Hour))
		services := append([]dashboardService(nil), s.mock.services...)
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]any{
			"dailyReward": map[string]any{"available": !claimed, "credits": 5, "streak": 4},
			"referral":    map[string]any{"code": "DEMO-7A3F21", "uses": 3, "creditsEarned": 75},
			"vote":        map[string]any{"url": s.c.VoteURL, "credits": s.c.VoteRewardCredits},
			"services":    services,
			"activity":    []map[string]any{{"kind": "login", "ip": "127.0.0.1", "agent": "Demo browser", "at": time.Now().Add(-20 * time.Minute)}, {"kind": "password", "ip": "127.0.0.1", "agent": "Demo browser", "at": time.Now().Add(-5 * 24 * time.Hour)}},
		})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	code := referralCode(a.Username, a.ID)
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT IGNORE INTO portal_referrals(account_id,code) VALUES(?,?)", a.ID, code)
	var uses, earned uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT code,uses,credits_earned FROM portal_referrals WHERE account_id=?", a.ID).Scan(&code, &uses, &earned)
	var claimedToday, streak uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*),COUNT(DISTINCT claim_date) FROM portal_daily_rewards WHERE account_id=? AND realm_key=? AND claim_date>=CURRENT_DATE-INTERVAL 6 DAY", a.ID, s.c.RealmKey).Scan(&claimedToday, &streak)
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_daily_rewards WHERE account_id=? AND realm_key=? AND claim_date=CURRENT_DATE", a.ID, s.c.RealmKey).Scan(&claimedToday)
	rows, _ := s.s.Auth.QueryContext(r.Context(), "SELECT action,character_name,success,response,created_at FROM portal_character_services WHERE account_id=? AND realm_key=? ORDER BY id DESC LIMIT 20", a.ID, s.c.RealmKey)
	services := []dashboardService{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var entry dashboardService
			if rows.Scan(&entry.Action, &entry.Character, &entry.Success, &entry.Response, &entry.Created) == nil {
				services = append(services, entry)
			}
		}
	}
	activity := []map[string]any{}
	activityRows, _ := s.s.Auth.QueryContext(r.Context(), "SELECT ip_address,user_agent,last_seen_at FROM portal_sessions WHERE account_id=? ORDER BY last_seen_at DESC LIMIT 20", a.ID)
	if activityRows != nil {
		defer activityRows.Close()
		for activityRows.Next() {
			var ip, agent string
			var at time.Time
			if activityRows.Scan(&ip, &agent, &at) == nil {
				activity = append(activity, map[string]any{"kind": "session", "ip": ip, "agent": agent, "at": at})
			}
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{
		"dailyReward": map[string]any{"available": claimedToday == 0, "credits": 5, "streak": streak},
		"referral":    map[string]any{"code": code, "uses": uses, "creditsEarned": earned},
		"vote":        map[string]any{"url": s.c.VoteURL, "credits": s.c.VoteRewardCredits},
		"services":    services,
		"activity":    activity,
	})
}

func (s *Server) claimDailyReward(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		today := time.Now().Local().Truncate(24 * time.Hour)
		if s.mock.dailyClaim.Equal(today) {
			problem(w, http.StatusConflict, "Daily reward already claimed")
			return
		}
		s.mock.dailyClaim = today
		s.mock.balance += 5
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": 5, "balance": s.mock.balance})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_daily_rewards(account_id,realm_key,claim_date,credits) VALUES(?,?,CURRENT_DATE,5)", a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not claim daily reward")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		problem(w, http.StatusConflict, "Daily reward already claimed")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance+5 WHERE account_id=?", a.ID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not credit daily reward")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,5,'Daily login reward')", a.ID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not record daily reward")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not commit daily reward")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance)
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": 5, "balance": balance})
}

// voteRewardCallback is called by the configured voting provider after it has
// verified a vote. A provider event ID makes retries idempotent.
func (s *Server) voteRewardCallback(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(s.c.VoteCallbackSecret)
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if secret == "" || len(provided) != len(secret) || subtle.ConstantTimeCompare([]byte(provided), []byte(secret)) != 1 {
		problem(w, http.StatusUnauthorized, "Invalid vote callback credentials")
		return
	}
	var in struct {
		Username string `json:"username"`
		EventID  string `json:"eventId"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.EventID = strings.TrimSpace(in.EventID)
	if !validAccountName(in.Username) || in.EventID == "" || len(in.EventID) > 128 || strings.ContainsAny(in.EventID, "\r\n\x00") || s.c.VoteRewardCredits <= 0 {
		problem(w, http.StatusUnprocessableEntity, "Invalid vote reward")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if _, exists := s.mock.users[in.Username]; !exists {
			problem(w, http.StatusNotFound, "Account not found")
			return
		}
		if s.mock.voteEvents == nil {
			s.mock.voteEvents = map[string]bool{}
		}
		if s.mock.voteEvents[in.EventID] {
			jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true, "credits": 0})
			return
		}
		s.mock.voteEvents[in.EventID] = true
		s.mock.balance += uint32(s.c.VoteRewardCredits)
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": s.c.VoteRewardCredits})
		return
	}
	var accountID uint32
	query := fmt.Sprintf("SELECT id FROM `%s`.account WHERE username=?", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), query, in.Username).Scan(&accountID) != nil {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_vote_rewards(event_id,account_id,realm_key,credits) VALUES(?,?,?,?)", in.EventID, accountID, s.c.RealmKey, s.c.VoteRewardCredits)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not record vote reward")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true, "credits": 0})
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", accountID, s.c.VoteRewardCredits); err != nil {
		problem(w, http.StatusInternalServerError, "Could not credit vote reward")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)", accountID, s.c.VoteRewardCredits, "Verified voting reward"); err != nil {
		problem(w, http.StatusInternalServerError, "Could not audit vote reward")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not commit vote reward")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": s.c.VoteRewardCredits})
}

func scanServiceHistory(rows *sql.Rows) []dashboardService {
	entries := []dashboardService{}
	for rows.Next() {
		var entry dashboardService
		if rows.Scan(&entry.Action, &entry.Character, &entry.Success, &entry.Response, &entry.Created) == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}
