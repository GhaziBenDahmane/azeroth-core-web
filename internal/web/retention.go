package web

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type referralMilestone struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	ReferralCount uint32 `json:"referralCount"`
	RewardCredits uint32 `json:"rewardCredits"`
	Claimed       bool   `json:"claimed"`
	Available     bool   `json:"available"`
	Remaining     uint32 `json:"remaining"`
}

type voteCampaign struct {
	ID               uint64     `json:"id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	StartsAt         time.Time  `json:"startsAt"`
	EndsAt           time.Time  `json:"endsAt"`
	MinimumVotes     uint32     `json:"minimumVotes"`
	WinnerCount      uint32     `json:"winnerCount"`
	PrizeDescription string     `json:"prizeDescription"`
	TargetEntries    uint32     `json:"targetEntries"`
	CommunityReward  string     `json:"communityRewardDescription"`
	GoalReached      bool       `json:"goalReached"`
	Commitment       string     `json:"commitment"`
	Seed             string     `json:"seed,omitempty"`
	Status           string     `json:"status"`
	DrawnAt          *time.Time `json:"drawnAt,omitempty"`
	TotalEntries     uint32     `json:"totalEntries"`
	ParticipantCount uint32     `json:"participantCount"`
	ViewerEntries    uint32     `json:"viewerEntries"`
	Winners          []any      `json:"winners"`
}

type voteCampaignInput struct {
	Name, Description, StartsAt, EndsAt, PrizeDescription, CommunityRewardDescription string
	MinimumVotes, WinnerCount, TargetEntries                                          uint32
}

func validateVoteCampaignInput(in *voteCampaignInput, start, end time.Time) error {
	in.Name, in.Description, in.PrizeDescription, in.CommunityRewardDescription = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), strings.TrimSpace(in.PrizeDescription), strings.TrimSpace(in.CommunityRewardDescription)
	goalIncomplete := (in.TargetEntries == 0) != (in.CommunityRewardDescription == "")
	if start.IsZero() || end.IsZero() || !end.After(start) || len(in.Name) < 3 || len(in.Name) > 120 || len(in.Description) > 500 || len(in.PrizeDescription) < 2 || len(in.PrizeDescription) > 255 || in.MinimumVotes == 0 || in.WinnerCount == 0 || in.WinnerCount > 100 || in.TargetEntries > 10000000 || len(in.CommunityRewardDescription) > 255 || goalIncomplete {
		return fmt.Errorf("valid title, period, minimum votes, winner count, prize, and complete community-goal fields are required")
	}
	return nil
}

func (s *Server) seedReferralMilestones(r *http.Request) {
	if s.c.MockMode {
		return
	}
	defaults := []struct {
		name    string
		count   uint32
		credits uint32
	}{{"First recruit", 1, 10}, {"Party formed", 3, 30}, {"Guild builder", 5, 75}, {"Community champion", 10, 200}}
	for order, item := range defaults {
		_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT IGNORE INTO portal_referral_milestones(realm_key,name,referral_count,reward_credits,sort_order) VALUES(?,?,?,?,?)`, s.c.RealmKey, item.name, item.count, item.credits, order)
	}
}

