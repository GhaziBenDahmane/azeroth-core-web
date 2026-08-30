package web

import (
	"context"
	"fmt"
)

type professionRecipe struct {
	SpellID       uint32 `json:"spellId"`
	Name          string `json:"name"`
	RankName      string `json:"rankName,omitempty"`
	Description   string `json:"description,omitempty"`
	IconID        uint32 `json:"iconId,omitempty"`
	RequiredSkill uint16 `json:"requiredSkill,omitempty"`
}

type professionCollection struct {
	ID      uint32             `json:"id"`
	Name    string             `json:"name"`
	Value   uint16             `json:"value"`
	Maximum uint16             `json:"maximum"`
	Recipes []professionRecipe `json:"recipes"`
}

func mockProfessionCollections() []professionCollection {
	return []professionCollection{
		{ID: 164, Name: "Blacksmithing", Value: 450, Maximum: 450, Recipes: []professionRecipe{{SpellID: 55377, Name: "Brilliant Titansteel Helm", RequiredSkill: 440}, {SpellID: 55372, Name: "Spiked Titansteel Helm", RequiredSkill: 440}, {SpellID: 61008, Name: "Icebane Chestguard", RequiredSkill: 425}}},
		{ID: 186, Name: "Mining", Value: 450, Maximum: 450, Recipes: []professionRecipe{{SpellID: 55211, Name: "Smelt Titanium", RequiredSkill: 450}, {SpellID: 49258, Name: "Smelt Saronite", RequiredSkill: 400}}},
	}
}

func (s *Server) loadProfessionCollections(ctx context.Context, guid uint32) ([]professionCollection, bool) {
	professions := []professionCollection{}
	byID := map[uint32]*professionCollection{}
	query := fmt.Sprintf(`SELECT skill,value,max FROM %s.character_skills WHERE guid=? AND skill IN (164,165,171,182,185,186,197,202,333,356,393,755,773) ORDER BY value DESC`, s.c.CharactersDB)
	rows, err := s.s.Characters.QueryContext(ctx, query, guid)
	if err != nil {
		return professions, false
	}
	for rows.Next() {
		var item professionCollection
		if rows.Scan(&item.ID, &item.Value, &item.Maximum) == nil {
			item.Name = professionNames[item.ID]
			if item.ID == 185 {
				item.Name = "Cooking"
			} else if item.ID == 356 {
				item.Name = "Fishing"
			}
			item.Recipes = []professionRecipe{}
			professions = append(professions, item)
			byID[item.ID] = &professions[len(professions)-1]
		}
	}
	rows.Close()
	if len(professions) == 0 {
		return professions, true
	}
	query = fmt.Sprintf(`SELECT sla.SkillLine,sp.ID,COALESCE(sp.Name_Lang_enUS,''),COALESCE(sp.NameSubtext_Lang_enUS,''),COALESCE(sp.Description_Lang_enUS,''),sp.SpellIconID,sla.MinSkillLineRank
		FROM %s.character_spell cs
		JOIN %s.skilllineability_dbc sla ON sla.Spell=cs.spell
		JOIN %s.spell_dbc sp ON sp.ID=cs.spell
		WHERE cs.guid=? AND cs.active=1 AND cs.disabled=0 AND sla.SkillLine IN (164,165,171,182,185,186,197,202,333,356,393,755,773)
		ORDER BY sla.SkillLine,sla.MinSkillLineRank DESC,sp.Name_Lang_enUS LIMIT 1500`, s.c.CharactersDB, s.c.WorldDB, s.c.WorldDB)
	rows, err = s.s.Characters.QueryContext(ctx, query, guid)
	if err != nil {
		return professions, false
	}
	defer rows.Close()
	for rows.Next() {
		var skill uint32
		var recipe professionRecipe
		if rows.Scan(&skill, &recipe.SpellID, &recipe.Name, &recipe.RankName, &recipe.Description, &recipe.IconID, &recipe.RequiredSkill) == nil && byID[skill] != nil {
			byID[skill].Recipes = append(byID[skill].Recipes, recipe)
		}
	}
	return professions, true
}
