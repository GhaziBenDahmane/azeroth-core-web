package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var voteSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,49}$`)

type voteSite struct {
	ID              uint32     `json:"id"`
	Slug            string     `json:"slug"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	Description     string     `json:"description"`
	RewardCredits   uint32     `json:"rewardCredits"`
	CooldownMinutes uint32     `json:"cooldownMinutes"`
	Active          bool       `json:"active"`
	SortOrder       int32      `json:"sortOrder"`
	LastVote        *time.Time `json:"lastVote,omitempty"`
	AvailableAt     *time.Time `json:"availableAt,omitempty"`
	Available       bool       `json:"available"`
}

type voteHistoryEntry struct {
	ID       uint64    `json:"id"`
	SiteName string    `json:"siteName"`
	SiteSlug string    `json:"siteSlug"`
	Credits  uint32    `json:"credits"`
	VotedAt  time.Time `json:"votedAt"`
}

func (s *Server) votes(w http.ResponseWriter, r *http.Request) {
	accountID := uint32(0)
	if account, err := s.auth(r); err == nil {
		accountID = account.ID
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,slug,name,url,description,reward_credits,cooldown_minutes,active,sort_order,
		(SELECT MAX(voted_at) FROM portal_vote_events e WHERE e.site_id=portal_vote_sites.id AND e.account_id=?)
		FROM portal_vote_sites WHERE realm_key=? AND active=1 ORDER BY sort_order,name`, accountID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load voting sites")
		return
	}
	defer rows.Close()
	now := time.Now()
	sites := []voteSite{}
	for rows.Next() {
		var site voteSite
		var last sql.NullTime
		if err := rows.Scan(&site.ID, &site.Slug, &site.Name, &site.URL, &site.Description, &site.RewardCredits, &site.CooldownMinutes, &site.Active, &site.SortOrder, &last); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read voting sites")
			return
		}
		site.Available = accountID > 0
		if last.Valid {
			site.LastVote = &last.Time
			available := last.Time.Add(time.Duration(site.CooldownMinutes) * time.Minute)
			site.AvailableAt = &available
			site.Available = accountID > 0 && !available.After(now)
		}
		sites = append(sites, site)
	}
	jsonOut(w, http.StatusOK, map[string]any{"sites": sites, "authenticated": accountID > 0})
}

func (s *Server) voteLeaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT a.username,COUNT(*),COALESCE(SUM(e.credits),0)
		FROM portal_vote_events e JOIN %s.account a ON a.id=e.account_id
		WHERE e.realm_key=? AND e.voted_at>=DATE_FORMAT(UTC_TIMESTAMP(),'%%Y-%%m-01')
		GROUP BY e.account_id,a.username ORDER BY COUNT(*) DESC,SUM(e.credits) DESC,a.username LIMIT 50`, s.c.AuthDB), s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load vote leaderboard")
		return
	}
	defer rows.Close()
	type row struct {
		Rank     uint32 `json:"rank"`
		Username string `json:"username"`
		Votes    uint32 `json:"votes"`
		Credits  uint32 `json:"credits"`
	}
	out := []row{}
	for rows.Next() {
		var item row
		item.Rank = uint32(len(out) + 1)
		if rows.Scan(&item.Username, &item.Votes, &item.Credits) == nil {
			out = append(out, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"period": time.Now().UTC().Format("2006-01"), "leaders": out})
}

func (s *Server) voteHistory(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in to view voting history")
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"summary": map[string]any{"total": 18, "thisMonth": 8, "currentStreakDays": 4}, "history": []voteHistoryEntry{
			{ID: 2, SiteName: "TopG", SiteSlug: "topg", Credits: 10, VotedAt: time.Now().Add(-8 * time.Hour)},
			{ID: 1, SiteName: "Xtremetop100", SiteSlug: "xtremetop100", Credits: 10, VotedAt: time.Now().Add(-32 * time.Hour)},
		}})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in to view voting history")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT e.id,s.name,s.slug,e.credits,e.voted_at
		FROM portal_vote_events e JOIN portal_vote_sites s ON s.id=e.site_id
		WHERE e.account_id=? AND e.realm_key=? ORDER BY e.voted_at DESC,e.id DESC LIMIT 100`, a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load voting history")
		return
	}
	defer rows.Close()
	history := []voteHistoryEntry{}
	for rows.Next() {
		var entry voteHistoryEntry
		if err := rows.Scan(&entry.ID, &entry.SiteName, &entry.SiteSlug, &entry.Credits, &entry.VotedAt); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read voting history")
			return
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not read voting history")
		return
	}
	rows.Close()
	var total, thisMonth uint32
	if err := s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*),COALESCE(SUM(voted_at>=DATE_FORMAT(UTC_TIMESTAMP(),'%Y-%m-01')),0) FROM portal_vote_events WHERE account_id=? AND realm_key=?`, a.ID, s.c.RealmKey).Scan(&total, &thisMonth); err != nil {
		problem(w, http.StatusInternalServerError, "Could not summarize voting history")
		return
	}
	dayRows, dayErr := s.s.Auth.QueryContext(r.Context(), `SELECT DISTINCT DATE(voted_at) FROM portal_vote_events WHERE account_id=? AND realm_key=? ORDER BY DATE(voted_at) DESC LIMIT 366`, a.ID, s.c.RealmKey)
	if dayErr != nil {
		problem(w, http.StatusInternalServerError, "Could not calculate voting streak")
		return
	}
	days := []time.Time{}
	for dayRows.Next() {
		var day time.Time
		if err := dayRows.Scan(&day); err != nil {
			dayRows.Close()
			problem(w, http.StatusInternalServerError, "Could not calculate voting streak")
			return
		}
		days = append(days, day)
	}
	dayRows.Close()
	jsonOut(w, http.StatusOK, map[string]any{"summary": map[string]any{"total": total, "thisMonth": thisMonth, "currentStreakDays": consecutiveVoteDays(days, time.Now().UTC())}, "history": history})
}

