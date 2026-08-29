package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type talentInsight struct {
	Group  uint8    `json:"group"`
	Active bool     `json:"active"`
	Points uint16   `json:"points"`
	Spells []uint32 `json:"spells"`
}

type glyphInsight struct {
	Group uint8  `json:"group"`
	Slot  uint8  `json:"slot"`
	ID    uint32 `json:"id"`
}

type achievementInsight struct {
	ID   uint32 `json:"id"`
	Date uint64 `json:"date"`
}

func (s *Server) armoryInsights(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if s.c.MockMode {
		found := false
		for _, character := range mockCharacters {
			found = found || strings.EqualFold(character.Name, name)
		}
		if !found {
			problem(w, http.StatusNotFound, "Character not found")
			return
		}
		now := time.Now()
		jsonOut(w, http.StatusOK, map[string]any{
			"talents":          []talentInsight{{Group: 0, Active: true, Points: 71, Spells: []uint32{20375, 35395, 53385}}, {Group: 1, Points: 71, Spells: []uint32{53563, 20216, 20473}}},
			"glyphs":           []glyphInsight{{0, 0, 54923}, {0, 1, 54922}, {0, 2, 54928}, {0, 3, 43340}, {0, 4, 43367}, {0, 5, 43365}},
			"achievements":     []achievementInsight{{4530, uint64(now.Add(-14 * 24 * time.Hour).Unix())}, {4584, uint64(now.Add(-21 * 24 * time.Hour).Unix())}, {2889, uint64(now.Add(-60 * 24 * time.Hour).Unix())}},
			"pvpMatches":       []map[string]any{{"result": "win", "bracket": 2, "opponent": "Frozen Resolve", "ratingChange": 14, "playedAt": now.Add(-2 * time.Hour)}, {"result": "loss", "bracket": 2, "opponent": "No Trinket Needed", "ratingChange": -11, "playedAt": now.Add(-5 * time.Hour)}},
			"pvpHistorySource": "Demo match archive; stock AzerothCore only retains aggregate arena records.",
			"raidComposition":  []map[string]any{{"name": "Arthoria", "class": 2, "level": 80, "online": false}, {"name": "Thornhoof", "class": 11, "level": 80, "online": false}, {"name": "Ironward", "class": 1, "level": 80, "online": false}},
			"guildActivity":    []map[string]any{{"character": "Arthoria", "achievement": 4530, "date": uint64(now.Add(-14 * 24 * time.Hour).Unix())}, {"character": "Thornhoof", "achievement": 2889, "date": uint64(now.Add(-18 * 24 * time.Hour).Unix())}},
		})
		return
	}
	var guid, guildID uint32
	var activeGroup uint8
	query := fmt.Sprintf("SELECT c.guid,c.activeTalentGroup,COALESCE(gm.guildid,0) FROM `%s`.characters c LEFT JOIN `%s`.guild_member gm ON gm.guid=c.guid WHERE c.name=? AND c.deleteDate IS NULL", s.c.CharactersDB, s.c.CharactersDB)
	if s.s.Characters.QueryRowContext(r.Context(), query, name).Scan(&guid, &activeGroup, &guildID) != nil {
		problem(w, http.StatusNotFound, "Character not found")
		return
	}
	talentsByGroup := map[uint8]*talentInsight{}
	query = fmt.Sprintf("SELECT talentGroup,spell FROM `%s`.character_talent WHERE guid=? ORDER BY talentGroup,spell", s.c.CharactersDB)
	if rows, err := s.s.Characters.QueryContext(r.Context(), query, guid); err == nil {
		for rows.Next() {
			var group uint8
			var spell uint32
			if rows.Scan(&group, &spell) == nil {
				if talentsByGroup[group] == nil {
					talentsByGroup[group] = &talentInsight{Group: group, Active: group == activeGroup, Spells: []uint32{}}
				}
				talentsByGroup[group].Spells = append(talentsByGroup[group].Spells, spell)
				talentsByGroup[group].Points++
			}
		}
		rows.Close()
	}
	talents := []talentInsight{}
	for group := uint8(0); group < 2; group++ {
		if talent := talentsByGroup[group]; talent != nil {
			talents = append(talents, *talent)
		}
	}
	glyphs := []glyphInsight{}
	query = fmt.Sprintf("SELECT talentGroup,glyph1,glyph2,glyph3,glyph4,glyph5,glyph6 FROM `%s`.character_glyphs WHERE guid=?", s.c.CharactersDB)
	if rows, err := s.s.Characters.QueryContext(r.Context(), query, guid); err == nil {
		for rows.Next() {
			var group uint8
			var values [6]uint32
			if rows.Scan(&group, &values[0], &values[1], &values[2], &values[3], &values[4], &values[5]) == nil {
				for slot, id := range values {
					if id > 0 {
						glyphs = append(glyphs, glyphInsight{Group: group, Slot: uint8(slot), ID: id})
					}
				}
			}
		}
		rows.Close()
	}
	achievements := []achievementInsight{}
	query = fmt.Sprintf("SELECT achievement,date FROM `%s`.character_achievement WHERE guid=? ORDER BY date DESC LIMIT 100", s.c.CharactersDB)
	if rows, err := s.s.Characters.QueryContext(r.Context(), query, guid); err == nil {
		for rows.Next() {
			var item achievementInsight
			if rows.Scan(&item.ID, &item.Date) == nil {
				achievements = append(achievements, item)
			}
		}
		rows.Close()
	}
	composition := []map[string]any{}
	activity := []map[string]any{}
	if guildID > 0 {
		query = fmt.Sprintf("SELECT c.name,c.class,c.level,c.online FROM `%s`.guild_member gm JOIN `%s`.characters c ON c.guid=gm.guid WHERE gm.guildid=? AND c.deleteDate IS NULL ORDER BY c.level DESC,c.name LIMIT 40", s.c.CharactersDB, s.c.CharactersDB)
		if rows, err := s.s.Characters.QueryContext(r.Context(), query, guildID); err == nil {
			for rows.Next() {
				var member struct {
					Name         string
					Class, Level uint8
					Online       bool
				}
				if rows.Scan(&member.Name, &member.Class, &member.Level, &member.Online) == nil {
					composition = append(composition, map[string]any{"name": member.Name, "class": member.Class, "level": member.Level, "online": member.Online})
				}
			}
			rows.Close()
		}
		query = fmt.Sprintf("SELECT c.name,ca.achievement,ca.date FROM `%s`.character_achievement ca JOIN `%s`.guild_member gm ON gm.guid=ca.guid JOIN `%s`.characters c ON c.guid=ca.guid WHERE gm.guildid=? ORDER BY ca.date DESC LIMIT 30", s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
		if rows, err := s.s.Characters.QueryContext(r.Context(), query, guildID); err == nil {
			for rows.Next() {
				var character string
				var achievement uint32
				var date uint64
				if rows.Scan(&character, &achievement, &date) == nil {
					activity = append(activity, map[string]any{"character": character, "achievement": achievement, "date": date})
				}
			}
			rows.Close()
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"talents": talents, "glyphs": glyphs, "achievements": achievements, "pvpMatches": []any{}, "pvpHistorySource": "Stock AzerothCore does not retain per-match arena history.", "raidComposition": composition, "guildActivity": activity})
}

func (s *Server) raidRankings(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		now := time.Now()
		jsonOut(w, http.StatusOK, map[string]any{
			"speed":   []map[string]any{{"rank": 1, "guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "difficulty": "25 player", "seconds": 4762}, {"rank": 2, "guild": "Ashen Vanguard", "raid": "Icecrown Citadel", "difficulty": "25 player", "seconds": 5231}},
			"recent":  []map[string]any{{"guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "boss": "The Lich King", "difficulty": "25 player", "killedAt": now.Add(-6 * time.Hour)}, {"guild": "Silver Covenant", "raid": "Ulduar", "boss": "Yogg-Saron", "difficulty": "10 player", "killedAt": now.Add(-19 * time.Hour)}},
			"seasons": []map[string]any{{"name": "Season 8", "active": true}, {"name": "Season 7", "active": false}, {"name": "Season 6", "active": false}},
		})
		return
	}
	type kill struct {
		Guild, Raid, Boss, Difficulty string
		Seconds                       uint32
		KilledAt                      time.Time
	}
	load := func(order string, limit int) []kill {
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT guild_name,raid,boss,difficulty,duration_seconds,killed_at FROM portal_raid_kills WHERE realm_key=? ORDER BY "+order+" LIMIT ?", s.c.RealmKey, limit)
		if err != nil {
			return []kill{}
		}
		defer rows.Close()
		out := []kill{}
		for rows.Next() {
			var item kill
			if rows.Scan(&item.Guild, &item.Raid, &item.Boss, &item.Difficulty, &item.Seconds, &item.KilledAt) == nil {
				out = append(out, item)
			}
		}
		return out
	}
	jsonOut(w, http.StatusOK, map[string]any{"speed": load("duration_seconds>0 DESC,duration_seconds ASC", 50), "recent": load("killed_at DESC", 50), "seasons": []map[string]any{{"name": "Current season", "active": true}}})
}
