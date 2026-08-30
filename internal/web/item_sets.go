package web

import (
	"context"
	"fmt"
)

type equippedItemSet struct {
	ID       uint32         `json:"id"`
	Name     string         `json:"name"`
	Equipped int            `json:"equipped"`
	Bonuses  []itemSetBonus `json:"bonuses"`
	Metadata bool           `json:"metadata"`
}

type itemSetBonus struct {
	Pieces  uint8  `json:"pieces"`
	SpellID uint32 `json:"spellId"`
	Name    string `json:"name,omitempty"`
	Active  bool   `json:"active"`
}

func (s *Server) loadEquippedItemSets(ctx context.Context, items []armoryItem) []equippedItemSet {
	counts := map[uint32]int{}
	ids := []uint32{}
	for _, item := range items {
		if item.SetID == 0 {
			continue
		}
		if counts[item.SetID] == 0 {
			ids = append(ids, item.SetID)
		}
		counts[item.SetID]++
	}
	if len(ids) == 0 {
		return []equippedItemSet{}
	}
	out := make([]equippedItemSet, 0, len(ids))
	for _, id := range ids {
		out = append(out, equippedItemSet{ID: id, Name: fmt.Sprintf("Item set %d", id), Equipped: counts[id], Bonuses: []itemSetBonus{}})
	}
	byID := map[uint32]*equippedItemSet{}
	for index := range out {
		byID[out[index].ID] = &out[index]
	}
	query := fmt.Sprintf(`SELECT ID,COALESCE(Name_Lang_enUS,''),SetSpellID_1,SetThreshold_1,SetSpellID_2,SetThreshold_2,SetSpellID_3,SetThreshold_3,SetSpellID_4,SetThreshold_4,SetSpellID_5,SetThreshold_5,SetSpellID_6,SetThreshold_6,SetSpellID_7,SetThreshold_7,SetSpellID_8,SetThreshold_8 FROM %s.itemset_dbc WHERE ID IN (%s)`, s.c.WorldDB, placeholders(len(ids)))
	rows, err := s.s.World.QueryContext(ctx, query, uintArgs(ids)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	spellIDs := []uint32{}
	for rows.Next() {
		var id uint32
		var name string
		var spells [8]uint32
		var thresholds [8]uint8
		if rows.Scan(&id, &name, &spells[0], &thresholds[0], &spells[1], &thresholds[1], &spells[2], &thresholds[2], &spells[3], &thresholds[3], &spells[4], &thresholds[4], &spells[5], &thresholds[5], &spells[6], &thresholds[6], &spells[7], &thresholds[7]) != nil {
			continue
		}
		set := byID[id]
		if set == nil {
			continue
		}
		if name != "" {
			set.Name = name
		}
		set.Metadata = true
		for index, spellID := range spells {
			if spellID == 0 || thresholds[index] == 0 {
				continue
			}
			set.Bonuses = append(set.Bonuses, itemSetBonus{Pieces: thresholds[index], SpellID: spellID, Active: counts[id] >= int(thresholds[index])})
			spellIDs = append(spellIDs, spellID)
		}
	}
	metadata, _ := s.loadSpellDBC(ctx, uniqueIDs(spellIDs))
	for index := range out {
		for bonusIndex := range out[index].Bonuses {
			out[index].Bonuses[bonusIndex].Name = metadata[out[index].Bonuses[bonusIndex].SpellID].Name
		}
	}
	return out
}
