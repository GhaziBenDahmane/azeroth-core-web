package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type talentInsight struct {
	Group       uint8          `json:"group"`
	Active      bool           `json:"active"`
	Points      uint16         `json:"points"`
	PointsKnown bool           `json:"pointsKnown"`
	Spells      []spellInsight `json:"spells"`
	Trees       []talentTree   `json:"trees,omitempty"`
}

type spellInsight struct {
	ID          uint32 `json:"id"`
	Name        string `json:"name,omitempty"`
	RankName    string `json:"rankName,omitempty"`
	Description string `json:"description,omitempty"`
	IconID      uint32 `json:"iconId,omitempty"`
	Rank        uint8  `json:"rank,omitempty"`
	Tier        uint8  `json:"tier,omitempty"`
	Column      uint8  `json:"column,omitempty"`
	TreeID      uint32 `json:"treeId,omitempty"`
	TreeName    string `json:"treeName,omitempty"`
}

type talentTree struct {
	ID     uint32 `json:"id"`
	Name   string `json:"name"`
	Points uint16 `json:"points"`
}

type glyphInsight struct {
	Group       uint8  `json:"group"`
	Slot        uint8  `json:"slot"`
	ID          uint32 `json:"id"`
	SpellID     uint32 `json:"spellId,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	IconID      uint32 `json:"iconId,omitempty"`
}

type achievementInsight struct {
	ID                 uint32                 `json:"id"`
	Date               uint64                 `json:"date"`
	Name               string                 `json:"name,omitempty"`
	Description        string                 `json:"description,omitempty"`
	Category           uint32                 `json:"category,omitempty"`
	CategoryName       string                 `json:"categoryName,omitempty"`
	ParentCategory     uint32                 `json:"parentCategory,omitempty"`
	ParentCategoryName string                 `json:"parentCategoryName,omitempty"`
	Points             uint16                 `json:"points,omitempty"`
	IconID             uint32                 `json:"iconId,omitempty"`
	Criteria           []achievementCriterion `json:"criteria,omitempty"`
}

type achievementCriterion struct {
	ID          uint32 `json:"id"`
	Description string `json:"description,omitempty"`
	Counter     uint64 `json:"counter"`
	Required    uint64 `json:"required"`
	Complete    bool   `json:"complete"`
}

func (s *Server) armoryInsights(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if s.c.MockMode {
		found := false
		var guid uint32
		for _, character := range mockCharacters {
			if strings.EqualFold(character.Name, name) {
				found, guid = true, character.GUID
			}
		}
		if !found {
			problem(w, http.StatusNotFound, "Character not found")
			return
		}
		privacy, visible := s.armoryPrivacy(r, guid, 1)
		if !visible {
			problem(w, http.StatusNotFound, "Character not found")
			return
		}
		if !privacy.ShowActivity {
			jsonOut(w, http.StatusOK, map[string]any{"talents": []any{}, "glyphs": []any{}, "achievements": []any{}, "pvpMatches": []any{}, "raidComposition": []any{}, "guildActivity": []any{}, "collections": characterCollections{}, "privacyLimited": true})
			return
		}
		now := time.Now()
		jsonOut(w, http.StatusOK, map[string]any{
			"talents": []talentInsight{
				{Group: 0, Active: true, Points: 71, PointsKnown: true, Trees: []talentTree{{ID: 383, Name: "Retribution", Points: 71}}, Spells: []spellInsight{{ID: 20375, Name: "Seal of Command", Rank: 1, TreeID: 383, TreeName: "Retribution"}, {ID: 35395, Name: "Crusader Strike", Rank: 1, TreeID: 383, TreeName: "Retribution"}, {ID: 53385, Name: "Divine Storm", Rank: 1, TreeID: 383, TreeName: "Retribution"}}},
				{Group: 1, Points: 71, PointsKnown: true, Trees: []talentTree{{ID: 382, Name: "Holy", Points: 71}}, Spells: []spellInsight{{ID: 53563, Name: "Beacon of Light", Rank: 1, TreeID: 382, TreeName: "Holy"}, {ID: 20216, Name: "Divine Favor", Rank: 1, TreeID: 382, TreeName: "Holy"}, {ID: 20473, Name: "Holy Shock", Rank: 1, TreeID: 382, TreeName: "Holy"}}},
			},
			"glyphs":              []glyphInsight{{Group: 0, Slot: 0, ID: 54923, SpellID: 54923, Name: "Glyph of Seal of Vengeance"}, {Group: 0, Slot: 1, ID: 54922, SpellID: 54922, Name: "Glyph of Judgement"}, {Group: 0, Slot: 2, ID: 54928, SpellID: 54928, Name: "Glyph of Consecration"}, {Group: 0, Slot: 3, ID: 43340, SpellID: 43340, Name: "Glyph of Blessing of Might"}, {Group: 0, Slot: 4, ID: 43367, SpellID: 43367, Name: "Glyph of Lay on Hands"}, {Group: 0, Slot: 5, ID: 43365, SpellID: 43365, Name: "Glyph of Blessing of Kings"}},
			"achievements":        []achievementInsight{{ID: 4530, Name: "All You Can Eat", Category: 14922, CategoryName: "Icecrown Citadel", ParentCategory: 168, ParentCategoryName: "Dungeons & Raids", Points: 10, Date: uint64(now.Add(-14 * 24 * time.Hour).Unix()), Criteria: []achievementCriterion{{ID: 12345, Description: "Defeat Sindragosa without more than five stacks", Counter: 1, Required: 1, Complete: true}}}, {ID: 4584, Name: "The Light of Dawn", Category: 14922, CategoryName: "Icecrown Citadel", ParentCategory: 168, ParentCategoryName: "Dungeons & Raids", Points: 10, Date: uint64(now.Add(-21 * 24 * time.Hour).Unix())}, {ID: 2889, Name: "The Antechamber of Ulduar", Category: 14922, CategoryName: "Ulduar", ParentCategory: 168, ParentCategoryName: "Dungeons & Raids", Points: 10, Date: uint64(now.Add(-60 * 24 * time.Hour).Unix())}},
			"capabilities":        map[string]any{"dbcMetadata": true, "talentRanks": true, "glyphMetadata": true, "achievementMetadata": true, "achievementCategories": true},
			"pvpMatches":          []map[string]any{{"result": "win", "season": "Season 8", "bracket": 2, "teamId": 18, "team": "Dawnbringers", "opponentId": 27, "opponent": "Frozen Resolve", "ratingBefore": 2148, "ratingAfter": 2162, "ratingChange": 14, "durationSeconds": 184, "playedAt": now.Add(-2 * time.Hour), "source": "signed_ingest"}, {"result": "loss", "season": "Season 8", "bracket": 2, "teamId": 18, "team": "Dawnbringers", "opponentId": 31, "opponent": "No Trinket Needed", "ratingBefore": 2162, "ratingAfter": 2151, "ratingChange": -11, "durationSeconds": 263, "playedAt": now.Add(-5 * time.Hour), "source": "signed_ingest"}},
			"battlegroundMatches": []map[string]any{{"battleground": "Warsong Gulch", "team": "alliance", "winningTeam": "alliance", "result": "win", "killingBlows": 7, "honorableKills": 24, "deaths": 2, "damageDone": 384220, "healingDone": 42100, "durationSeconds": 1094, "playedAt": now.Add(-8 * time.Hour)}},
			"pvpHistorySource":    "Demo match archive; stock AzerothCore only retains aggregate arena records.",
			"raidComposition":     []map[string]any{{"name": "Arthoria", "class": 2, "level": 80, "online": false}, {"name": "Thornhoof", "class": 11, "level": 80, "online": false}, {"name": "Ironward", "class": 1, "level": 80, "online": false}},
			"guildActivity":       []map[string]any{{"character": "Arthoria", "achievement": 4530, "date": uint64(now.Add(-14 * 24 * time.Hour).Unix())}, {"character": "Thornhoof", "achievement": 2889, "date": uint64(now.Add(-18 * 24 * time.Hour).Unix())}},
			"collections":         mockCharacterCollections(),
			"professions":         mockProfessionCollections(),
		})
		return
	}
	var guid, guildID, ownerID uint32
	var activeGroup uint8
	query := fmt.Sprintf("SELECT c.guid,c.account,c.activeTalentGroup,COALESCE(gm.guildid,0) FROM `%s`.characters c LEFT JOIN `%s`.guild_member gm ON gm.guid=c.guid WHERE c.name=? AND c.deleteDate IS NULL", s.c.CharactersDB, s.c.CharactersDB)
	if s.s.Characters.QueryRowContext(r.Context(), query, name).Scan(&guid, &ownerID, &activeGroup, &guildID) != nil {
		problem(w, http.StatusNotFound, "Character not found")
		return
	}
	privacy, visible := s.armoryPrivacy(r, guid, ownerID)
	if !visible {
		problem(w, http.StatusNotFound, "Character not found")
		return
	}
	if !privacy.ShowActivity {
		jsonOut(w, http.StatusOK, map[string]any{"talents": []any{}, "glyphs": []any{}, "achievements": []any{}, "pvpMatches": []any{}, "raidComposition": []any{}, "guildActivity": []any{}, "collections": characterCollections{}, "privacyLimited": true})
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
					talentsByGroup[group] = &talentInsight{Group: group, Active: group == activeGroup, Spells: []spellInsight{}}
				}
				talentsByGroup[group].Spells = append(talentsByGroup[group].Spells, spellInsight{ID: spell})
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
	capabilities := s.enrichArmoryMetadata(r.Context(), talents, glyphs, achievements)
	capabilities["achievementCriteria"] = s.loadAchievementCriteria(r.Context(), guid, achievements)
	composition := []map[string]any{}
	activity := []map[string]any{}
	pvpMatches := []map[string]any{}
	battlegroundMatches := []map[string]any{}
	pvpSource := "Stock AzerothCore does not retain per-match arena history."
	if s.c.CompetitiveIngestSecret != "" {
		pvpSource = "Signed ingestion archive; stock AzerothCore does not retain this per-match detail."
		if rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT m.result,m.season_slug,m.bracket,m.team_id,m.team_name,m.opponent_id,m.opponent_name,m.rating_before,m.rating_after,m.rating_change,m.duration_seconds,m.played_at FROM portal_pvp_matches m JOIN portal_pvp_match_members mm ON mm.match_id=m.id WHERE m.realm_key=? AND (mm.character_guid=? OR LOWER(mm.character_name)=LOWER(?)) ORDER BY m.played_at DESC LIMIT 30`, s.c.RealmKey, guid, name); err == nil {
			for rows.Next() {
				var result, season, team, opponent string
				var bracket uint8
				var teamID, opponentID, duration uint32
				var ratingBefore, ratingAfter uint16
				var ratingChange int16
				var playedAt time.Time
				if rows.Scan(&result, &season, &bracket, &teamID, &team, &opponentID, &opponent, &ratingBefore, &ratingAfter, &ratingChange, &duration, &playedAt) == nil {
					pvpMatches = append(pvpMatches, map[string]any{"result": result, "season": season, "bracket": bracket, "teamId": teamID, "team": team, "opponentId": opponentID, "opponent": opponent, "ratingBefore": ratingBefore, "ratingAfter": ratingAfter, "ratingChange": ratingChange, "durationSeconds": duration, "playedAt": playedAt, "source": "signed_ingest"})
				}
			}
			rows.Close()
		}
		if rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT m.battleground,mm.team_name,m.winning_team,mm.killing_blows,mm.honorable_kills,mm.deaths,mm.damage_done,mm.healing_done,m.duration_seconds,m.played_at FROM portal_battleground_matches m JOIN portal_battleground_members mm ON mm.match_id=m.id WHERE m.realm_key=? AND (mm.character_guid=? OR LOWER(mm.character_name)=LOWER(?)) ORDER BY m.played_at DESC LIMIT 30`, s.c.RealmKey, guid, name); err == nil {
			for rows.Next() {
				var battleground, team, winner string
				var killingBlows, honorableKills, deaths, duration uint32
				var damage, healing uint64
				var playedAt time.Time
				if rows.Scan(&battleground, &team, &winner, &killingBlows, &honorableKills, &deaths, &damage, &healing, &duration, &playedAt) == nil {
					result := "loss"
					if team == winner {
						result = "win"
					}
					battlegroundMatches = append(battlegroundMatches, map[string]any{"battleground": battleground, "team": team, "winningTeam": winner, "result": result, "killingBlows": killingBlows, "honorableKills": honorableKills, "deaths": deaths, "damageDone": damage, "healingDone": healing, "durationSeconds": duration, "playedAt": playedAt})
				}
			}
			rows.Close()
		}
	}
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
	professions, professionMetadata := s.loadProfessionCollections(r.Context(), guid)
	capabilities["professionRecipes"] = professionMetadata
	jsonOut(w, http.StatusOK, map[string]any{"talents": talents, "glyphs": glyphs, "achievements": achievements, "capabilities": capabilities, "pvpMatches": pvpMatches, "battlegroundMatches": battlegroundMatches, "pvpHistorySource": pvpSource, "raidComposition": composition, "guildActivity": activity, "collections": s.loadCharacterCollections(r.Context(), guid), "professions": professions})
}

func (s *Server) raidRankings(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		now := time.Now()
		jsonOut(w, http.StatusOK, map[string]any{
			"speed":       []map[string]any{{"rank": 1, "guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "difficulty": "25 player", "seconds": 4762, "verifiedMembers": 25, "source": "signed_ingest"}, {"rank": 2, "guild": "Ashen Vanguard", "raid": "Icecrown Citadel", "difficulty": "25 player", "seconds": 5231, "verifiedMembers": 24, "source": "signed_ingest"}},
			"recent":      []map[string]any{{"guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "boss": "The Lich King", "difficulty": "25 player", "killedAt": now.Add(-6 * time.Hour), "verifiedMembers": 25, "source": "signed_ingest"}, {"guild": "Silver Covenant", "raid": "Ulduar", "boss": "Yogg-Saron", "difficulty": "10 player", "killedAt": now.Add(-19 * time.Hour), "verifiedMembers": 10, "source": "signed_ingest"}},
			"attempts":    []map[string]any{{"guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "boss": "The Lich King", "difficulty": "25 player", "result": "kill", "attemptNumber": 18, "seconds": 812, "bossHealthPercent": 0, "occurredAt": now.Add(-6 * time.Hour), "verifiedMembers": 25, "source": "signed_ingest", "roles": map[string]int{"tank": 2, "healer": 5, "damage": 18}, "classes": map[string]int{"1": 3, "2": 3, "3": 2, "4": 3, "5": 3, "6": 2, "7": 2, "8": 2, "9": 2, "11": 3}}, {"guild": "Keepers of Dawn", "raid": "Icecrown Citadel", "boss": "The Lich King", "difficulty": "25 player", "result": "wipe", "attemptNumber": 17, "seconds": 694, "bossHealthPercent": 8.4, "occurredAt": now.Add(-7 * time.Hour), "verifiedMembers": 25, "source": "signed_ingest", "roles": map[string]int{"tank": 2, "healer": 5, "damage": 18}, "classes": map[string]int{"1": 3, "2": 3, "3": 2, "4": 3, "5": 3, "6": 2, "7": 2, "8": 2, "9": 2, "11": 3}}},
			"seasons":     []map[string]any{{"name": "Season 8", "active": true}, {"name": "Season 7", "active": false}, {"name": "Season 6", "active": false}},
			"eligibility": defaultRaidEligibilityRules(), "source": "Signed competitive ingestion; only eligible events are public",
		})
		return
	}
	type kill struct {
		Rank            uint32    `json:"rank,omitempty"`
		Guild           string    `json:"guild"`
		Raid            string    `json:"raid"`
		Boss            string    `json:"boss"`
		Difficulty      string    `json:"difficulty"`
		Seconds         uint32    `json:"seconds"`
		KilledAt        time.Time `json:"killedAt"`
		VerifiedMembers uint16    `json:"verifiedMembers"`
		Source          string    `json:"source"`
	}
	load := func(order string, limit int) []kill {
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT guild_name,raid,boss,difficulty,duration_seconds,killed_at,verified_members,source_kind FROM portal_raid_kills WHERE realm_key=? AND eligible=1 ORDER BY "+order+" LIMIT ?", s.c.RealmKey, limit)
		if err != nil {
			return []kill{}
		}
		defer rows.Close()
		out := []kill{}
		for rows.Next() {
			var item kill
			if rows.Scan(&item.Guild, &item.Raid, &item.Boss, &item.Difficulty, &item.Seconds, &item.KilledAt, &item.VerifiedMembers, &item.Source) == nil {
				item.Rank = uint32(len(out) + 1)
				out = append(out, item)
			}
		}
		return out
	}
	type attempt struct {
		ID                uint64         `json:"-"`
		Guild             string         `json:"guild"`
		Raid              string         `json:"raid"`
		Boss              string         `json:"boss"`
		Difficulty        string         `json:"difficulty"`
		Result            string         `json:"result"`
		AttemptNumber     uint32         `json:"attemptNumber"`
		Seconds           uint32         `json:"seconds"`
		BossHealthPercent float64        `json:"bossHealthPercent"`
		OccurredAt        time.Time      `json:"occurredAt"`
		VerifiedMembers   uint16         `json:"verifiedMembers"`
		Source            string         `json:"source"`
		Roles             map[string]int `json:"roles"`
		Classes           map[string]int `json:"classes"`
	}
	attempts := []attempt{}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,guild_name,raid,boss,difficulty,result,attempt_number,duration_seconds,boss_health_pct,occurred_at,verified_members,source_kind FROM portal_raid_attempts WHERE realm_key=? ORDER BY occurred_at DESC LIMIT 50`, s.c.RealmKey)
	if err == nil {
		for rows.Next() {
			var item attempt
			item.Roles, item.Classes = map[string]int{}, map[string]int{}
			if rows.Scan(&item.ID, &item.Guild, &item.Raid, &item.Boss, &item.Difficulty, &item.Result, &item.AttemptNumber, &item.Seconds, &item.BossHealthPercent, &item.OccurredAt, &item.VerifiedMembers, &item.Source) == nil {
				attempts = append(attempts, item)
			}
		}
		rows.Close()
	}
	if len(attempts) > 0 {
		placeholders, args := make([]string, len(attempts)), make([]any, len(attempts))
		byID := make(map[uint64]*attempt, len(attempts))
		for index := range attempts {
			placeholders[index], args[index], byID[attempts[index].ID] = "?", attempts[index].ID, &attempts[index]
		}
		memberRows, memberErr := s.s.Auth.QueryContext(r.Context(), `SELECT attempt_id,class_id,role_name FROM portal_raid_attempt_members WHERE attempt_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
		if memberErr == nil {
			for memberRows.Next() {
				var attemptID uint64
				var classID uint8
				var role string
				if memberRows.Scan(&attemptID, &classID, &role) == nil && byID[attemptID] != nil {
					byID[attemptID].Classes[fmt.Sprint(classID)]++
					if role == "dps" {
						role = "damage"
					}
					if role != "" {
						byID[attemptID].Roles[role]++
					}
				}
			}
			memberRows.Close()
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"speed": load("duration_seconds ASC", 50), "recent": load("killed_at DESC", 50), "attempts": attempts, "seasons": []map[string]any{{"name": "Current season", "active": true}}, "eligibility": s.loadRaidEligibilityRules(r), "source": "Signed competitive ingestion; only eligible events are public"})
}
