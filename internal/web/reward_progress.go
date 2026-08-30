package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var dailyRewardCycle = [...]uint32{5, 5, 7, 5, 8, 10, 15}

type loyaltyLevel struct {
	Name        string `json:"name"`
	Points      uint32 `json:"points"`
	Floor       uint32 `json:"floor"`
	NextName    string `json:"nextName,omitempty"`
	NextFloor   uint32 `json:"nextFloor,omitempty"`
	Remaining   uint32 `json:"remaining"`
	Description string `json:"description"`
}

type playerMission struct {
	ID              uint64 `json:"id"`
	Slug            string `json:"slug,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Category        string `json:"category"`
	Metric          string `json:"metric"`
	Target          uint32 `json:"target"`
	Progress        uint32 `json:"progress"`
	RewardCredits   uint32 `json:"rewardCredits"`
	Period          string `json:"period"`
	Claimed         bool   `json:"claimed"`
	Available       bool   `json:"available"`
	DataAvailable   bool   `json:"dataAvailable"`
	ProgressMessage string `json:"progressMessage,omitempty"`
	Active          bool   `json:"active"`
	SortOrder       int    `json:"sortOrder"`
}

func validatePlayerMission(mission playerMission) error {
	mission.Slug = strings.ToLower(strings.TrimSpace(mission.Slug))
	metrics := map[string]string{"raid_kills": "pve", "achievements": "pve", "honorable_kills": "pvp", "verified_votes": "community"}
	expectedCategory, validMetric := metrics[mission.Metric]
	if !voteSlugPattern.MatchString(mission.Slug) || strings.TrimSpace(mission.Name) == "" || len(mission.Name) > 120 || len(mission.Description) > 500 {
		return fmt.Errorf("a valid slug, name, and description are required")
	}
	if !validMetric || mission.Category != expectedCategory {
		return fmt.Errorf("mission category does not match its supported progress source")
	}
	if mission.Target == 0 || mission.Target > 1000000 || mission.RewardCredits == 0 || mission.RewardCredits > 100000 {
		return fmt.Errorf("target and reward must be within the supported range")
	}
	return nil
}

func (s *Server) adminPlayerMissions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,slug,name,description,category,metric,target_value,reward_credits,active,sort_order FROM portal_missions WHERE realm_key=? ORDER BY sort_order,id`, s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load missions")
			return
		}
		defer rows.Close()
		missions := []playerMission{}
		for rows.Next() {
			var mission playerMission
			if rows.Scan(&mission.ID, &mission.Slug, &mission.Name, &mission.Description, &mission.Category, &mission.Metric, &mission.Target, &mission.RewardCredits, &mission.Active, &mission.SortOrder) == nil {
				missions = append(missions, mission)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"missions": missions})
		return
	}
	var mission playerMission
	if !decode(w, r, &mission) {
		return
	}
	mission.Slug = strings.ToLower(strings.TrimSpace(mission.Slug))
	if err := validatePlayerMission(mission); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_missions(realm_key,slug,name,description,category,metric,target_value,reward_credits,active,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, mission.Slug, strings.TrimSpace(mission.Name), strings.TrimSpace(mission.Description), mission.Category, mission.Metric, mission.Target, mission.RewardCredits, mission.Active, mission.SortOrder)
	if err != nil {
		problem(w, http.StatusConflict, "Could not create mission; its slug may already exist")
		return
	}
	id, _ := result.LastInsertId()
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key) VALUES(?,'mission.create',?,?,?)`, actor.ID, strconv.FormatInt(id, 10), mission.Name, s.c.RealmKey)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Server) adminPlayerMission(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid mission")
		return
	}
	if r.Method == http.MethodDelete {
		result, execErr := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_missions SET active=0 WHERE id=? AND realm_key=?`, id, s.c.RealmKey)
		if execErr != nil {
			problem(w, http.StatusInternalServerError, "Could not disable mission")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			problem(w, http.StatusNotFound, "Mission not found")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key) VALUES(?,'mission.disable',?,'Mission disabled',?)`, actor.ID, strconv.FormatUint(id, 10), s.c.RealmKey)
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var mission playerMission
	if !decode(w, r, &mission) {
		return
	}
	mission.Slug = strings.ToLower(strings.TrimSpace(mission.Slug))
	if err := validatePlayerMission(mission); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_missions SET slug=?,name=?,description=?,category=?,metric=?,target_value=?,reward_credits=?,active=?,sort_order=? WHERE id=? AND realm_key=?`, mission.Slug, strings.TrimSpace(mission.Name), strings.TrimSpace(mission.Description), mission.Category, mission.Metric, mission.Target, mission.RewardCredits, mission.Active, mission.SortOrder, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusConflict, "Could not update mission; its slug may already exist")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Mission not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key) VALUES(?,'mission.update',?,?,?)`, actor.ID, strconv.FormatUint(id, 10), mission.Name, s.c.RealmKey)
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func loyaltyForPoints(points uint32) loyaltyLevel {
	levels := []struct {
		name, description string
		floor             uint32
	}{
		{"Initiate", "Your journey with the realm has begun.", 0},
		{"Adventurer", "A regular member of the realm community.", 30},
		{"Veteran", "A proven and active member of the realm.", 100},
		{"Champion", "A long-standing contributor to the community.", 250},
		{"Legend", "Among the realm's most loyal players.", 500},
	}
	current := levels[0]
	next := -1
	for index, level := range levels {
		if points >= level.floor {
			current = level
			if index+1 < len(levels) {
				next = index + 1
			} else {
				next = -1
			}
		}
	}
	result := loyaltyLevel{Name: current.name, Points: points, Floor: current.floor, Description: current.description}
	if next >= 0 {
		result.NextName = levels[next].name
		result.NextFloor = levels[next].floor
		result.Remaining = levels[next].floor - points
	}
	return result
}

func (s *Server) loadLoyalty(ctx context.Context, accountID uint32) loyaltyLevel {
	if s.c.MockMode {
		return loyaltyForPoints(138)
	}
	var joined time.Time
	var rewardDays, votes, referrals uint32
	query := fmt.Sprintf("SELECT joindate FROM `%s`.account WHERE id=?", s.c.AuthDB)
	if err := s.s.Auth.QueryRowContext(ctx, query, accountID).Scan(&joined); err != nil {
		joined = time.Now().UTC()
	}
	_ = s.s.Auth.QueryRowContext(ctx, "SELECT COUNT(*) FROM portal_daily_rewards WHERE account_id=? AND realm_key=?", accountID, s.c.RealmKey).Scan(&rewardDays)
	_ = s.s.Auth.QueryRowContext(ctx, "SELECT COUNT(*) FROM portal_vote_events WHERE account_id=? AND realm_key=?", accountID, s.c.RealmKey).Scan(&votes)
	_ = s.s.Auth.QueryRowContext(ctx, "SELECT COALESCE(uses,0) FROM portal_referrals WHERE account_id=?", accountID).Scan(&referrals)
	ageDays := uint32(0)
	if joined.Before(time.Now()) {
		ageDays = uint32(time.Since(joined).Hours() / 24)
	}
	// The formula is intentionally stable and visible in the UI: one point per
	// account day, three per daily claim, five per verified vote, and ten per recruit.
	return loyaltyForPoints(ageDays + rewardDays*3 + votes*5 + referrals*10)
}

func (s *Server) seedPlayerMissions(ctx context.Context) {
	defaults := []struct {
		slug, name, description, category, metric string
		target, reward                            uint32
	}{
		{"raid-vanguard", "Raid vanguard", "Take part in three verified raid boss kills this month.", "pve", "raid_kills", 3, 15},
		{"achievement-hunter", "Achievement hunter", "Earn ten character achievements this month.", "pve", "achievements", 10, 10},
		{"battle-hardened", "Battle-hardened", "Earn 100 honorable kills in verified battleground reports this month.", "pvp", "honorable_kills", 100, 15},
		{"realm-supporter", "Realm supporter", "Complete eight verified votes this month.", "community", "verified_votes", 8, 20},
	}
	for order, mission := range defaults {
		_, _ = s.s.Auth.ExecContext(ctx, `INSERT IGNORE INTO portal_missions(realm_key,slug,name,description,category,metric,target_value,reward_credits,sort_order) VALUES(?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, mission.slug, mission.name, mission.description, mission.category, mission.metric, mission.target, mission.reward, order)
	}
}