func consecutiveVoteDays(days []time.Time, now time.Time) uint32 {
	if len(days) == 0 {
		return 0
	}
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	latest := time.Date(days[0].UTC().Year(), days[0].UTC().Month(), days[0].UTC().Day(), 0, 0, 0, 0, time.UTC)
	if latest.Before(today.AddDate(0, 0, -1)) {
		return 0
	}
	streak, expected := uint32(0), latest
	for _, raw := range days {
		day := time.Date(raw.UTC().Year(), raw.UTC().Month(), raw.UTC().Day(), 0, 0, 0, 0, time.UTC)
		if day.Equal(expected) {
			streak++
			expected = expected.AddDate(0, 0, -1)
		} else if day.Before(expected) {
			break
		}
	}
	return streak
}

func (s *Server) visitVoteSite(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid voting site")
		return
	}
	var target string
	var cooldown uint32
	var last sql.NullTime
	err = s.s.Auth.QueryRowContext(r.Context(), `SELECT url,cooldown_minutes,(SELECT MAX(voted_at) FROM portal_vote_events WHERE site_id=portal_vote_sites.id AND account_id=?) FROM portal_vote_sites WHERE id=? AND realm_key=? AND active=1`, a.ID, id, s.c.RealmKey).Scan(&target, &cooldown, &last)
	if err != nil {
		problem(w, http.StatusNotFound, "Voting site not found")
		return
	}
	if last.Valid && last.Time.Add(time.Duration(cooldown)*time.Minute).After(time.Now()) {
		problem(w, http.StatusConflict, "This voting site is still on cooldown")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_vote_clicks(site_id,account_id,realm_key) VALUES(?,?,?)", id, a.ID, s.c.RealmKey)
	jsonOut(w, http.StatusOK, map[string]string{"url": target})
}

func (s *Server) voteSiteCallback(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.c.VoteCallbackSecret == "" || len(provided) != len(s.c.VoteCallbackSecret) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.c.VoteCallbackSecret)) != 1 {
		problem(w, http.StatusUnauthorized, "Invalid vote callback credentials")
		return
	}
	var in struct {
		Username string `json:"username"`
		EventID  string `json:"eventId"`
		IP       string `json:"ip"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.EventID = strings.TrimSpace(in.EventID)
	if in.Username == "" || len(in.Username) > 32 || in.EventID == "" || len(in.EventID) > 128 {
		problem(w, http.StatusUnprocessableEntity, "username and a unique eventId are required")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var siteID, credits, cooldown, accountID uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT id,reward_credits,cooldown_minutes FROM portal_vote_sites WHERE slug=? AND realm_key=? AND active=1 FOR UPDATE", r.PathValue("slug"), s.c.RealmKey).Scan(&siteID, &credits, &cooldown); err != nil {
		problem(w, http.StatusNotFound, "Voting site not found")
		return
	}
	if err = tx.QueryRowContext(r.Context(), fmt.Sprintf("SELECT id FROM %s.account WHERE username=?", s.c.AuthDB), in.Username).Scan(&accountID); err != nil {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	var duplicate uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_vote_events WHERE site_id=? AND provider_event_id=?", siteID, in.EventID).Scan(&duplicate); err != nil {
		problem(w, http.StatusInternalServerError, "Could not validate vote event")
		return
	}
	if duplicate > 0 {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true, "credits": 0})
		return
	}
	var recent uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_vote_events WHERE site_id=? AND account_id=? AND voted_at>UTC_TIMESTAMP()-INTERVAL ? MINUTE", siteID, accountID, cooldown).Scan(&recent); err != nil || recent > 0 {
		problem(w, http.StatusConflict, "Vote is inside the site cooldown")
		return
	}
	var ipHash []byte
	if suppliedIP := strings.TrimSpace(in.IP); suppliedIP != "" {
		parsedIP := net.ParseIP(suppliedIP)
		if parsedIP == nil {
			problem(w, http.StatusUnprocessableEntity, "ip must be a valid voter IPv4 or IPv6 address")
			return
		}
		hash := sha256.Sum256([]byte(s.c.VoteCallbackSecret + "\x00" + parsedIP.String()))
		ipHash = hash[:]
		var recentIP uint32
		if err = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_vote_events WHERE site_id=? AND ip_hash=? AND voted_at>UTC_TIMESTAMP()-INTERVAL ? MINUTE", siteID, ipHash, cooldown).Scan(&recentIP); err != nil {
			problem(w, http.StatusInternalServerError, "Could not validate vote network cooldown")
			return
		}
		if recentIP > 0 {
			problem(w, http.StatusConflict, "This voting site was already rewarded from that network during its cooldown")
			return
		}
	}
	result, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_vote_events(site_id,account_id,realm_key,provider_event_id,credits,ip_hash) VALUES(?,?,?,?,?,?)", siteID, accountID, s.c.RealmKey, in.EventID, credits, ipHash[:])
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not record vote")
		return
	}
	inserted, _ := result.RowsAffected()
	if inserted == 0 {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true})
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", accountID, credits); err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)", accountID, credits, "Verified vote: "+r.PathValue("slug"))
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id) VALUES(0,'vote.reward',?,?,?,?)`, in.Username, fmt.Sprintf("%s awarded %d credits (provider event %s)", r.PathValue("slug"), credits, in.EventID), s.c.RealmKey, RequestID(r.Context()))
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not award vote")
		return
	}
	s.notifyAccount(r.Context(), accountID, "reward", "Vote reward received", fmt.Sprintf("%d credits were added to your wallet.", credits), "/account/rewards")
	s.notifyDiscordAsync("Verified vote", "**%s** earned %d credits from `%s` on **%s**.", in.Username, credits, r.PathValue("slug"), s.c.RealmName)
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": credits})
}

