package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type raidEligibilityRules struct {
	MinMembers10          uint8     `json:"minMembers10"`
	MaxMembers10          uint8     `json:"maxMembers10"`
	MinMembers25          uint8     `json:"minMembers25"`
	MaxMembers25          uint8     `json:"maxMembers25"`
	MinDurationSeconds    uint32    `json:"minDurationSeconds"`
	MaxDurationSeconds    uint32    `json:"maxDurationSeconds"`
	MaxEventAgeHours      uint32    `json:"maxEventAgeHours"`
	RequireCharacterGUIDs bool      `json:"requireCharacterGuids"`
	UpdatedAt             time.Time `json:"updatedAt,omitempty"`
}

func defaultRaidEligibilityRules() raidEligibilityRules {
	return raidEligibilityRules{MinMembers10: 8, MaxMembers10: 10, MinMembers25: 20, MaxMembers25: 25, MinDurationSeconds: 60, MaxDurationSeconds: 21600, MaxEventAgeHours: 168, RequireCharacterGUIDs: true}
}

func (s *Server) loadRaidEligibilityRules(r *http.Request) raidEligibilityRules {
	rules := defaultRaidEligibilityRules()
	if s.c.MockMode {
		return rules
	}
	err := s.s.Auth.QueryRowContext(r.Context(), `SELECT min_members_10,max_members_10,min_members_25,max_members_25,min_duration_seconds,max_duration_seconds,max_event_age_hours,require_character_guids,updated_at FROM portal_raid_eligibility_rules WHERE realm_key=?`, s.c.RealmKey).Scan(&rules.MinMembers10, &rules.MaxMembers10, &rules.MinMembers25, &rules.MaxMembers25, &rules.MinDurationSeconds, &rules.MaxDurationSeconds, &rules.MaxEventAgeHours, &rules.RequireCharacterGUIDs, &rules.UpdatedAt)
	if err != nil && err != sql.ErrNoRows {
		return defaultRaidEligibilityRules()
	}
	return rules
}

func validRaidEligibilityRules(rules raidEligibilityRules) bool {
	return rules.MinMembers10 > 0 && rules.MinMembers10 <= rules.MaxMembers10 && rules.MaxMembers10 <= 10 &&
		rules.MinMembers25 > 0 && rules.MinMembers25 <= rules.MaxMembers25 && rules.MaxMembers25 <= 25 &&
		rules.MinDurationSeconds >= 10 && rules.MinDurationSeconds < rules.MaxDurationSeconds && rules.MaxDurationSeconds <= 86400 &&
		rules.MaxEventAgeHours > 0 && rules.MaxEventAgeHours <= 24*365
}