func (s *Server) loadReferralMilestones(r *http.Request, accountID, uses uint32) []referralMilestone {
	if s.c.MockMode {
		return []referralMilestone{{ID: 1, Name: "First recruit", ReferralCount: 1, RewardCredits: 10, Claimed: true}, {ID: 2, Name: "Party formed", ReferralCount: 3, RewardCredits: 30, Available: true}, {ID: 3, Name: "Guild builder", ReferralCount: 5, RewardCredits: 75, Remaining: 2}}
	}
	s.seedReferralMilestones(r)
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT m.id,m.name,m.referral_count,m.reward_credits,(c.account_id IS NOT NULL)
		FROM portal_referral_milestones m LEFT JOIN portal_referral_milestone_claims c ON c.milestone_id=m.id AND c.account_id=?
		WHERE m.realm_key=? AND m.active=1 ORDER BY m.referral_count,m.sort_order,m.id`, accountID, s.c.RealmKey)
	if err != nil {
		return []referralMilestone{}
	}
	defer rows.Close()
	out := []referralMilestone{}
	for rows.Next() {
		var item referralMilestone
		if rows.Scan(&item.ID, &item.Name, &item.ReferralCount, &item.RewardCredits, &item.Claimed) == nil {
			item.Available = !item.Claimed && uses >= item.ReferralCount
			if uses < item.ReferralCount {
				item.Remaining = item.ReferralCount - uses
			}
			out = append(out, item)
		}
	}
	return out
}

func (s *Server) claimReferralMilestone(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid referral milestone")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": 30, "balance": 530})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var required, reward, uses uint32
	if err = tx.QueryRowContext(r.Context(), `SELECT referral_count,reward_credits FROM portal_referral_milestones WHERE id=? AND realm_key=? AND active=1 FOR UPDATE`, id, s.c.RealmKey).Scan(&required, &reward); err != nil {
		problem(w, http.StatusNotFound, "Referral milestone not found")
		return
	}
	if err = tx.QueryRowContext(r.Context(), `SELECT uses FROM portal_referrals WHERE account_id=? FOR UPDATE`, a.ID).Scan(&uses); err != nil || uses < required {
		problem(w, http.StatusConflict, "This referral milestone is not unlocked yet")
		return
	}
	result, err := tx.ExecContext(r.Context(), `INSERT IGNORE INTO portal_referral_milestone_claims(milestone_id,account_id,credits) VALUES(?,?,?)`, id, a.ID, reward)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not claim referral milestone")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		problem(w, http.StatusConflict, "Referral milestone already claimed")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)`, a.ID, reward); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)`, a.ID, reward, fmt.Sprintf("Referral milestone: %d recruits", required))
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not credit referral milestone")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT balance FROM portal_wallets WHERE account_id=?`, a.ID).Scan(&balance)
	s.notifyAccount(r.Context(), a.ID, "reward", "Referral milestone claimed", fmt.Sprintf("%d credits were added to your wallet.", reward), "/account/rewards")
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": reward, "balance": balance})
}