func (s *Server) accountCharacterGUIDs(ctx context.Context, accountID uint32) ([]uint32, error) {
	rows, err := s.s.Characters.QueryContext(ctx, "SELECT guid FROM characters WHERE account=? ORDER BY guid LIMIT 100", accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	guids := []uint32{}
	for rows.Next() {
		var guid uint32
		if err := rows.Scan(&guid); err != nil {
			return nil, err
		}
		guids = append(guids, guid)
	}
	return guids, rows.Err()
}

func (s *Server) missionProgress(ctx context.Context, accountID uint32, metric string, periodStart time.Time) (uint32, error) {
	switch metric {
	case "verified_votes":
		var value uint32
		err := s.s.Auth.QueryRowContext(ctx, "SELECT COUNT(*) FROM portal_vote_events WHERE account_id=? AND realm_key=? AND voted_at>=?", accountID, s.c.RealmKey, periodStart).Scan(&value)
		return value, err
	case "achievements":
		var value uint32
		err := s.s.Characters.QueryRowContext(ctx, `SELECT COUNT(*) FROM character_achievement ca JOIN characters c ON c.guid=ca.guid WHERE c.account=? AND ca.date>=?`, accountID, periodStart.Unix()).Scan(&value)
		return value, err
	case "raid_kills", "honorable_kills":
		guids, err := s.accountCharacterGUIDs(ctx, accountID)
		if err != nil || len(guids) == 0 {
			return 0, err
		}
		args := []any{s.c.RealmKey, periodStart}
		args = append(args, uintArgs(guids)...)
		var value uint32
		if metric == "raid_kills" {
			query := `SELECT COUNT(DISTINCT a.id) FROM portal_raid_attempts a JOIN portal_raid_attempt_members m ON m.attempt_id=a.id WHERE a.realm_key=? AND a.occurred_at>=? AND a.result='kill' AND m.character_guid IN (` + placeholders(len(guids)) + `)`
			err = s.s.Auth.QueryRowContext(ctx, query, args...).Scan(&value)
		} else {
			query := `SELECT COALESCE(SUM(m.honorable_kills),0) FROM portal_battleground_matches b JOIN portal_battleground_members m ON m.match_id=b.id WHERE b.realm_key=? AND b.played_at>=? AND m.character_guid IN (` + placeholders(len(guids)) + `)`
			err = s.s.Auth.QueryRowContext(ctx, query, args...).Scan(&value)
		}
		return value, err
	default:
		return 0, fmt.Errorf("unsupported mission metric %q", metric)
	}
}

func (s *Server) loadPlayerMissions(ctx context.Context, accountID uint32) []playerMission {
	periodStart := time.Now().UTC()
	periodStart = time.Date(periodStart.Year(), periodStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	period := periodStart.Format("2006-01")
	if s.c.MockMode {
		s.mock.mu.Lock()
		claims := make(map[uint64]bool, len(s.mock.missionClaims))
		for id, claimed := range s.mock.missionClaims {
			claims[id] = claimed
		}
		s.mock.mu.Unlock()
		missions := []playerMission{
			{ID: 1, Slug: "raid-vanguard", Name: "Raid vanguard", Description: "Take part in three verified raid boss kills this month.", Category: "pve", Metric: "raid_kills", Target: 3, Progress: 2, RewardCredits: 15, Period: period, DataAvailable: true, Active: true},
			{ID: 2, Slug: "achievement-hunter", Name: "Achievement hunter", Description: "Earn ten character achievements this month.", Category: "pve", Metric: "achievements", Target: 10, Progress: 10, RewardCredits: 10, Period: period, Available: true, DataAvailable: true, Active: true, SortOrder: 1},
			{ID: 3, Slug: "battle-hardened", Name: "Battle-hardened", Description: "Earn 100 honorable kills in verified battleground reports this month.", Category: "pvp", Metric: "honorable_kills", Target: 100, Progress: 64, RewardCredits: 15, Period: period, DataAvailable: true, Active: true, SortOrder: 2},
			{ID: 4, Slug: "realm-supporter", Name: "Realm supporter", Description: "Complete eight verified votes this month.", Category: "community", Metric: "verified_votes", Target: 8, Progress: 4, RewardCredits: 20, Period: period, DataAvailable: true, Active: true, SortOrder: 3},
		}
		for index := range missions {
			missions[index].Claimed = claims[missions[index].ID]
			if missions[index].Claimed {
				missions[index].Available = false
			}
		}
		return missions
	}
	s.seedPlayerMissions(ctx)
	rows, err := s.s.Auth.QueryContext(ctx, `SELECT m.id,m.name,m.description,m.category,m.metric,m.target_value,m.reward_credits,(c.account_id IS NOT NULL) FROM portal_missions m LEFT JOIN portal_mission_claims c ON c.mission_id=m.id AND c.account_id=? AND c.period_key=? WHERE m.realm_key=? AND m.active=1 ORDER BY m.sort_order,m.id`, accountID, period, s.c.RealmKey)
	if err != nil {
		return []playerMission{}
	}
	defer rows.Close()
	missions := []playerMission{}
	for rows.Next() {
		var mission playerMission
		if rows.Scan(&mission.ID, &mission.Name, &mission.Description, &mission.Category, &mission.Metric, &mission.Target, &mission.RewardCredits, &mission.Claimed) != nil {
			continue
		}
		mission.Period = period
		progress, progressErr := s.missionProgress(ctx, accountID, mission.Metric, periodStart)
		mission.DataAvailable = progressErr == nil
		if progressErr != nil {
			mission.ProgressMessage = "Realm telemetry is not available for this mission yet."
		} else {
			mission.Progress = progress
			mission.Available = !mission.Claimed && progress >= mission.Target
		}
		missions = append(missions, mission)
	}
	return missions
}

func (s *Server) claimPlayerMission(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid mission")
		return
	}
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if s.mock.missionClaims == nil {
			s.mock.missionClaims = map[uint64]bool{}
		}
		if id != 2 || s.mock.missionClaims[id] {
			problem(w, http.StatusConflict, "Mission is not ready to claim")
			return
		}
		s.mock.missionClaims[id] = true
		s.mock.balance += 10
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": 10, "balance": s.mock.balance})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var metric, name string
	var target, reward uint32
	if err = s.s.Auth.QueryRowContext(r.Context(), `SELECT metric,name,target_value,reward_credits FROM portal_missions WHERE id=? AND realm_key=? AND active=1`, id, s.c.RealmKey).Scan(&metric, &name, &target, &reward); err != nil {
		problem(w, http.StatusNotFound, "Mission not found")
		return
	}
	periodStart := time.Now().UTC()
	periodStart = time.Date(periodStart.Year(), periodStart.Month(), 1, 0, 0, 0, 0, time.UTC)
	progress, err := s.missionProgress(r.Context(), a.ID, metric, periodStart)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Mission progress is temporarily unavailable")
		return
	}
	if progress < target {
		problem(w, http.StatusConflict, "Mission target has not been reached")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	period := periodStart.Format("2006-01")
	result, err := tx.ExecContext(r.Context(), `INSERT IGNORE INTO portal_mission_claims(mission_id,account_id,period_key,credits) VALUES(?,?,?,?)`, id, a.ID, period, reward)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not claim mission")
		return
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		problem(w, http.StatusConflict, "Mission reward already claimed")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)`, a.ID, reward); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)`, a.ID, reward, "Monthly mission: "+name)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not credit mission reward")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT balance FROM portal_wallets WHERE account_id=?`, a.ID).Scan(&balance)
	s.notifyAccount(r.Context(), a.ID, "reward", "Mission complete", fmt.Sprintf("%d credits were added for %s.", reward, name), "/account/rewards")
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "credits": reward, "balance": balance})
}

func scanDailyRewardState(ctx context.Context, db *sql.DB, accountID uint32, realmKey string) (claimed bool, streak, todayCredits uint32) {
	rows, err := db.QueryContext(ctx, `SELECT claim_date,credits FROM portal_daily_rewards WHERE account_id=? AND realm_key=? AND claim_date<=UTC_DATE() ORDER BY claim_date DESC LIMIT 365`, accountID, realmKey)
	if err != nil {
		return false, 0, 0
	}
	defer rows.Close()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	expected := today
	for rows.Next() {
		var day time.Time
		var credits uint32
		if rows.Scan(&day, &credits) != nil {
			continue
		}
		day = day.UTC().Truncate(24 * time.Hour)
		if day.Equal(today) {
			claimed, todayCredits = true, credits
		}
		if day.Equal(expected) || (streak == 0 && day.Equal(today.Add(-24*time.Hour))) {
			streak++
			expected = day.Add(-24 * time.Hour)
		} else if day.Before(expected) {
			break
		}
	}
	return claimed, streak, todayCredits
}
