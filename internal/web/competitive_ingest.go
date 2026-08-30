package web

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

type competitiveMember struct {
	GUID  uint32 `json:"guid"`
	Name  string `json:"name"`
	Class uint8  `json:"class"`
	Role  string `json:"role"`
}

func (s *Server) authorizeCompetitiveIngest(w http.ResponseWriter, r *http.Request) bool {
	secret := strings.TrimSpace(s.c.CompetitiveIngestSecret)
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if secret == "" || len(secret) != len(provided) || subtle.ConstantTimeCompare([]byte(secret), []byte(provided)) != 1 {
		problem(w, http.StatusUnauthorized, "Invalid ingestion credentials")
		return false
	}
	return true
}

func (s *Server) ingestRaidKill(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeCompetitiveIngest(w, r) {
		return
	}
	var in struct {
		EventID                           string `json:"eventId"`
		GuildID                           uint32 `json:"guildId"`
		GuildName, Raid, Boss, Difficulty string
		Result                            string              `json:"result"`
		AttemptNumber                     uint32              `json:"attemptNumber"`
		BossHealthPercent                 float64             `json:"bossHealthPercent"`
		DurationSeconds                   uint32              `json:"durationSeconds"`
		KilledAt                          time.Time           `json:"killedAt"`
		OccurredAt                        time.Time           `json:"occurredAt"`
		Members                           []competitiveMember `json:"members"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.EventID = strings.TrimSpace(in.EventID)
	in.GuildName = strings.TrimSpace(in.GuildName)
	in.Raid = strings.TrimSpace(in.Raid)
	in.Boss = strings.TrimSpace(in.Boss)
	in.Difficulty = strings.TrimSpace(in.Difficulty)
	in.Result = strings.ToLower(strings.TrimSpace(in.Result))
	if in.Result == "" {
		in.Result = "kill"
	}
	if in.OccurredAt.IsZero() {
		in.OccurredAt = in.KilledAt
	}
	if in.KilledAt.IsZero() {
		in.KilledAt = in.OccurredAt
	}
	if in.EventID == "" || len(in.EventID) > 128 || in.GuildName == "" || len(in.GuildName) > 80 || in.Raid == "" || len(in.Raid) > 80 || in.Boss == "" || len(in.Boss) > 80 || in.Difficulty == "" || len(in.Difficulty) > 30 || (in.Result != "kill" && in.Result != "wipe") || in.OccurredAt.IsZero() || len(in.Members) > 40 || in.OccurredAt.After(time.Now().Add(5*time.Minute)) || in.DurationSeconds > 21600 || in.BossHealthPercent < 0 || in.BossHealthPercent > 100 {
		problem(w, 422, "Invalid raid event")
		return
	}
	if in.Result == "kill" {
		in.BossHealthPercent = 0
	}
	rules := s.loadRaidEligibilityRules(r)
	reasons := []string{}
	memberNames, memberGUIDs := map[string]bool{}, map[uint32]bool{}
	for index := range in.Members {
		member := &in.Members[index]
		member.Name, member.Role = strings.TrimSpace(member.Name), strings.ToLower(strings.TrimSpace(member.Role))
		nameKey := strings.ToLower(member.Name)
		if member.Name == "" || len(member.Name) > 32 || member.Class == 0 || member.Class > 11 || member.Class == 10 || (member.Role != "" && member.Role != "tank" && member.Role != "healer" && member.Role != "damage" && member.Role != "dps") || memberNames[nameKey] || member.GUID > 0 && memberGUIDs[member.GUID] {
			problem(w, 422, "Invalid or duplicate raid member")
			return
		}
		memberNames[nameKey] = true
		if member.GUID > 0 {
			memberGUIDs[member.GUID] = true
		}
	}
	memberCount := len(in.Members)
	difficulty := strings.ToLower(in.Difficulty)
	switch {
	case strings.Contains(difficulty, "10"):
		if memberCount < int(rules.MinMembers10) || memberCount > int(rules.MaxMembers10) {
			reasons = append(reasons, "roster is outside the configured 10-player bounds")
		}
	case strings.Contains(difficulty, "25"):
		if memberCount < int(rules.MinMembers25) || memberCount > int(rules.MaxMembers25) {
			reasons = append(reasons, "roster is outside the configured 25-player bounds")
		}
	default:
		reasons = append(reasons, "difficulty does not identify a 10- or 25-player raid")
	}
	if in.Result == "kill" && (in.DurationSeconds < rules.MinDurationSeconds || in.DurationSeconds > rules.MaxDurationSeconds) {
		reasons = append(reasons, "duration is outside configured bounds")
	}
	if in.OccurredAt.Before(time.Now().Add(-time.Duration(rules.MaxEventAgeHours) * time.Hour)) {
		reasons = append(reasons, "event is older than the configured ingestion window")
	}
	if in.GuildID == 0 {
		reasons = append(reasons, "guild identifier is missing")
	}
	if rules.RequireCharacterGUIDs {
		for _, member := range in.Members {
			if member.GUID == 0 {
				reasons = append(reasons, "one or more character identifiers are missing")
				break
			}
		}
	}
	eligible, eligibilityReason := len(reasons) == 0, "Verified signed ingestion event"
	if !eligible {
		eligibilityReason = strings.Join(reasons, "; ")
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_competitive_events(realm_key,source_event_id,event_type) VALUES(?,?,'raid')", s.c.RealmKey, in.EventID)
	if err != nil {
		problem(w, 500, "Could not record raid event")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		jsonOut(w, 200, map[string]any{"ok": true, "duplicate": true})
		return
	}
	attemptResult, err := tx.ExecContext(r.Context(), `INSERT INTO portal_raid_attempts(realm_key,source_event_id,guild_id,guild_name,raid,boss,difficulty,result,attempt_number,duration_seconds,boss_health_pct,occurred_at,verified_members,source_kind) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, in.EventID, in.GuildID, in.GuildName, in.Raid, in.Boss, in.Difficulty, in.Result, in.AttemptNumber, in.DurationSeconds, in.BossHealthPercent, in.OccurredAt, memberCount, "signed_ingest")
	if err != nil {
		problem(w, 500, "Could not store raid attempt")
		return
	}
	attemptID, _ := attemptResult.LastInsertId()
	for _, member := range in.Members {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_raid_attempt_members(attempt_id,character_guid,character_name,class_id,role_name) VALUES(?,?,?,?,?)", attemptID, member.GUID, member.Name, member.Class, strings.TrimSpace(member.Role)); err != nil {
			problem(w, 500, "Could not store raid attempt composition")
			return
		}
	}
	if in.Result == "wipe" {
		if err = tx.Commit(); err != nil {
			problem(w, 500, "Could not commit raid attempt")
			return
		}
		jsonOut(w, 201, map[string]any{"ok": true, "attemptId": attemptID, "result": in.Result})
		return
	}
	result, err = tx.ExecContext(r.Context(), "INSERT INTO portal_raid_kills(realm_key,guild_id,guild_name,raid,boss,difficulty,duration_seconds,killed_at,source_event_id,eligible,eligibility_reason,verified_members,source_kind) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)", s.c.RealmKey, in.GuildID, in.GuildName, in.Raid, in.Boss, in.Difficulty, in.DurationSeconds, in.OccurredAt, in.EventID, eligible, eligibilityReason, memberCount, "signed_ingest")
	if err != nil {
		problem(w, 500, "Could not store raid event")
		return
	}
	killID, _ := result.LastInsertId()
	for _, member := range in.Members {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_raid_members(kill_id,character_guid,character_name,class_id,role_name) VALUES(?,?,?,?,?)", killID, member.GUID, member.Name, member.Class, strings.TrimSpace(member.Role)); err != nil {
			problem(w, 500, "Could not store raid composition")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit raid event")
		return
	}
	jsonOut(w, 201, map[string]any{"ok": true, "id": killID, "attemptId": attemptID, "result": in.Result, "eligible": eligible, "eligibilityReason": eligibilityReason})
}

