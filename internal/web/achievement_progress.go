package web

import (
	"context"
	"fmt"
)

func (s *Server) loadAchievementCriteria(ctx context.Context, guid uint32, achievements []achievementInsight) bool {
	if len(achievements) == 0 {
		return true
	}
	ids := make([]uint32, 0, len(achievements))
	byAchievement := make(map[uint32]*achievementInsight, len(achievements))
	for index := range achievements {
		ids = append(ids, achievements[index].ID)
		byAchievement[achievements[index].ID] = &achievements[index]
	}
	query := fmt.Sprintf(`SELECT ID,Achievement_Id,COALESCE(Description_Lang_enUS,''),Quantity FROM %s.achievement_criteria_dbc WHERE Achievement_Id IN (%s) ORDER BY Achievement_Id,ID`, s.c.WorldDB, placeholders(len(ids)))
	rows, err := s.s.World.QueryContext(ctx, query, uintArgs(ids)...)
	if err != nil {
		return false
	}
	type criterionLocation struct {
		achievementID uint32
		index         int
	}
	criteriaByID := map[uint32]criterionLocation{}
	for rows.Next() {
		var achievementID uint32
		var item achievementCriterion
		if rows.Scan(&item.ID, &achievementID, &item.Description, &item.Required) == nil && byAchievement[achievementID] != nil {
			if item.Required == 0 {
				item.Required = 1
			}
			item.Counter, item.Complete = item.Required, true
			byAchievement[achievementID].Criteria = append(byAchievement[achievementID].Criteria, item)
			criteriaByID[item.ID] = criterionLocation{achievementID: achievementID, index: len(byAchievement[achievementID].Criteria) - 1}
		}
	}
	rows.Close()
	if len(criteriaByID) == 0 {
		return true
	}
	criteriaIDs := make([]uint32, 0, len(criteriaByID))
	for id := range criteriaByID {
		criteriaIDs = append(criteriaIDs, id)
	}
	query = fmt.Sprintf(`SELECT criteria,counter FROM %s.character_achievement_progress WHERE guid=? AND criteria IN (%s)`, s.c.CharactersDB, placeholders(len(criteriaIDs)))
	args := []any{guid}
	args = append(args, uintArgs(criteriaIDs)...)
	if rows, err = s.s.Characters.QueryContext(ctx, query, args...); err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var id uint32
		var counter uint64
		if rows.Scan(&id, &counter) == nil {
			location, found := criteriaByID[id]
			if !found {
				continue
			}
			item := &byAchievement[location.achievementID].Criteria[location.index]
			item.Counter = counter
			item.Complete = counter >= item.Required
		}
	}
	return true
}
