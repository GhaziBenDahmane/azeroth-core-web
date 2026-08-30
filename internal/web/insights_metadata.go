package web

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
)

type talentDBC struct {
	TreeID uint32
	Tier   uint8
	Column uint8
	Rank   uint8
}

type spellDBC struct {
	Name, RankName, Description string
	IconID                      uint32
}

func placeholders(count int) string {
	if count <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func uintArgs(ids []uint32) []any {
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	return args
}

func uniqueIDs(ids []uint32) []uint32 {
	seen := map[uint32]bool{}
	out := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (s *Server) enrichArmoryMetadata(ctx context.Context, talents []talentInsight, glyphs []glyphInsight, achievements []achievementInsight) map[string]any {
	capabilities := map[string]any{
		"dbcMetadata": false, "talentRanks": false, "glyphMetadata": false, "achievementMetadata": false,
		"source": "Optional AzerothCore world DBC tables are not available; Wowhead links remain available for raw IDs.",
	}
	talentIDs := []uint32{}
	for _, group := range talents {
		for _, spell := range group.Spells {
			talentIDs = append(talentIDs, spell.ID)
		}
	}
	talentIDs = uniqueIDs(talentIDs)
	talentMeta, treeNames, talentOK := s.loadTalentDBC(ctx, talentIDs)

	glyphIDs := make([]uint32, 0, len(glyphs))
	for _, glyph := range glyphs {
		glyphIDs = append(glyphIDs, glyph.ID)
	}
	glyphSpells, glyphOK := s.loadGlyphDBC(ctx, uniqueIDs(glyphIDs))
	spellIDs := append([]uint32(nil), talentIDs...)
	for _, spellID := range glyphSpells {
		spellIDs = append(spellIDs, spellID)
	}
	spellMeta, spellOK := s.loadSpellDBC(ctx, uniqueIDs(spellIDs))

	if talentOK {
		for groupIndex := range talents {
			known := true
			treePoints := map[uint32]uint16{}
			for spellIndex := range talents[groupIndex].Spells {
				spell := &talents[groupIndex].Spells[spellIndex]
				meta, found := talentMeta[spell.ID]
				if !found {
					known = false
					continue
				}
				spell.Rank, spell.Tier, spell.Column, spell.TreeID = meta.Rank, meta.Tier, meta.Column, meta.TreeID
				spell.TreeName = treeNames[meta.TreeID]
				treePoints[meta.TreeID] += uint16(meta.Rank)
			}
			talents[groupIndex].PointsKnown = known && len(talents[groupIndex].Spells) > 0
			if talents[groupIndex].PointsKnown {
				for _, points := range treePoints {
					talents[groupIndex].Points += points
				}
			}
			for treeID, points := range treePoints {
				talents[groupIndex].Trees = append(talents[groupIndex].Trees, talentTree{ID: treeID, Name: treeNames[treeID], Points: points})
			}
			sort.Slice(talents[groupIndex].Trees, func(i, j int) bool { return talents[groupIndex].Trees[i].Points > talents[groupIndex].Trees[j].Points })
		}
		capabilities["talentRanks"] = true
	}
	if spellOK {
		for groupIndex := range talents {
			for spellIndex := range talents[groupIndex].Spells {
				spell := &talents[groupIndex].Spells[spellIndex]
				if meta, found := spellMeta[spell.ID]; found {
					spell.Name, spell.RankName, spell.Description, spell.IconID = meta.Name, meta.RankName, meta.Description, meta.IconID
				}
			}
		}
	}
	if glyphOK {
		for index := range glyphs {
			glyphs[index].SpellID = glyphSpells[glyphs[index].ID]
			if meta, found := spellMeta[glyphs[index].SpellID]; found {
				glyphs[index].Name, glyphs[index].Description, glyphs[index].IconID = meta.Name, meta.Description, meta.IconID
			}
		}
		capabilities["glyphMetadata"] = spellOK
	}
	if s.loadAchievementDBC(ctx, achievements) {
		capabilities["achievementMetadata"] = true
		capabilities["achievementCategories"] = s.loadAchievementCategories(ctx, achievements)
	}
	if talentOK || glyphOK || spellOK || capabilities["achievementMetadata"] == true {
		capabilities["dbcMetadata"] = true
		capabilities["source"] = "AzerothCore 3.3.5a DBC tables"
	}
	return capabilities
}

type achievementCategoryDBC struct {
	Name   string
	Parent uint32
}

func (s *Server) loadAchievementCategories(ctx context.Context, achievements []achievementInsight) bool {
	if len(achievements) == 0 {
		return true
	}
	rows, err := s.s.World.QueryContext(ctx, fmt.Sprintf("SELECT ID,Parent,COALESCE(Name_Lang_enUS,'') FROM `%s`.achievement_category_dbc", s.c.WorldDB))
	if err != nil {
		slog.Debug("armory achievement categories unavailable", "realm", s.c.RealmKey, "error", err)
		return false
	}
	defer rows.Close()
	categories := map[uint32]achievementCategoryDBC{}
	for rows.Next() {
		var id uint32
		var parent int32
		var item achievementCategoryDBC
		if rows.Scan(&id, &parent, &item.Name) == nil {
			if parent > 0 {
				item.Parent = uint32(parent)
			}
			categories[id] = item
		}
	}
	if rows.Err() != nil {
		return false
	}
	for index := range achievements {
		category, ok := categories[achievements[index].Category]
		if !ok {
			continue
		}
		achievements[index].CategoryName = category.Name
		achievements[index].ParentCategory = category.Parent
		achievements[index].ParentCategoryName = categories[category.Parent].Name
	}
	return true
}

func (s *Server) loadTalentDBC(ctx context.Context, ids []uint32) (map[uint32]talentDBC, map[uint32]string, bool) {
	metadata, trees := map[uint32]talentDBC{}, map[uint32]string{}
	if len(ids) == 0 {
		return metadata, trees, true
	}
	list := placeholders(len(ids))
	clauses, args := []string{}, []any{}
	for rank := 1; rank <= 9; rank++ {
		clauses = append(clauses, fmt.Sprintf("SpellRank_%d IN (%s)", rank, list))
		args = append(args, uintArgs(ids)...)
	}
	query := fmt.Sprintf("SELECT TabID,TierID,ColumnIndex,SpellRank_1,SpellRank_2,SpellRank_3,SpellRank_4,SpellRank_5,SpellRank_6,SpellRank_7,SpellRank_8,SpellRank_9 FROM `%s`.talent_dbc WHERE %s", s.c.WorldDB, strings.Join(clauses, " OR "))
	rows, err := s.s.World.QueryContext(ctx, query, args...)
	if err != nil {
		slog.Debug("armory talent metadata unavailable", "realm", s.c.RealmKey, "error", err)
		return metadata, trees, false
	}
	defer rows.Close()
	for rows.Next() {
		var treeID uint32
		var tier, column uint8
		var ranks [9]uint32
		if rows.Scan(&treeID, &tier, &column, &ranks[0], &ranks[1], &ranks[2], &ranks[3], &ranks[4], &ranks[5], &ranks[6], &ranks[7], &ranks[8]) != nil {
			continue
		}
		for index, spellID := range ranks {
			if spellID != 0 {
				metadata[spellID] = talentDBC{TreeID: treeID, Tier: tier, Column: column, Rank: uint8(index + 1)}
			}
		}
	}
	if err = rows.Err(); err != nil {
		return metadata, trees, false
	}
	treeIDs := []uint32{}
	for _, meta := range metadata {
		treeIDs = append(treeIDs, meta.TreeID)
	}
	treeIDs = uniqueIDs(treeIDs)
	if len(treeIDs) == 0 {
		return metadata, trees, true
	}
	rows, err = s.s.World.QueryContext(ctx, fmt.Sprintf("SELECT ID,COALESCE(Name_Lang_enUS,'') FROM `%s`.talenttab_dbc WHERE ID IN (%s)", s.c.WorldDB, placeholders(len(treeIDs))), uintArgs(treeIDs)...)
	if err != nil {
		slog.Debug("armory talent tree names unavailable", "realm", s.c.RealmKey, "error", err)
		return metadata, trees, true
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var name string
		if rows.Scan(&id, &name) == nil {
			trees[id] = name
		}
	}
	return metadata, trees, true
}

func (s *Server) loadGlyphDBC(ctx context.Context, ids []uint32) (map[uint32]uint32, bool) {
	metadata := map[uint32]uint32{}
	if len(ids) == 0 {
		return metadata, true
	}
	rows, err := s.s.World.QueryContext(ctx, fmt.Sprintf("SELECT ID,SpellID FROM `%s`.glyphproperties_dbc WHERE ID IN (%s)", s.c.WorldDB, placeholders(len(ids))), uintArgs(ids)...)
	if err != nil {
		slog.Debug("armory glyph metadata unavailable", "realm", s.c.RealmKey, "error", err)
		return metadata, false
	}
	defer rows.Close()
	for rows.Next() {
		var id, spellID uint32
		if rows.Scan(&id, &spellID) == nil {
			metadata[id] = spellID
		}
	}
	return metadata, rows.Err() == nil
}

func (s *Server) loadSpellDBC(ctx context.Context, ids []uint32) (map[uint32]spellDBC, bool) {
	metadata := map[uint32]spellDBC{}
	if len(ids) == 0 {
		return metadata, true
	}
	query := fmt.Sprintf("SELECT ID,COALESCE(Name_Lang_enUS,''),COALESCE(NameSubtext_Lang_enUS,''),COALESCE(Description_Lang_enUS,''),SpellIconID FROM `%s`.spell_dbc WHERE ID IN (%s)", s.c.WorldDB, placeholders(len(ids)))
	rows, err := s.s.World.QueryContext(ctx, query, uintArgs(ids)...)
	if err != nil {
		slog.Debug("armory spell metadata unavailable", "realm", s.c.RealmKey, "error", err)
		return metadata, false
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var item spellDBC
		if rows.Scan(&id, &item.Name, &item.RankName, &item.Description, &item.IconID) == nil {
			metadata[id] = item
		}
	}
	return metadata, rows.Err() == nil
}

func (s *Server) loadAchievementDBC(ctx context.Context, achievements []achievementInsight) bool {
	if len(achievements) == 0 {
		return true
	}
	ids := make([]uint32, len(achievements))
	byID := map[uint32]*achievementInsight{}
	for index := range achievements {
		ids[index], byID[achievements[index].ID] = achievements[index].ID, &achievements[index]
	}
	query := fmt.Sprintf("SELECT ID,COALESCE(Title_Lang_enUS,''),COALESCE(Description_Lang_enUS,''),Category,Points,IconID FROM `%s`.achievement_dbc WHERE ID IN (%s)", s.c.WorldDB, placeholders(len(ids)))
	rows, err := s.s.World.QueryContext(ctx, query, uintArgs(ids)...)
	if err != nil {
		slog.Debug("armory achievement metadata unavailable", "realm", s.c.RealmKey, "error", err)
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var name, description string
		var category uint32
		var points uint16
		var iconID uint32
		if rows.Scan(&id, &name, &description, &category, &points, &iconID) == nil {
			if item := byID[id]; item != nil {
				item.Name, item.Description, item.Category, item.Points, item.IconID = name, description, category, points, iconID
			}
		}
	}
	return rows.Err() == nil
}

func parseItemEnhancements(encoded string) []itemEnhancement {
	fields := strings.Fields(encoded)
	items := []itemEnhancement{}
	for slot := 0; slot <= 4; slot++ {
		index := slot * 3
		if index >= len(fields) {
			break
		}
		id, err := strconv.ParseUint(fields[index], 10, 32)
		if err != nil || id == 0 {
			continue
		}
		kind := "enchant"
		if slot == 1 {
			kind = "temporary"
		} else if slot >= 2 {
			kind = "gem"
		}
		items = append(items, itemEnhancement{Slot: uint8(slot), Kind: kind, EnchantmentID: uint32(id)})
	}
	return items
}

func (s *Server) enrichItemEnhancements(ctx context.Context, items []armoryItem) bool {
	ids := []uint32{}
	for index := range items {
		items[index].Enhancements = parseItemEnhancements(items[index].Enchantments)
		for _, enhancement := range items[index].Enhancements {
			ids = append(ids, enhancement.EnchantmentID)
		}
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return true
	}
	type metadata struct {
		name   string
		itemID uint32
	}
	byID := map[uint32]metadata{}
	query := fmt.Sprintf("SELECT ID,COALESCE(Name_Lang_enUS,''),Src_ItemID FROM `%s`.spellitemenchantment_dbc WHERE ID IN (%s)", s.c.WorldDB, placeholders(len(ids)))
	rows, err := s.s.World.QueryContext(ctx, query, uintArgs(ids)...)
	if err != nil {
		slog.Debug("armory enchantment metadata unavailable", "realm", s.c.RealmKey, "error", err)
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var id, itemID uint32
		var name string
		if rows.Scan(&id, &name, &itemID) == nil {
			byID[id] = metadata{name: name, itemID: itemID}
		}
	}
	for itemIndex := range items {
		for enhancementIndex := range items[itemIndex].Enhancements {
			enhancement := &items[itemIndex].Enhancements[enhancementIndex]
			if found, ok := byID[enhancement.EnchantmentID]; ok {
				enhancement.Name, enhancement.ItemID = found.name, found.itemID
			}
		}
	}
	return rows.Err() == nil
}
