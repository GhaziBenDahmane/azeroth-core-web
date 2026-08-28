package web

import (
	"context"
	"fmt"
)

type profession struct {
	ID      uint32 `json:"id"`
	Name    string `json:"name"`
	Value   uint16 `json:"value"`
	Maximum uint16 `json:"maximum"`
}

type characterProfile struct {
	Achievements uint32       `json:"achievements"`
	Exalted      uint32       `json:"exaltedReputations"`
	TalentSpecs  uint8        `json:"talentSpecs"`
	TalentSpells uint32       `json:"talentSpells"`
	Glyphs       uint8        `json:"glyphs"`
	Professions  []profession `json:"professions"`
}

var professionNames = map[uint32]string{164: "Blacksmithing", 165: "Leatherworking", 171: "Alchemy", 182: "Herbalism", 186: "Mining", 197: "Tailoring", 202: "Engineering", 333: "Enchanting", 393: "Skinning", 755: "Jewelcrafting", 773: "Inscription"}

func (s *Server) loadCharacterProfile(ctx context.Context, guid uint32) characterProfile {
	p := characterProfile{Professions: []profession{}}
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s.character_achievement WHERE guid=?", s.c.CharactersDB)
	_ = s.s.Characters.QueryRowContext(ctx, q, guid).Scan(&p.Achievements)
	q = fmt.Sprintf("SELECT COUNT(*) FROM %s.character_reputation WHERE guid=? AND standing>=42000", s.c.CharactersDB)
	_ = s.s.Characters.QueryRowContext(ctx, q, guid).Scan(&p.Exalted)
	q = fmt.Sprintf("SELECT COUNT(DISTINCT talentGroup),COUNT(*) FROM %s.character_talent WHERE guid=?", s.c.CharactersDB)
	_ = s.s.Characters.QueryRowContext(ctx, q, guid).Scan(&p.TalentSpecs, &p.TalentSpells)
	q = fmt.Sprintf("SELECT COALESCE(MAX((glyph1<>0)+(glyph2<>0)+(glyph3<>0)+(glyph4<>0)+(glyph5<>0)+(glyph6<>0)),0) FROM %s.character_glyphs WHERE guid=?", s.c.CharactersDB)
	_ = s.s.Characters.QueryRowContext(ctx, q, guid).Scan(&p.Glyphs)
	q = fmt.Sprintf("SELECT skill,value,max FROM %s.character_skills WHERE guid=? AND skill IN (164,165,171,182,186,197,202,333,393,755,773) ORDER BY value DESC", s.c.CharactersDB)
	if rows, err := s.s.Characters.QueryContext(ctx, q, guid); err == nil {
		defer rows.Close()
		for rows.Next() {
			var x profession
			if rows.Scan(&x.ID, &x.Value, &x.Maximum) == nil {
				x.Name = professionNames[x.ID]
				p.Professions = append(p.Professions, x)
			}
		}
	}
	return p
}
