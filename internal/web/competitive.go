package web

import (
	"fmt"
	"net/http"
	"strconv"
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

func (s *Server) arenaRankings(w http.ResponseWriter, r *http.Request) {
	bracket, _ := strconv.Atoi(r.URL.Query().Get("bracket"))
	if bracket != 2 && bracket != 3 && bracket != 5 {
		bracket = 2
	}
	q := fmt.Sprintf(`SELECT arenaTeamId,name,type,rating,seasonGames,seasonWins FROM %s.arena_team WHERE type=? AND seasonGames>0 ORDER BY rating DESC,seasonWins DESC LIMIT 50`, s.c.CharactersDB)
	rows, err := s.s.Characters.QueryContext(r.Context(), q, bracket)
	if err != nil {
		problem(w, 500, "Could not load arena rankings")
		return
	}
	defer rows.Close()
	teams := []arenaTeam{}
	rank := uint32(0)
	for rows.Next() {
		var t arenaTeam
		rank++
		t.Rank = rank
		if rows.Scan(&t.ID, &t.Name, &t.Bracket, &t.Rating, &t.SeasonGames, &t.SeasonWins) != nil {
			continue
		}
		mq := fmt.Sprintf(`SELECT c.name,c.class,m.personalRating,m.seasonGames,m.seasonWins FROM %s.arena_team_member m JOIN %s.characters c ON c.guid=m.guid WHERE m.arenaTeamId=? ORDER BY m.personalRating DESC`, s.c.CharactersDB, s.c.CharactersDB)
		mr, e := s.s.Characters.QueryContext(r.Context(), mq, t.ID)
		t.Members = []arenaMember{}
		if e == nil {
			for mr.Next() {
				var member arenaMember
				if mr.Scan(&member.Name, &member.Class, &member.PersonalRating, &member.SeasonGames, &member.SeasonWins) == nil {
					t.Members = append(t.Members, member)
				}
			}
			mr.Close()
		}
		teams = append(teams, t)
	}
	jsonOut(w, 200, map[string]any{"bracket": bracket, "teams": teams})
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