func (s *Server) discordRewardCallback(w http.ResponseWriter, r *http.Request) {
	secret := strings.TrimSpace(s.c.DiscordBotRewardSecret)
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if secret == "" || len(secret) != len(provided) || subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) != 1 {
		problem(w, http.StatusUnauthorized, "Invalid Discord reward credentials")
		return
	}
	var in struct {
		DiscordUserID string         `json:"discordUserId"`
		EventID       string         `json:"eventId"`
		Credits       uint32         `json:"credits"`
		Reason        string         `json:"reason"`
		Metadata      map[string]any `json:"metadata"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.DiscordUserID, in.EventID, in.Reason = strings.TrimSpace(in.DiscordUserID), strings.TrimSpace(in.EventID), strings.TrimSpace(in.Reason)
	if in.DiscordUserID == "" || len(in.DiscordUserID) > 128 || in.EventID == "" || len(in.EventID) > 128 || in.Credits == 0 || in.Credits > 10000 || in.Reason == "" || len(in.Reason) > 255 {
		problem(w, http.StatusUnprocessableEntity, "discordUserId, unique eventId, reason, and 1–10,000 credits are required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": in.Credits, "accountId": 1})
		return
	}
	metadata, _ := json.Marshal(in.Metadata)
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var identityID uint64
	var accountID uint32
	if err = tx.QueryRowContext(r.Context(), `SELECT p.identity_id,ia.account_id FROM portal_identity_providers p JOIN portal_identity_accounts ia ON ia.identity_id=p.identity_id AND ia.is_primary=1 WHERE p.provider='discord' AND p.provider_user_id=? LIMIT 1 FOR UPDATE`, in.DiscordUserID).Scan(&identityID, &accountID); err != nil {
		problem(w, http.StatusNotFound, "Discord account is not linked to a portal identity")
		return
	}
	result, err := tx.ExecContext(r.Context(), `INSERT IGNORE INTO portal_external_reward_events(provider,provider_event_id,realm_key,provider_user_id,identity_id,account_id,credits,reason,metadata_json) VALUES('discord',?,?,?,?,?,?,?,?)`, in.EventID, s.c.RealmKey, in.DiscordUserID, identityID, accountID, in.Credits, in.Reason, metadata)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not record Discord reward")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true, "credits": 0})
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)`, accountID, in.Credits); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)`, accountID, in.Credits, "Discord reward: "+in.Reason)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not credit Discord reward")
		return
	}
	s.notifyAccount(r.Context(), accountID, "reward", "Discord reward received", fmt.Sprintf("%d credits were added for %s.", in.Credits, in.Reason), "/account/rewards")
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": in.Credits, "accountId": accountID})
}

func (s *Server) voteCampaigns(w http.ResponseWriter, r *http.Request) {
	viewer := uint32(0)
	if a, err := s.auth(r); err == nil {
		viewer = a.ID
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"campaigns": []voteCampaign{{ID: 1, Name: "August supporter draw", Description: "Every verified vote is one entry.", StartsAt: time.Now().Add(-14 * 24 * time.Hour), EndsAt: time.Now().Add(2 * 24 * time.Hour), MinimumVotes: 2, WinnerCount: 3, PrizeDescription: "Spectral Tiger mount", TargetEntries: 200, CommunityReward: "A realm-wide bonus weekend", Commitment: "7c2d…demo", Status: "active", TotalEntries: 148, ParticipantCount: 34, ViewerEntries: 4, Winners: []any{}}}})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,name,description,starts_at,ends_at,minimum_votes,winner_count,prize_description,target_entries,community_reward_description,seed_commitment,status,draw_seed,drawn_at FROM portal_vote_campaigns WHERE realm_key=? ORDER BY starts_at DESC LIMIT 20`, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load voting campaigns")
		return
	}
	defer rows.Close()
	campaigns := []voteCampaign{}
	for rows.Next() {
		var item voteCampaign
		var seed string
		var drawn sql.NullTime
		if rows.Scan(&item.ID, &item.Name, &item.Description, &item.StartsAt, &item.EndsAt, &item.MinimumVotes, &item.WinnerCount, &item.PrizeDescription, &item.TargetEntries, &item.CommunityReward, &item.Commitment, &item.Status, &seed, &drawn) != nil {
			continue
		}
		if drawn.Valid {
			item.DrawnAt = &drawn.Time
			item.Seed = seed
		}
		_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*),COUNT(DISTINCT account_id),SUM(account_id=?) FROM portal_vote_events WHERE realm_key=? AND voted_at>=? AND voted_at<?`, viewer, s.c.RealmKey, item.StartsAt, item.EndsAt).Scan(&item.TotalEntries, &item.ParticipantCount, &item.ViewerEntries)
		item.GoalReached = item.TargetEntries > 0 && item.TotalEntries >= item.TargetEntries
		winnerRows, _ := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT w.rank_no,a.username,w.vote_count,w.draw_hash FROM portal_vote_campaign_winners w JOIN %s.account a ON a.id=w.account_id WHERE w.campaign_id=? ORDER BY w.rank_no`, s.c.AuthDB), item.ID)
		item.Winners = []any{}
		if winnerRows != nil {
			for winnerRows.Next() {
				var rank, votes uint32
				var username, drawHash string
				if winnerRows.Scan(&rank, &username, &votes, &drawHash) == nil {
					item.Winners = append(item.Winners, map[string]any{"rank": rank, "username": username, "votes": votes, "drawHash": drawHash})
				}
			}
			winnerRows.Close()
		}
		campaigns = append(campaigns, item)
	}
	jsonOut(w, http.StatusOK, map[string]any{"campaigns": campaigns})
}

