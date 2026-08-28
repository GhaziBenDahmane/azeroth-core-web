package web

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) realmOverview(w http.ResponseWriter, r *http.Request) {
	type realm struct {
		Name, Address string
		Port          uint16
		Population    float32
	}
	var info realm
	q := fmt.Sprintf("SELECT name,address,port,population FROM `%s`.realmlist WHERE id=?", s.c.AuthDB)
	_ = s.s.Auth.QueryRowContext(r.Context(), q, s.c.RealmID).Scan(&info.Name, &info.Address, &info.Port, &info.Population)
	var online, total, alliance, horde uint32
	cq := fmt.Sprintf(`SELECT COUNT(*),COALESCE(SUM(online),0),COALESCE(SUM(CASE WHEN race IN (1,3,4,7,11) THEN online ELSE 0 END),0),COALESCE(SUM(CASE WHEN race IN (2,5,6,8,10) THEN online ELSE 0 END),0) FROM %s.characters WHERE deleteDate IS NULL`, s.c.CharactersDB)
	_ = s.s.Characters.QueryRowContext(r.Context(), cq).Scan(&total, &online, &alliance, &horde)
	var uptime, maxPlayers uint64
	uq := fmt.Sprintf("SELECT uptime,maxplayers FROM `%s`.uptime WHERE realmid=? ORDER BY starttime DESC LIMIT 1", s.c.AuthDB)
	_ = s.s.Auth.QueryRowContext(r.Context(), uq, s.c.RealmID).Scan(&uptime, &maxPlayers)
	jsonOut(w, 200, map[string]any{"name": info.Name, "address": info.Address, "port": info.Port, "population": info.Population, "characters": total, "online": online, "allianceOnline": alliance, "hordeOnline": horde, "uptime": uptime, "recordOnline": maxPlayers})
}

func (s *Server) guildList(w http.ResponseWriter, r *http.Request) {
	q := fmt.Sprintf(`SELECT g.guildid,g.name,c.name,COUNT(gm.guid),COALESCE(AVG(m.level),0),COALESCE(SUM(m.online),0) FROM %s.guild g JOIN %s.characters c ON c.guid=g.leaderguid LEFT JOIN %s.guild_member gm ON gm.guildid=g.guildid LEFT JOIN %s.characters m ON m.guid=gm.guid GROUP BY g.guildid,g.name,c.name ORDER BY COUNT(gm.guid) DESC,g.name LIMIT 100`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), q)
	if e != nil {
		problem(w, 500, "Could not load guilds")
		return
	}
	defer rows.Close()
	type guild struct {
		ID           uint32 `json:"id"`
		Name, Leader string
		Members      uint32
		AverageLevel float64
		Online       uint32
	}
	out := []guild{}
	for rows.Next() {
		var g guild
		if rows.Scan(&g.ID, &g.Name, &g.Leader, &g.Members, &g.AverageLevel, &g.Online) == nil {
			out = append(out, g)
		}
	}
	jsonOut(w, 200, map[string]any{"guilds": out})
}

func (s *Server) guildDetail(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid guild")
		return
	}
	var name, leader, motd, info string
	q := fmt.Sprintf(`SELECT g.name,c.name,g.motd,g.info FROM %s.guild g JOIN %s.characters c ON c.guid=g.leaderguid WHERE g.guildid=?`, s.c.CharactersDB, s.c.CharactersDB)
	if s.s.Characters.QueryRowContext(r.Context(), q, id).Scan(&name, &leader, &motd, &info) != nil {
		problem(w, 404, "Guild not found")
		return
	}
	mq := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,gr.rname FROM %s.guild_member gm JOIN %s.characters c ON c.guid=gm.guid LEFT JOIN %s.guild_rank gr ON gr.guildid=gm.guildid AND gr.rid=gm.rank WHERE gm.guildid=? ORDER BY gm.rank,c.level DESC,c.name`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), mq, id)
	if e != nil {
		problem(w, 500, "Could not load guild roster")
		return
	}
	defer rows.Close()
	type member struct {
		character
		Rank string `json:"rank"`
	}
	members := []member{}
	for rows.Next() {
		var m member
		if rows.Scan(&m.GUID, &m.Name, &m.Race, &m.Class, &m.Gender, &m.Level, &m.Zone, &m.Online, &m.TotalTime, &m.Rank) == nil {
			members = append(members, m)
		}
	}
	jsonOut(w, 200, map[string]any{"guild": map[string]any{"id": id, "name": name, "leader": leader, "motd": motd, "info": info}, "members": members})
}