func (s *Server) ingestPvPMatch(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeCompetitiveIngest(w, r) {
		return
	}
	var in struct {
		EventID         string              `json:"eventId"`
		Season          string              `json:"season"`
		Bracket         uint8               `json:"bracket"`
		TeamID          uint32              `json:"teamId"`
		TeamName        string              `json:"teamName"`
		OpponentID      uint32              `json:"opponentId"`
		OpponentName    string              `json:"opponentName"`
		Result          string              `json:"result"`
		RatingBefore    uint16              `json:"ratingBefore"`
		RatingAfter     uint16              `json:"ratingAfter"`
		RatingChange    int16               `json:"ratingChange"`
		DurationSeconds uint32              `json:"durationSeconds"`
		PlayedAt        time.Time           `json:"playedAt"`
		Members         []competitiveMember `json:"members"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.EventID = strings.TrimSpace(in.EventID)
	in.Season = strings.TrimSpace(in.Season)
	if in.Season == "" {
		in.Season = "current"
	}
	in.TeamName = strings.TrimSpace(in.TeamName)
	in.OpponentName = strings.TrimSpace(in.OpponentName)
	in.Result = strings.ToLower(strings.TrimSpace(in.Result))
	if in.RatingBefore > 0 || in.RatingAfter > 0 {
		delta := int(in.RatingAfter) - int(in.RatingBefore)
		if delta < -32768 || delta > 32767 || (in.RatingChange != 0 && int(in.RatingChange) != delta) {
			problem(w, 422, "Arena rating values are inconsistent")
			return
		}
		in.RatingChange = int16(delta)
	}
	if in.EventID == "" || len(in.EventID) > 128 || in.Season == "" || len(in.Season) > 80 || (in.Bracket != 2 && in.Bracket != 3 && in.Bracket != 5) || (in.Result != "win" && in.Result != "loss") || in.TeamName == "" || len(in.TeamName) > 100 || in.OpponentName == "" || len(in.OpponentName) > 100 || in.PlayedAt.IsZero() || in.PlayedAt.After(time.Now().Add(5*time.Minute)) || in.DurationSeconds > 7200 || len(in.Members) == 0 || len(in.Members) > 5 {
		problem(w, 422, "Invalid PvP event")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_competitive_events(realm_key,source_event_id,event_type) VALUES(?,?,'pvp')", s.c.RealmKey, in.EventID)
	if err != nil {
		problem(w, 500, "Could not record PvP event")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		jsonOut(w, 200, map[string]any{"ok": true, "duplicate": true})
		return
	}
	result, err = tx.ExecContext(r.Context(), "INSERT INTO portal_pvp_matches(realm_key,source_event_id,season_slug,bracket,team_id,team_name,opponent_id,opponent_name,result,rating_before,rating_after,rating_change,duration_seconds,played_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", s.c.RealmKey, in.EventID, in.Season, in.Bracket, in.TeamID, in.TeamName, in.OpponentID, in.OpponentName, in.Result, in.RatingBefore, in.RatingAfter, in.RatingChange, in.DurationSeconds, in.PlayedAt)
	if err != nil {
		problem(w, 500, "Could not store PvP event")
		return
	}
	matchID, _ := result.LastInsertId()
	for _, member := range in.Members {
		member.Name = strings.TrimSpace(member.Name)
		if member.Name == "" || len(member.Name) > 32 {
			problem(w, 422, "Invalid PvP member")
			return
		}
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_pvp_match_members(match_id,character_guid,character_name) VALUES(?,?,?)", matchID, member.GUID, member.Name); err != nil {
			problem(w, 500, "Could not store PvP roster")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit PvP event")
		return
	}
	jsonOut(w, 201, map[string]any{"ok": true, "id": matchID})
}

func (s *Server) ingestBattlegroundMatch(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeCompetitiveIngest(w, r) {
		return
	}
	var in struct {
		EventID, Battleground, WinningTeam string
		DurationSeconds                    uint32 `json:"durationSeconds"`
		PlayedAt                           time.Time
		Members                            []struct {
			GUID                                 uint32 `json:"guid"`
			Name, Team                           string
			Class                                uint8 `json:"class"`
			KillingBlows, HonorableKills, Deaths uint32
			DamageDone, HealingDone              uint64
		}
	}
	if !decode(w, r, &in) {
		return
	}
	in.EventID, in.Battleground, in.WinningTeam = strings.TrimSpace(in.EventID), strings.TrimSpace(in.Battleground), strings.ToLower(strings.TrimSpace(in.WinningTeam))
	if in.EventID == "" || len(in.EventID) > 128 || in.Battleground == "" || len(in.Battleground) > 100 || (in.WinningTeam != "alliance" && in.WinningTeam != "horde") || in.PlayedAt.IsZero() || len(in.Members) == 0 || len(in.Members) > 80 || in.DurationSeconds > 86400 {
		problem(w, http.StatusUnprocessableEntity, "Invalid battleground event")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `INSERT IGNORE INTO portal_competitive_events(realm_key,source_event_id,event_type) VALUES(?,?,'battleground')`, s.c.RealmKey, in.EventID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not record battleground event")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true})
		return
	}
	result, err = tx.ExecContext(r.Context(), `INSERT INTO portal_battleground_matches(realm_key,source_event_id,battleground,winning_team,duration_seconds,played_at) VALUES(?,?,?,?,?,?)`, s.c.RealmKey, in.EventID, in.Battleground, in.WinningTeam, in.DurationSeconds, in.PlayedAt)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not store battleground event")
		return
	}
	matchID, _ := result.LastInsertId()
	for _, member := range in.Members {
		member.Name, member.Team = strings.TrimSpace(member.Name), strings.ToLower(strings.TrimSpace(member.Team))
		if member.Name == "" || len(member.Name) > 32 || (member.Team != "alliance" && member.Team != "horde") || member.Class == 0 || member.Class > 11 || member.Class == 10 {
			problem(w, http.StatusUnprocessableEntity, "Invalid battleground member")
			return
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_battleground_members(match_id,character_guid,character_name,team_name,class_id,killing_blows,honorable_kills,deaths,damage_done,healing_done) VALUES(?,?,?,?,?,?,?,?,?,?)`, matchID, member.GUID, member.Name, member.Team, member.Class, member.KillingBlows, member.HonorableKills, member.Deaths, member.DamageDone, member.HealingDone); err != nil {
			problem(w, http.StatusInternalServerError, "Could not store battleground roster")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not commit battleground event")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": matchID})
}
