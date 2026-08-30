package web

import (
	"fmt"
	"net/http"
	"strconv"
)

type characterPrivacy struct {
	Hidden       bool `json:"hidden"`
	ShowGear     bool `json:"showGear"`
	ShowActivity bool `json:"showActivity"`
}

func defaultCharacterPrivacy() characterPrivacy {
	return characterPrivacy{ShowGear: true, ShowActivity: true}
}

func (s *Server) characterPrivacySettings(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	guidValue, err := strconv.ParseUint(r.PathValue("guid"), 10, 32)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid character")
		return
	}
	guid := uint32(guidValue)
	if s.c.MockMode {
		owned := false
		for _, character := range mockCharacters {
			if character.GUID == guid {
				owned = true
				break
			}
		}
		if !owned {
			problem(w, http.StatusNotFound, "Character not found")
			return
		}
		if r.Method == http.MethodGet {
			s.mock.mu.Lock()
			item, ok := s.mock.characterPrivacy[guid]
			s.mock.mu.Unlock()
			if !ok {
				item = defaultCharacterPrivacy()
			}
			jsonOut(w, http.StatusOK, map[string]any{"privacy": item})
			return
		}
		var item characterPrivacy
		if !decode(w, r, &item) {
			return
		}
		s.mock.mu.Lock()
		s.mock.characterPrivacy[guid] = item
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var owner uint32
	if s.s.Characters.QueryRowContext(r.Context(), fmt.Sprintf(`SELECT account FROM %s.characters WHERE guid=? AND deleteDate IS NULL`, s.c.CharactersDB), guid).Scan(&owner) != nil || owner != a.ID {
		problem(w, http.StatusNotFound, "Character not found")
		return
	}
	if r.Method == http.MethodGet {
		item := defaultCharacterPrivacy()
		_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT hidden,show_gear,show_activity FROM portal_character_privacy WHERE realm_key=? AND character_guid=? AND account_id=?`, s.c.RealmKey, guid, a.ID).Scan(&item.Hidden, &item.ShowGear, &item.ShowActivity)
		jsonOut(w, http.StatusOK, map[string]any{"privacy": item})
		return
	}
	var item characterPrivacy
	if !decode(w, r, &item) {
		return
	}
	_, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_character_privacy(account_id,realm_key,character_guid,hidden,show_gear,show_activity) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE account_id=VALUES(account_id),hidden=VALUES(hidden),show_gear=VALUES(show_gear),show_activity=VALUES(show_activity)`, a.ID, s.c.RealmKey, guid, item.Hidden, item.ShowGear, item.ShowActivity)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not save character privacy")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) hiddenCharacterGUIDs(r *http.Request) map[uint32]bool {
	hidden := map[uint32]bool{}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for guid, item := range s.mock.characterPrivacy {
			if item.Hidden {
				hidden[guid] = true
			}
		}
		return hidden
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT character_guid FROM portal_character_privacy WHERE realm_key=? AND hidden=1`, s.c.RealmKey)
	if err != nil {
		return hidden
	}
	defer rows.Close()
	for rows.Next() {
		var guid uint32
		if rows.Scan(&guid) == nil {
			hidden[guid] = true
		}
	}
	return hidden
}

func (s *Server) armoryPrivacy(r *http.Request, guid, owner uint32) (characterPrivacy, bool) {
	item := defaultCharacterPrivacy()
	if s.c.MockMode {
		s.mock.mu.Lock()
		stored, ok := s.mock.characterPrivacy[guid]
		s.mock.mu.Unlock()
		if ok {
			item = stored
		}
	} else {
		_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT hidden,show_gear,show_activity FROM portal_character_privacy WHERE realm_key=? AND character_guid=?`, s.c.RealmKey, guid).Scan(&item.Hidden, &item.ShowGear, &item.ShowActivity)
	}
	owned := false
	if s.c.MockMode {
		_, owned = s.mockUser(r)
	} else {
		viewer, err := s.auth(r)
		owned = err == nil && viewer.ID == owner
	}
	return item, !item.Hidden || owned
}
