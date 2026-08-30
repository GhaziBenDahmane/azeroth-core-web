package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type arenaMember struct {
	Name           string `json:"name"`
	Class          uint8  `json:"class"`
	PersonalRating uint16 `json:"personalRating"`
	SeasonGames    uint16 `json:"seasonGames"`
	SeasonWins     uint16 `json:"seasonWins"`
}
type arenaTeam struct {
	ID          uint32        `json:"id"`
	Rank        uint32        `json:"rank"`
	Name        string        `json:"name"`
	Bracket     uint8         `json:"bracket"`
	Rating      uint16        `json:"rating"`
	SeasonGames uint16        `json:"seasonGames"`
	SeasonWins  uint16        `json:"seasonWins"`
	Members     []arenaMember `json:"members"`
}

type characterArenaTeam struct {
	ID             uint32 `json:"id"`
	Rank           uint32 `json:"rank"`
	Name           string `json:"name"`
	Bracket        uint8  `json:"bracket"`
	Rating         uint16 `json:"rating"`
	SeasonGames    uint16 `json:"seasonGames"`
	SeasonWins     uint16 `json:"seasonWins"`
	PersonalRating uint16 `json:"personalRating"`
	PersonalGames  uint16 `json:"personalGames"`
	PersonalWins   uint16 `json:"personalWins"`
}

func (s *Server) characterArenaTeams(r *http.Request, guid uint32) []characterArenaTeam {
	q := fmt.Sprintf(`SELECT t.arenaTeamId,t.name,t.type,t.rating,t.seasonGames,t.seasonWins,m.personalRating,m.seasonGames,m.seasonWins,(SELECT COUNT(*)+1 FROM %s.arena_team ranked WHERE ranked.type=t.type AND (ranked.rating>t.rating OR (ranked.rating=t.rating AND ranked.arenaTeamId<t.arenaTeamId))) FROM %s.arena_team_member m JOIN %s.arena_team t ON t.arenaTeamId=m.arenaTeamId WHERE m.guid=? ORDER BY t.type`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, err := s.s.Characters.QueryContext(r.Context(), q, guid)
	if err != nil {
		return []characterArenaTeam{}
	}
	defer rows.Close()
	out := []characterArenaTeam{}
	for rows.Next() {
		var x characterArenaTeam
		if rows.Scan(&x.ID, &x.Name, &x.Bracket, &x.Rating, &x.SeasonGames, &x.SeasonWins, &x.PersonalRating, &x.PersonalGames, &x.PersonalWins, &x.Rank) == nil {
			out = append(out, x)
		}
	}
	return out
}

func (s *Server) arenaRankings(w http.ResponseWriter, r *http.Request) {
	bracket, _ := strconv.Atoi(r.URL.Query().Get("bracket"))
	if bracket != 2 && bracket != 3 && bracket != 5 {
		bracket = 2
	}
	page := rankingPage(r)
	season := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("season")))
	if season != "" && season != "current" {
		s.archivedArenaRankings(w, r, season, bracket, page)
		return
	}
	const pageSize = 25
	offset := (page - 1) * pageSize
	where, args := "type=? AND seasonGames>0", []any{bracket}
	if excluded := s.activeRankingExclusions(r, "arena_team"); len(excluded) > 0 {
		where += " AND LOWER(name) NOT IN (" + placeholders(len(excluded)) + ")"
		for _, name := range excluded {
			args = append(args, name)
		}
	}
	args = append(args, pageSize+1, offset)
	q := fmt.Sprintf(`SELECT arenaTeamId,name,type,rating,seasonGames,seasonWins FROM %s.arena_team WHERE %s ORDER BY rating DESC,seasonWins DESC LIMIT ? OFFSET ?`, s.c.CharactersDB, where)
	rows, err := s.s.Characters.QueryContext(r.Context(), q, args...)
	if err != nil {
		problem(w, 500, "Could not load arena rankings")
		return
	}
	defer rows.Close()
	teams := []arenaTeam{}
	rank := uint32(offset)
	for rows.Next() {
		var t arenaTeam
		rank++
		t.Rank = rank
		if rows.Scan(&t.ID, &t.Name, &t.Bracket, &t.Rating, &t.SeasonGames, &t.SeasonWins) != nil {
			continue
		}
		t.Members = []arenaMember{}
		teams = append(teams, t)
	}
	hasMore := len(teams) > pageSize
	if hasMore {
		teams = teams[:pageSize]
	}
	if len(teams) > 0 {
		teamIndexes, args := map[uint32]int{}, make([]any, len(teams))
		for index := range teams {
			teamIndexes[teams[index].ID], args[index] = index, teams[index].ID
		}
		mq := fmt.Sprintf(`SELECT m.arenaTeamId,c.name,c.class,m.personalRating,m.seasonGames,m.seasonWins FROM %s.arena_team_member m JOIN %s.characters c ON c.guid=m.guid WHERE m.arenaTeamId IN (%s) ORDER BY m.arenaTeamId,m.personalRating DESC`, s.c.CharactersDB, s.c.CharactersDB, placeholders(len(teams)))
		if members, queryErr := s.s.Characters.QueryContext(r.Context(), mq, args...); queryErr == nil {
			for members.Next() {
				var teamID uint32
				var member arenaMember
				if members.Scan(&teamID, &member.Name, &member.Class, &member.PersonalRating, &member.SeasonGames, &member.SeasonWins) == nil {
					if index, found := teamIndexes[teamID]; found {
						teams[index].Members = append(teams[index].Members, member)
					}
				}
			}
			members.Close()
		}
	}
	jsonOut(w, 200, map[string]any{"bracket": bracket, "teams": teams, "page": page, "hasMore": hasMore, "season": "current", "seasonName": "Current season", "source": "Live AzerothCore arena tables", "seasons": s.arenaSeasons(r)})
}

