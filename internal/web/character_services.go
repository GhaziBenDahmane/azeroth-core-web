package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type deletedCharacter struct {
	GUID      uint32 `json:"guid"`
	Name      string `json:"name"`
	DeletedAt uint64 `json:"deletedAt"`
}

func (s *Server) deletedCharacters(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, 401, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		out := append([]deletedCharacter(nil), s.mock.deleted...)
		s.mock.mu.Unlock()
		jsonOut(w, 200, map[string]any{"characters": out})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	q := fmt.Sprintf("SELECT guid,deleteInfos_Name,deleteDate FROM `%s`.characters WHERE deleteDate IS NOT NULL AND deleteInfos_Account=? ORDER BY deleteDate DESC LIMIT 50", s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), q, a.ID)
	if e != nil {
		problem(w, 500, "Could not load deleted characters")
		return
	}
	defer rows.Close()
	out := []deletedCharacter{}
	for rows.Next() {
		var c deletedCharacter
		if rows.Scan(&c.GUID, &c.Name, &c.DeletedAt) == nil {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}

func (s *Server) characterService(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		s.mockCharacterService(w, r)
		return
	}
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	guid64, e := strconv.ParseUint(r.PathValue("guid"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid character")
		return
	}
	guid := uint32(guid64)
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	var name string
	var online bool
	var command string
	if in.Action == "restore" {
		q := fmt.Sprintf("SELECT deleteInfos_Name FROM `%s`.characters WHERE guid=? AND deleteDate IS NOT NULL AND deleteInfos_Account=?", s.c.CharactersDB)
		if s.s.Characters.QueryRowContext(r.Context(), q, guid, a.ID).Scan(&name) != nil {
			problem(w, 404, "Deleted character not found")
			return
		}
		command = fmt.Sprintf("character deleted restore %d", guid)
	} else {
		q := fmt.Sprintf("SELECT name,online FROM `%s`.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
		if s.s.Characters.QueryRowContext(r.Context(), q, guid, a.ID).Scan(&name, &online) != nil {
			problem(w, 404, "Character not found")
			return
		}
		if online {
			problem(w, 409, "Character must be offline")
			return
		}
		if !validCharacterName(name) {
			problem(w, 422, "Character name is not safe for service execution")
			return
		}
		switch in.Action {
		case "rename":
			command = "character rename " + name
		case "customize":
			command = "character customize " + name
		case "unstuck":
			command = "unstuck " + name
		default:
			problem(w, 422, "Action must be rename, customize, unstuck, or restore")
			return
		}
	}
	if !s.soap.Enabled() {
		problem(w, 503, "Character services require AzerothCore SOAP")
		return
	}
	response, cmdErr := s.soap.Command(r.Context(), command)
	success := cmdErr == nil
	if cmdErr != nil {
		response = cmdErr.Error()
	}
	response = strings.Join(strings.Fields(response), " ")
	if len(response) > 500 {
		response = response[:500]
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_character_services(account_id,character_guid,realm_key,action,character_name,success,response) VALUES(?,?,?,?,?,?,?)", a.ID, guid, s.c.RealmKey, in.Action, name, success, response)
	if cmdErr != nil {
		problem(w, 502, "AzerothCore rejected the character service")
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "action": in.Action, "character": name})
}

func (s *Server) mockDeletedCharacters(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	s.mock.mu.Lock()
	out := append([]deletedCharacter(nil), s.mock.deleted...)
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"characters": out})
}

func (s *Server) mockCharacterService(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	guid, e := strconv.ParseUint(r.PathValue("guid"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid character")
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	valid := map[string]bool{"rename": true, "customize": true, "unstuck": true, "restore": true}
	if !valid[in.Action] {
		problem(w, 422, "Invalid character service")
		return
	}
	s.mock.mu.Lock()
	if in.Action == "restore" {
		found := false
		for i, c := range s.mock.deleted {
			if uint64(c.GUID) == guid {
				found = true
				s.mock.deleted = append(s.mock.deleted[:i], s.mock.deleted[i+1:]...)
				break
			}
		}
		if !found {
			s.mock.mu.Unlock()
			problem(w, 404, "Deleted character not found")
			return
		}
	} else {
		found := false
		for _, c := range mockCharacters {
			if uint64(c.GUID) == guid {
				found = true
				if c.Online {
					s.mock.mu.Unlock()
					problem(w, 409, "Character must be offline")
					return
				}
				break
			}
		}
		if !found {
			s.mock.mu.Unlock()
			problem(w, 404, "Character not found")
			return
		}
	}
	s.mock.mu.Unlock()
	jsonOut(w, 200, map[string]any{"ok": true, "action": in.Action, "message": "Demo service applied"})
}
