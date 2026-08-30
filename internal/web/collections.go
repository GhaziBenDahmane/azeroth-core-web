package web

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type reputationInsight struct {
	FactionID uint32 `json:"factionId"`
	Name      string `json:"name,omitempty"`
	Standing  int32  `json:"standing"`
	Flags     uint8  `json:"flags"`
}

type titleInsight struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

type collectionSpell struct {
	ID          uint32 `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IconID      uint32 `json:"iconId,omitempty"`
}

type characterCollections struct {
	Reputations  []reputationInsight `json:"reputations"`
	Titles       []titleInsight      `json:"titles"`
	Mounts       []collectionSpell   `json:"mounts"`
	Companions   []collectionSpell   `json:"companions"`
	Capabilities map[string]bool     `json:"capabilities"`
}

func mockCharacterCollections() characterCollections {
	return characterCollections{
		Reputations:  []reputationInsight{{FactionID: 1098, Name: "Knights of the Ebon Blade", Standing: 42999, Flags: 1}, {FactionID: 1106, Name: "Argent Crusade", Standing: 42999, Flags: 1}},
		Titles:       []titleInsight{{ID: 123, Name: "the Kingslayer"}, {ID: 140, Name: "the Light of Dawn"}},
		Mounts:       []collectionSpell{{ID: 72286, Name: "Invincible"}, {ID: 60025, Name: "Albino Drake"}},
		Companions:   []collectionSpell{{ID: 65358, Name: "Argent Squire"}, {ID: 71840, Name: "Toxic Wasteling"}},
		Capabilities: map[string]bool{"reputations": true, "titles": true, "mounts": true, "companions": true},
	}
}

func (s *Server) loadCharacterCollections(ctx context.Context, guid uint32) characterCollections {
	out := characterCollections{Reputations: []reputationInsight{}, Titles: []titleInsight{}, Mounts: []collectionSpell{}, Companions: []collectionSpell{}, Capabilities: map[string]bool{}}
	factionIDs := []uint32{}
	if rows, err := s.s.Characters.QueryContext(ctx, fmt.Sprintf(`SELECT faction,standing,flags FROM %s.character_reputation WHERE guid=? ORDER BY standing DESC LIMIT 200`, s.c.CharactersDB), guid); err == nil {
		for rows.Next() {
			var item reputationInsight
			if rows.Scan(&item.FactionID, &item.Standing, &item.Flags) == nil {
				out.Reputations = append(out.Reputations, item)
				factionIDs = append(factionIDs, item.FactionID)
			}
		}
		rows.Close()
		out.Capabilities["reputations"] = true
	}
	if len(factionIDs) > 0 {
		names := map[uint32]string{}
		if rows, err := s.s.World.QueryContext(ctx, fmt.Sprintf(`SELECT ID,COALESCE(Name_Lang_enUS,'') FROM %s.faction_dbc WHERE ID IN (%s)`, s.c.WorldDB, placeholders(len(factionIDs))), uintArgs(factionIDs)...); err == nil {
			for rows.Next() {
				var id uint32
				var name string
				if rows.Scan(&id, &name) == nil {
					names[id] = name
				}
			}
			rows.Close()
			for index := range out.Reputations {
				out.Reputations[index].Name = names[out.Reputations[index].FactionID]
			}
		}
	}
	var knownTitles string
	if s.s.Characters.QueryRowContext(ctx, fmt.Sprintf(`SELECT knownTitles FROM %s.characters WHERE guid=?`, s.c.CharactersDB), guid).Scan(&knownTitles) == nil {
		blocks := []uint64{}
		for _, raw := range strings.Fields(knownTitles) {
			value, _ := strconv.ParseUint(raw, 10, 64)
			blocks = append(blocks, value)
		}
		if rows, err := s.s.World.QueryContext(ctx, fmt.Sprintf(`SELECT ID,Mask_ID,COALESCE(Name_Lang_enUS,'') FROM %s.char_titles_dbc ORDER BY Mask_ID`, s.c.WorldDB)); err == nil {
			for rows.Next() {
				var id, mask uint32
				var name string
				if rows.Scan(&id, &mask, &name) == nil && int(mask/32) < len(blocks) && blocks[mask/32]&(uint64(1)<<(mask%32)) != 0 {
					out.Titles = append(out.Titles, titleInsight{ID: id, Name: strings.TrimSpace(strings.ReplaceAll(name, "%s", ""))})
				}
			}
			rows.Close()
			out.Capabilities["titles"] = true
		}
	}
	spellIDs := []uint32{}
	if rows, err := s.s.Characters.QueryContext(ctx, fmt.Sprintf(`SELECT spell FROM %s.character_spell WHERE guid=? AND active=1 AND disabled=0`, s.c.CharactersDB), guid); err == nil {
		for rows.Next() {
			var id uint32
			if rows.Scan(&id) == nil {
				spellIDs = append(spellIDs, id)
			}
		}
		rows.Close()
	}
	if len(spellIDs) > 0 {
		query := fmt.Sprintf(`SELECT ID,COALESCE(Name_Lang_enUS,''),COALESCE(Description_Lang_enUS,''),SpellIconID,Effect_1,EffectApplyAuraName_1 FROM %s.spell_dbc WHERE ID IN (%s) AND ((Effect_1=6 AND EffectApplyAuraName_1=78) OR Effect_1=28)`, s.c.WorldDB, placeholders(len(spellIDs)))
		if rows, err := s.s.World.QueryContext(ctx, query, uintArgs(spellIDs)...); err == nil {
			for rows.Next() {
				var item collectionSpell
				var effect, aura uint32
				if rows.Scan(&item.ID, &item.Name, &item.Description, &item.IconID, &effect, &aura) == nil {
					if aura == 78 {
						out.Mounts = append(out.Mounts, item)
					} else if effect == 28 {
						out.Companions = append(out.Companions, item)
					}
				}
			}
			rows.Close()
			out.Capabilities["mounts"], out.Capabilities["companions"] = true, true
		}
	}
	return out
}