func rankingPage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		return 1
	}
	if page > 1000 {
		return 1000
	}
	return page
}

func (s *Server) rankingCapabilities(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]bool{"honorable-kills": true, "played-time": true, "level": true, "achievements": true, "exalted-reputations": true, "guild-members": true, "mounts": s.c.MockMode, "companions": s.c.MockMode}
	if !s.c.MockMode {
		var available bool
		_ = s.s.World.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=? AND table_name='spell_dbc')`, s.c.WorldDB).Scan(&available)
		metrics["mounts"], metrics["companions"] = available, available
	}
	jsonOut(w, http.StatusOK, map[string]any{"metrics": metrics, "battlegroundHistory": s.c.MockMode || s.c.CompetitiveIngestSecret != "", "arenaHistory": s.c.MockMode || s.c.CompetitiveIngestSecret != "", "raidSpeed": s.c.MockMode || s.c.CompetitiveIngestSecret != ""})
}

func (s *Server) expandedRankings(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "honorable-kills"
	}
	type row struct {
		Rank    uint32 `json:"rank"`
		Name    string `json:"name"`
		Class   uint8  `json:"class,omitempty"`
		Race    uint8  `json:"race,omitempty"`
		Spec    string `json:"spec,omitempty"`
		Faction string `json:"faction,omitempty"`
		Level   uint8  `json:"level,omitempty"`
		Value   uint64 `json:"value"`
		Online  bool   `json:"online,omitempty"`
	}
	out := []row{}
	page := rankingPage(r)
	const pageSize = 25
	offset := (page - 1) * pageSize
	var query string
	classID, _ := strconv.Atoi(r.URL.Query().Get("class"))
	if classID < 0 || classID > 11 || classID == 10 {
		problem(w, 422, "Invalid class filter")
		return
	}
	faction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("faction")))
	if faction != "" && faction != "alliance" && faction != "horde" {
		problem(w, 422, "Invalid faction filter")
		return
	}
	spec := strings.TrimSpace(r.URL.Query().Get("spec"))
	if len(spec) > 40 {
		problem(w, 422, "Invalid specialization filter")
		return
	}
	if spec != "" {
		problem(w, 422, "Specialization rankings require a configured talent metadata provider")
		return
	}
	where := "c.deleteDate IS NULL"
	args := []any{}
	if excluded := s.activeRankingExclusions(r, "character"); len(excluded) > 0 && metric != "guild-members" {
		where += " AND LOWER(c.name) NOT IN (" + placeholders(len(excluded)) + ")"
		for _, name := range excluded {
			args = append(args, name)
		}
	}
	if classID > 0 {
		where += " AND c.class=?"
		args = append(args, classID)
	}
	if faction == "alliance" {
		where += " AND c.race IN (1,3,4,7,11)"
	} else if faction == "horde" {
		where += " AND c.race IN (2,5,6,8,10)"
	}
	switch metric {
	case "honorable-kills":
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,c.totalKills,c.online FROM `%s`.characters c WHERE %s ORDER BY c.totalKills DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, where)
	case "played-time":
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,c.totaltime,c.online FROM `%s`.characters c WHERE %s ORDER BY c.totaltime DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, where)
	case "level":
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,c.level,c.online FROM `%s`.characters c WHERE %s ORDER BY c.level DESC,c.totaltime DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, where)
	case "achievements":
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,COALESCE(a.score,0),c.online FROM `%s`.characters c LEFT JOIN (SELECT guid,COUNT(*) score FROM `%s`.character_achievement GROUP BY guid) a ON a.guid=c.guid WHERE %s ORDER BY a.score DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, s.c.CharactersDB, where)
	case "exalted-reputations":
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,COALESCE(rep.score,0),c.online FROM `%s`.characters c LEFT JOIN (SELECT guid,COUNT(*) score FROM `%s`.character_reputation WHERE standing>=42000 GROUP BY guid) rep ON rep.guid=c.guid WHERE %s ORDER BY rep.score DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, s.c.CharactersDB, where)
	case "mounts", "companions":
		condition := "(sp.Effect_1=6 AND sp.EffectApplyAuraName_1=78)"
		if metric == "companions" {
			condition = "sp.Effect_1=28"
		}
		query = fmt.Sprintf("SELECT c.name,c.class,c.race,c.level,COUNT(DISTINCT sp.ID),c.online FROM `%s`.characters c LEFT JOIN `%s`.character_spell cs ON cs.guid=c.guid AND cs.active=1 AND cs.disabled=0 LEFT JOIN `%s`.spell_dbc sp ON sp.ID=cs.spell AND %s WHERE %s GROUP BY c.guid,c.name,c.class,c.race,c.level,c.online ORDER BY COUNT(DISTINCT sp.ID) DESC,c.guid LIMIT ? OFFSET ?", s.c.CharactersDB, s.c.CharactersDB, s.c.WorldDB, condition, where)
	case "guild-members":
		guildWhere, guildArgs := "1=1", []any{}
		if excluded := s.activeRankingExclusions(r, "guild"); len(excluded) > 0 {
			guildWhere += " AND LOWER(g.name) NOT IN (" + placeholders(len(excluded)) + ")"
			for _, name := range excluded {
				guildArgs = append(guildArgs, name)
			}
		}
		query = fmt.Sprintf("SELECT g.name,0,0,0,COUNT(gm.guid),0 FROM `%s`.guild g LEFT JOIN `%s`.guild_member gm ON gm.guildid=g.guildid WHERE %s GROUP BY g.guildid,g.name ORDER BY COUNT(gm.guid) DESC,g.guildid LIMIT ? OFFSET ?", s.c.CharactersDB, s.c.CharactersDB, guildWhere)
		args = guildArgs
	default:
		problem(w, 422, "Unknown ranking metric")
		return
	}
	args = append(args, pageSize+1, offset)
	rows, e := s.s.Characters.QueryContext(r.Context(), query, args...)
	if e != nil {
		problem(w, 500, "Could not load rankings")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var x row
		x.Rank = uint32(offset + len(out) + 1)
		if rows.Scan(&x.Name, &x.Class, &x.Race, &x.Level, &x.Value, &x.Online) == nil {
			if isAllianceRace(x.Race) {
				x.Faction = "Alliance"
			} else if isHordeRace(x.Race) {
				x.Faction = "Horde"
			}
			out = append(out, x)
		}
	}
	hasMore := len(out) > pageSize
	if hasMore {
		out = out[:pageSize]
	}
	jsonOut(w, 200, map[string]any{"metric": metric, "rows": out, "page": page, "hasMore": hasMore, "filters": map[string]any{"class": classID, "faction": faction, "spec": spec}, "source": "AzerothCore character data; specialization requires a compatible talent metadata provider"})
}

func (s *Server) mockExpandedRankings(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "honorable-kills"
	}
	valid := map[string]bool{"honorable-kills": true, "played-time": true, "level": true, "achievements": true, "exalted-reputations": true, "mounts": true, "companions": true, "guild-members": true}
	if !valid[metric] {
		problem(w, 422, "Unknown ranking metric")
		return
	}
	rows := []map[string]any{}
	if metric == "guild-members" {
		for i, g := range []struct {
			Name    string
			Members uint64
		}{{"Keepers of Dawn", 84}, {"Ashen Vanguard", 61}, {"Silver Covenant", 43}} {
			rows = append(rows, map[string]any{"rank": i + 1, "name": g.Name, "value": g.Members})
		}
		jsonOut(w, 200, map[string]any{"metric": metric, "rows": rows, "source": "Demo AzerothCore data"})
		return
	}
	classID, _ := strconv.Atoi(r.URL.Query().Get("class"))
	faction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("faction")))
	specFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("spec")))
	specs := map[string]string{"Arthoria": "Retribution", "Thornhoof": "Feral", "Velistra": "Frost", "Grimward": "Unholy", "Quickarrow": "Marksmanship", "Emberhex": "Affliction", "Ironward": "Protection", "Nightshiv": "Assassination", "Dawnprayer": "Holy", "Stormcaller": "Elemental"}
	for i, c := range mockCharacters {
		characterFaction := "Horde"
		if isAllianceRace(c.Race) {
			characterFaction = "Alliance"
		}
		spec := specs[c.Name]
		if classID > 0 && int(c.Class) != classID || faction != "" && !strings.EqualFold(faction, characterFaction) || specFilter != "" && !strings.EqualFold(specFilter, spec) {
			continue
		}
		value := uint64(12000 - i*713)
		if metric == "played-time" {
			value = uint64(c.TotalTime)
		} else if metric == "level" {
			value = uint64(c.Level)
		} else if metric == "achievements" {
			value = uint64(164 - i*9)
		} else if metric == "exalted-reputations" {
			value = uint64(18 - i)
		} else if metric == "mounts" {
			value = uint64(94 - i*4)
		} else if metric == "companions" {
			value = uint64(42 - i*2)
		}
		rows = append(rows, map[string]any{"rank": len(rows) + 1, "name": c.Name, "class": c.Class, "race": c.Race, "faction": characterFaction, "spec": spec, "level": c.Level, "value": value, "online": c.Online})
	}
	jsonOut(w, 200, map[string]any{"metric": metric, "rows": rows, "source": "Demo AzerothCore data"})
}

type progressDefinition struct {
	Achievement               uint32
	Raid, Section, Difficulty string
	Bosses                    []string
}

var progressDefinitions = []progressDefinition{
	{568, "Naxxramas", "Arachnid Quarter", "10 player", []string{"Anub'Rekhan", "Grand Widow Faerlina", "Maexxna"}},
	{569, "Naxxramas", "Military Quarter", "10 player", []string{"Instructor Razuvious", "Gothik the Harvester", "The Four Horsemen"}},
	{570, "Naxxramas", "Construct Quarter", "10 player", []string{"Patchwerk", "Grobbulus", "Gluth", "Thaddius"}},
	{571, "Naxxramas", "Plague Quarter", "10 player", []string{"Noth the Plaguebringer", "Heigan the Unclean", "Loatheb"}},
	{576, "Naxxramas", "Frostwyrm Lair", "10 player", []string{"Sapphiron", "Kel'Thuzad"}},
	{2886, "Ulduar", "The Siege", "10 player", []string{"Flame Leviathan", "Razorscale", "Ignis", "XT-002 Deconstructor"}},
	{2887, "Ulduar", "The Antechamber", "10 player", []string{"Assembly of Iron", "Kologarn", "Auriaya"}},
	{2888, "Ulduar", "The Keepers", "10 player", []string{"Hodir", "Thorim", "Freya", "Mimiron"}},
	{2889, "Ulduar", "Descent into Madness", "10 player", []string{"General Vezax", "Yogg-Saron"}},
	{3917, "Trial of the Crusader", "Call of the Crusade", "10 player", []string{"Northrend Beasts", "Lord Jaraxxus", "Faction Champions", "Twin Val'kyr", "Anub'arak"}},
	{4531, "Icecrown Citadel", "The Lower Spire", "10 player", []string{"Lord Marrowgar", "Lady Deathwhisper", "Gunship Battle", "Deathbringer Saurfang"}},
	{4528, "Icecrown Citadel", "The Plagueworks", "10 player", []string{"Festergut", "Rotface", "Professor Putricide"}},
	{4529, "Icecrown Citadel", "The Crimson Hall", "10 player", []string{"Blood Prince Council", "Blood-Queen Lana'thel"}},
	{4530, "Icecrown Citadel", "The Frostwing Halls", "10 player", []string{"Valithria Dreamwalker", "Sindragosa"}},
	{4532, "Icecrown Citadel", "The Frozen Throne", "10 player", []string{"The Lich King"}},
	{4604, "Icecrown Citadel", "The Lower Spire", "25 player", []string{"Lord Marrowgar", "Lady Deathwhisper", "Gunship Battle", "Deathbringer Saurfang"}},
	{4605, "Icecrown Citadel", "The Plagueworks", "25 player", []string{"Festergut", "Rotface", "Professor Putricide"}},
	{4606, "Icecrown Citadel", "The Crimson Hall", "25 player", []string{"Blood Prince Council", "Blood-Queen Lana'thel"}},
	{4607, "Icecrown Citadel", "The Frostwing Halls", "25 player", []string{"Valithria Dreamwalker", "Sindragosa"}},
	{4608, "Icecrown Citadel", "The Frozen Throne", "25 player", []string{"The Lich King"}},
	{4817, "Ruby Sanctum", "The Twilight Destroyer", "10 player", []string{"Halion"}},
	{4815, "Ruby Sanctum", "The Twilight Destroyer", "25 player", []string{"Halion"}},
}

func (s *Server) raidProgression(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var guid, guildID uint32
	var guildName string
	q := fmt.Sprintf(`SELECT c.guid,COALESCE(g.guildid,0),COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.name=? AND c.deleteDate IS NULL`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	if s.s.Characters.QueryRowContext(r.Context(), q, name).Scan(&guid, &guildID, &guildName) != nil {
		problem(w, 404, "Character not found")
		return
	}
	ids := make([]any, len(progressDefinitions)+1)
	placeholders := ""
	ids[0] = guid
	for i, d := range progressDefinitions {
		ids[i+1] = d.Achievement
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}
	characterDates := map[uint32]uint64{}
	cq := fmt.Sprintf(`SELECT achievement,date FROM %s.character_achievement WHERE guid=? AND achievement IN (%s)`, s.c.CharactersDB, placeholders)
	if rows, e := s.s.Characters.QueryContext(r.Context(), cq, ids...); e == nil {
		for rows.Next() {
			var id uint32
			var date uint64
			if rows.Scan(&id, &date) == nil {
				characterDates[id] = date
			}
		}
		rows.Close()
	}
	guildDates := map[uint32]uint64{}
	if guildID > 0 {
		args := make([]any, len(progressDefinitions)+1)
		args[0] = guildID
		for i, d := range progressDefinitions {
			args[i+1] = d.Achievement
		}
		gq := fmt.Sprintf(`SELECT ca.achievement,MIN(ca.date) FROM %s.character_achievement ca JOIN %s.guild_member gm ON gm.guid=ca.guid WHERE gm.guildid=? AND ca.achievement IN (%s) GROUP BY ca.achievement`, s.c.CharactersDB, s.c.CharactersDB, placeholders)
		if rows, e := s.s.Characters.QueryContext(r.Context(), gq, args...); e == nil {
			for rows.Next() {
				var id uint32
				var date uint64
				if rows.Scan(&id, &date) == nil {
					guildDates[id] = date
				}
			}
			rows.Close()
		}
	}
	type result struct {
		Achievement   uint32   `json:"achievement"`
		Raid          string   `json:"raid"`
		Section       string   `json:"section"`
		Difficulty    string   `json:"difficulty"`
		Bosses        []string `json:"bosses"`
		CharacterDate uint64   `json:"characterDate,omitempty"`
		GuildDate     uint64   `json:"guildDate,omitempty"`
	}
	out := make([]result, 0, len(progressDefinitions))
	for _, d := range progressDefinitions {
		out = append(out, result{d.Achievement, d.Raid, d.Section, d.Difficulty, d.Bosses, characterDates[d.Achievement], guildDates[d.Achievement]})
	}
	jsonOut(w, 200, map[string]any{"character": name, "guild": guildName, "progression": out, "source": "achievement timestamps"})
}