func (s *Server) adminRaidEligibilityRules(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "realm")
	if !ok {
		problem(w, http.StatusForbidden, "Realm operator access required")
		return
	}
	if r.Method == http.MethodGet {
		jsonOut(w, http.StatusOK, map[string]any{"rules": s.loadRaidEligibilityRules(r)})
		return
	}
	var rules raidEligibilityRules
	if !decode(w, r, &rules) {
		return
	}
	if !validRaidEligibilityRules(rules) {
		problem(w, http.StatusUnprocessableEntity, "Invalid raid eligibility rules")
		return
	}
	if !s.c.MockMode {
		_, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_raid_eligibility_rules(realm_key,min_members_10,max_members_10,min_members_25,max_members_25,min_duration_seconds,max_duration_seconds,max_event_age_hours,require_character_guids,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE min_members_10=VALUES(min_members_10),max_members_10=VALUES(max_members_10),min_members_25=VALUES(min_members_25),max_members_25=VALUES(max_members_25),min_duration_seconds=VALUES(min_duration_seconds),max_duration_seconds=VALUES(max_duration_seconds),max_event_age_hours=VALUES(max_event_age_hours),require_character_guids=VALUES(require_character_guids),updated_by=VALUES(updated_by)`, s.c.RealmKey, rules.MinMembers10, rules.MaxMembers10, rules.MinMembers25, rules.MaxMembers25, rules.MinDurationSeconds, rules.MaxDurationSeconds, rules.MaxEventAgeHours, rules.RequireCharacterGUIDs, actor.ID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not save raid eligibility rules")
			return
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"rules": rules})
}

type rankingExclusion struct {
	ID        uint64     `json:"id"`
	Scope     string     `json:"scope"`
	Target    string     `json:"target"`
	Reason    string     `json:"reason"`
	Active    bool       `json:"active"`
	StartsAt  *time.Time `json:"startsAt,omitempty"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (s *Server) activeRankingExclusions(r *http.Request, scope string) []string {
	if s.c.MockMode {
		return []string{}
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT LOWER(target_key) FROM portal_ranking_exclusions WHERE realm_key=? AND scope=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW())`, s.c.RealmKey, scope)
	if err != nil {
		return []string{}
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var value string
		if rows.Scan(&value) == nil {
			out = append(out, value)
		}
	}
	return out
}

func (s *Server) adminRankingExclusions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "moderation")
	if !ok {
		problem(w, http.StatusForbidden, "Moderation permission required")
		return
	}
	if s.c.MockMode {
		if r.Method == http.MethodPost {
			jsonOut(w, http.StatusCreated, map[string]any{"id": 1})
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"exclusions": []rankingExclusion{}})
		return
	}
	if r.Method == http.MethodPost {
		var input rankingExclusion
		if !decode(w, r, &input) {
			return
		}
		input.Scope, input.Target, input.Reason = strings.ToLower(strings.TrimSpace(input.Scope)), strings.ToLower(strings.TrimSpace(input.Target)), strings.TrimSpace(input.Reason)
		if (input.Scope != "character" && input.Scope != "arena_team" && input.Scope != "guild") || input.Target == "" || len(input.Target) > 100 || len(input.Reason) < 3 || len(input.Reason) > 500 || input.EndsAt != nil && input.StartsAt != nil && !input.EndsAt.After(*input.StartsAt) {
			problem(w, http.StatusUnprocessableEntity, "Invalid ranking exclusion")
			return
		}
		result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_ranking_exclusions(realm_key,scope,target_key,reason,active,starts_at,ends_at,created_by) VALUES(?,?,?,?,1,?,?,?) ON DUPLICATE KEY UPDATE reason=VALUES(reason),active=1,starts_at=VALUES(starts_at),ends_at=VALUES(ends_at),created_by=VALUES(created_by)`, s.c.RealmKey, input.Scope, input.Target, input.Reason, input.StartsAt, input.EndsAt, actor.ID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not save ranking exclusion")
			return
		}
		id, _ := result.LastInsertId()
		jsonOut(w, http.StatusCreated, map[string]any{"id": id})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,scope,target_key,reason,active,starts_at,ends_at,created_at FROM portal_ranking_exclusions WHERE realm_key=? ORDER BY active DESC,created_at DESC LIMIT 500`, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load ranking exclusions")
		return
	}
	defer rows.Close()
	out := []rankingExclusion{}
	for rows.Next() {
		var item rankingExclusion
		if rows.Scan(&item.ID, &item.Scope, &item.Target, &item.Reason, &item.Active, &item.StartsAt, &item.EndsAt, &item.CreatedAt) == nil {
			out = append(out, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"exclusions": out})
}

func (s *Server) adminRankingExclusionDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "moderation"); !ok {
		problem(w, http.StatusForbidden, "Moderation permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid ranking exclusion")
		return
	}
	if !s.c.MockMode {
		if _, err = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_ranking_exclusions SET active=0 WHERE id=? AND realm_key=?`, id, s.c.RealmKey); err != nil {
			problem(w, http.StatusInternalServerError, "Could not remove ranking exclusion")
			return
		}
	}
	jsonOut(w, http.StatusOK, map[string]bool{"active": false})
}