func (s *Server) adminVoteCampaigns(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	if r.Method == http.MethodGet {
		s.voteCampaigns(w, r)
		return
	}
	var in voteCampaignInput
	if !decode(w, r, &in) {
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": 2, "commitment": "demo-commitment"})
		return
	}
	start, errStart := time.Parse(time.RFC3339, in.StartsAt)
	end, errEnd := time.Parse(time.RFC3339, in.EndsAt)
	if errStart != nil || errEnd != nil || validateVoteCampaignInput(&in, start, end) != nil {
		problem(w, http.StatusUnprocessableEntity, "Valid title, period, minimum votes, winner count, prize, and complete community-goal fields are required")
		return
	}
	seedBytes := make([]byte, 32)
	if _, err := rand.Read(seedBytes); err != nil {
		problem(w, http.StatusInternalServerError, "Could not prepare campaign draw")
		return
	}
	seed := hex.EncodeToString(seedBytes)
	commit := sha256.Sum256([]byte(seed))
	status := "scheduled"
	if !start.After(time.Now()) {
		status = "active"
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_vote_campaigns(realm_key,name,description,starts_at,ends_at,minimum_votes,winner_count,prize_description,target_entries,community_reward_description,draw_seed,seed_commitment,status,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, in.Name, in.Description, start, end, in.MinimumVotes, in.WinnerCount, in.PrizeDescription, in.TargetEntries, in.CommunityRewardDescription, seed, hex.EncodeToString(commit[:]), status, actor.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not create voting campaign")
		return
	}
	id, _ := result.LastInsertId()
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id, "commitment": hex.EncodeToString(commit[:])})
}

type drawTicket struct {
	accountID uint32
	votes     uint32
	hash      string
}

func (s *Server) drawVoteCampaign(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid voting campaign")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "winners": 3, "seed": "demo-seed"})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var seed, status string
	var starts, ends time.Time
	var minimum, winnerCount uint32
	if err = tx.QueryRowContext(r.Context(), `SELECT draw_seed,status,starts_at,ends_at,minimum_votes,winner_count FROM portal_vote_campaigns WHERE id=? AND realm_key=? FOR UPDATE`, id, s.c.RealmKey).Scan(&seed, &status, &starts, &ends, &minimum, &winnerCount); err != nil {
		problem(w, http.StatusNotFound, "Voting campaign not found")
		return
	}
	if status == "drawn" {
		problem(w, http.StatusConflict, "Voting campaign was already drawn")
		return
	}
	if time.Now().Before(ends) {
		problem(w, http.StatusConflict, "Voting campaign has not ended")
		return
	}
	type voteRow struct {
		id      uint64
		account uint32
	}
	voteRows, err := tx.QueryContext(r.Context(), `SELECT id,account_id FROM portal_vote_events WHERE realm_key=? AND voted_at>=? AND voted_at<? ORDER BY id`, s.c.RealmKey, starts, ends)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load campaign entries")
		return
	}
	votes := map[uint32]uint32{}
	all := []voteRow{}
	for voteRows.Next() {
		var row voteRow
		if voteRows.Scan(&row.id, &row.account) == nil {
			votes[row.account]++
			all = append(all, row)
		}
	}
	voteRows.Close()
	tickets := []drawTicket{}
	for _, row := range all {
		if votes[row.account] < minimum {
			continue
		}
		hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", seed, row.id, row.account)))
		tickets = append(tickets, drawTicket{accountID: row.account, votes: votes[row.account], hash: hex.EncodeToString(hash[:])})
	}
	sort.Slice(tickets, func(i, j int) bool { return tickets[i].hash < tickets[j].hash })
	selected := map[uint32]bool{}
	rank := uint32(0)
	for _, ticket := range tickets {
		if selected[ticket.accountID] {
			continue
		}
		rank++
		selected[ticket.accountID] = true
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_vote_campaign_winners(campaign_id,account_id,rank_no,vote_count,draw_hash) VALUES(?,?,?,?,?)`, id, ticket.accountID, rank, ticket.votes, ticket.hash); err != nil {
			problem(w, http.StatusInternalServerError, "Could not record campaign winners")
			return
		}
		if rank >= winnerCount {
			break
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal_vote_campaigns SET status='drawn',drawn_at=NOW() WHERE id=?`, id); err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not finalize campaign draw")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'vote-campaign.draw',?,?)`, actor.ID, strconv.FormatUint(id, 10), fmt.Sprintf("Selected %d winners using committed seed", rank))
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "winners": rank, "seed": seed})
}