func validateVoteSite(site voteSite) error {
	site.Slug = strings.ToLower(strings.TrimSpace(site.Slug))
	if !voteSlugPattern.MatchString(site.Slug) || strings.TrimSpace(site.Name) == "" || len(site.Name) > 100 || len(site.Description) > 500 || site.CooldownMinutes < 5 || site.CooldownMinutes > 10080 || site.RewardCredits > 100000 {
		return fmt.Errorf("invalid voting-site name, slug, reward, or cooldown")
	}
	parsed, err := url.ParseRequestURI(site.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("voting-site URL must be absolute HTTP(S)")
	}
	return nil
}

func (s *Server) adminVoteSites(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,slug,name,url,description,reward_credits,cooldown_minutes,active,sort_order FROM portal_vote_sites WHERE realm_key=? ORDER BY sort_order,name", s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load voting sites")
			return
		}
		defer rows.Close()
		sites := []voteSite{}
		for rows.Next() {
			var site voteSite
			if rows.Scan(&site.ID, &site.Slug, &site.Name, &site.URL, &site.Description, &site.RewardCredits, &site.CooldownMinutes, &site.Active, &site.SortOrder) == nil {
				sites = append(sites, site)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"sites": sites})
		return
	}
	var site voteSite
	if !decode(w, r, &site) {
		return
	}
	site.Slug = strings.ToLower(strings.TrimSpace(site.Slug))
	if err := validateVoteSite(site); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_vote_sites(realm_key,slug,name,url,description,reward_credits,cooldown_minutes,active,sort_order) VALUES(?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, site.Slug, site.Name, site.URL, site.Description, site.RewardCredits, site.CooldownMinutes, site.Active, site.SortOrder)
	if err != nil {
		problem(w, http.StatusConflict, "Could not create voting site; its slug may already exist")
		return
	}
	id, _ := result.LastInsertId()
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'vote-site.create',?,?)", a.ID, strconv.FormatInt(id, 10), site.Name)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Server) adminVoteSite(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid voting site")
		return
	}
	if r.Method == http.MethodDelete {
		result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_vote_sites SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not disable voting site")
			return
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			problem(w, http.StatusNotFound, "Voting site not found")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'vote-site.disable',?,'Voting site disabled')", a.ID, strconv.FormatUint(id, 10))
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var site voteSite
	if !decode(w, r, &site) {
		return
	}
	site.Slug = strings.ToLower(strings.TrimSpace(site.Slug))
	if err := validateVoteSite(site); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_vote_sites SET slug=?,name=?,url=?,description=?,reward_credits=?,cooldown_minutes=?,active=?,sort_order=? WHERE id=? AND realm_key=?`, site.Slug, site.Name, site.URL, site.Description, site.RewardCredits, site.CooldownMinutes, site.Active, site.SortOrder, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusConflict, "Could not update voting site; its slug may already exist")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Voting site not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'vote-site.update',?,?)", a.ID, strconv.FormatUint(id, 10), site.Name)
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
