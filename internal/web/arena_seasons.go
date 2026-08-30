package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var arenaSeasonSlug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type arenaSeason struct {
	ID        uint64     `json:"id"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	Status    string     `json:"status"`
	StartsAt  *time.Time `json:"startsAt,omitempty"`
	EndsAt    *time.Time `json:"endsAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

func (s *Server) arenaSeasons(r *http.Request) []arenaSeason {
	if s.c.MockMode {
		now := time.Now()
		return []arenaSeason{{Name: "Current season", Slug: "current", Status: "active", CreatedAt: now}, {ID: 2, Name: "Season 7", Slug: "season-7", Status: "archived", CreatedAt: now.AddDate(0, -3, 0)}, {ID: 1, Name: "Season 6", Slug: "season-6", Status: "archived", CreatedAt: now.AddDate(0, -6, 0)}}
	}
	items := []arenaSeason{{Name: "Current season", Slug: "current", Status: "active", CreatedAt: time.Now()}}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,name,slug,status,starts_at,ends_at,created_at FROM portal_arena_seasons WHERE realm_key=? ORDER BY created_at DESC`, s.c.RealmKey)
	if err != nil {
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var item arenaSeason
		if rows.Scan(&item.ID, &item.Name, &item.Slug, &item.Status, &item.StartsAt, &item.EndsAt, &item.CreatedAt) == nil {
			items = append(items, item)
		}
	}
	return items
}

func (s *Server) archivedArenaRankings(w http.ResponseWriter, r *http.Request, slug string, bracket, page int) {
	const pageSize = 25
	offset := (page - 1) * pageSize
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT sn.rank_no,sn.team_id,sn.team_name,sn.rating,sn.season_games,sn.season_wins,sn.members_json,se.name FROM portal_arena_snapshots sn JOIN portal_arena_seasons se ON se.id=sn.season_id WHERE se.realm_key=? AND se.slug=? AND sn.bracket=? ORDER BY sn.rank_no LIMIT ? OFFSET ?`, s.c.RealmKey, slug, bracket, pageSize+1, offset)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load the archived season")
		return
	}
	defer rows.Close()
	teams, seasonName := []arenaTeam{}, ""
	for rows.Next() {
		var team arenaTeam
		var membersJSON string
		if rows.Scan(&team.Rank, &team.ID, &team.Name, &team.Rating, &team.SeasonGames, &team.SeasonWins, &membersJSON, &seasonName) == nil {
			team.Bracket = uint8(bracket)
			team.Members = []arenaMember{}
			_ = json.Unmarshal([]byte(membersJSON), &team.Members)
			teams = append(teams, team)
		}
	}
	if seasonName == "" {
		problem(w, http.StatusNotFound, "Arena season snapshot not found")
		return
	}
	hasMore := len(teams) > pageSize
	if hasMore {
		teams = teams[:pageSize]
	}
	jsonOut(w, http.StatusOK, map[string]any{"bracket": bracket, "teams": teams, "page": page, "hasMore": hasMore, "season": slug, "seasonName": seasonName, "source": "Immutable portal season snapshot", "seasons": s.arenaSeasons(r)})
}

func (s *Server) adminArenaSeasons(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "realm")
	if !ok {
		problem(w, http.StatusForbidden, "Realm operator permission required")
		return
	}
	if r.Method == http.MethodGet {
		jsonOut(w, http.StatusOK, map[string]any{"seasons": s.arenaSeasons(r)})
		return
	}
	var input struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Name, input.Slug = strings.TrimSpace(input.Name), strings.ToLower(strings.TrimSpace(input.Slug))
	if len(input.Name) < 2 || len(input.Name) > 100 || !arenaSeasonSlug.MatchString(input.Slug) || len(input.Slug) > 100 || input.Slug == "current" {
		problem(w, http.StatusUnprocessableEntity, "Provide a 2–100 character name and a URL-safe season slug")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]any{"id": 3, "capturedTeams": 12})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start season snapshot")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `INSERT INTO portal_arena_seasons(realm_key,name,slug,status,ends_at,created_by) VALUES(?,?,?,'archived',NOW(),?)`, s.c.RealmKey, input.Name, input.Slug, actor.ID)
	if err != nil {
		problem(w, http.StatusConflict, "That season slug already exists")
		return
	}
	seasonID, _ := result.LastInsertId()
	captured := 0
	for _, bracket := range []int{2, 3, 5} {
		query := fmt.Sprintf(`SELECT arenaTeamId,name,rating,seasonGames,seasonWins FROM %s.arena_team WHERE type=? AND seasonGames>0 ORDER BY rating DESC,seasonWins DESC,arenaTeamId LIMIT 500`, s.c.CharactersDB)
		rows, queryErr := s.s.Characters.QueryContext(r.Context(), query, bracket)
		if queryErr != nil {
			problem(w, http.StatusBadGateway, "Could not read live arena teams")
			return
		}
		rank := 0
		for rows.Next() {
			var team arenaTeam
			if rows.Scan(&team.ID, &team.Name, &team.Rating, &team.SeasonGames, &team.SeasonWins) != nil {
				continue
			}
			rank++
			team.Members = []arenaMember{}
			memberQuery := fmt.Sprintf(`SELECT c.name,c.class,m.personalRating,m.seasonGames,m.seasonWins FROM %s.arena_team_member m JOIN %s.characters c ON c.guid=m.guid WHERE m.arenaTeamId=? ORDER BY m.personalRating DESC`, s.c.CharactersDB, s.c.CharactersDB)
			if members, memberErr := s.s.Characters.QueryContext(r.Context(), memberQuery, team.ID); memberErr == nil {
				for members.Next() {
					var member arenaMember
					if members.Scan(&member.Name, &member.Class, &member.PersonalRating, &member.SeasonGames, &member.SeasonWins) == nil {
						team.Members = append(team.Members, member)
					}
				}
				members.Close()
			}
			membersJSON, _ := json.Marshal(team.Members)
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_arena_snapshots(season_id,bracket,rank_no,team_id,team_name,rating,season_games,season_wins,members_json) VALUES(?,?,?,?,?,?,?,?,?)`, seasonID, bracket, rank, team.ID, team.Name, team.Rating, team.SeasonGames, team.SeasonWins, string(membersJSON)); err != nil {
				rows.Close()
				problem(w, http.StatusInternalServerError, "Could not store season snapshot")
				return
			}
			captured++
		}
		rows.Close()
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not finalize season snapshot")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'arena.snapshot',?,?)`, actor.ID, input.Slug, fmt.Sprintf("teams=%d", captured))
	jsonOut(w, http.StatusCreated, map[string]any{"id": seasonID, "capturedTeams": captured})
}
