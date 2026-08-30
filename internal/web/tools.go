package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type toolResource struct {
	ID          uint64    `json:"id"`
	Kind        string    `json:"kind"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	Summary     string    `json:"summary"`
	Body        string    `json:"body"`
	Version     string    `json:"version,omitempty"`
	DownloadURL string    `json:"downloadUrl,omitempty"`
	ImageURL    string    `json:"imageUrl,omitempty"`
	Tags        string    `json:"tags,omitempty"`
	Status      string    `json:"status"`
	SortOrder   int       `json:"sortOrder"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const toolResourceSelect = `id,kind,title,slug,summary,body,version,download_url,image_url,tags,status,sort_order,updated_at`

func mockToolResources() []toolResource {
	now := time.Now()
	return []toolResource{
		{ID: 1, Kind: "addon", Title: "Deadly Boss Mods", Slug: "deadly-boss-mods", Summary: "Encounter timers and warnings for Wrath raids.", Body: "Install the extracted folder in Interface/AddOns and restart the client.", Version: "3.3.5a", DownloadURL: "https://github.com/DeadlyBossMods/DBM-WotLK", Tags: "raids,boss-mod", Status: "published", UpdatedAt: now},
		{ID: 2, Kind: "weakaura", Title: "Icecrown raid reminders", Slug: "icecrown-raid-reminders", Summary: "A curated set of visual reminders for Icecrown Citadel.", Body: "Import the provided string through WeakAuras after verifying its author and version.", Version: "WotLK", DownloadURL: "https://wago.io/wotlk-weakauras", Tags: "icc,raid", Status: "published", UpdatedAt: now.Add(-time.Hour)},
	}
}

func validateToolResource(item *toolResource) error {
	item.Kind, item.Title, item.Slug = strings.ToLower(strings.TrimSpace(item.Kind)), strings.TrimSpace(item.Title), strings.ToLower(strings.TrimSpace(item.Slug))
	item.Summary, item.Body, item.Version = strings.TrimSpace(item.Summary), strings.TrimSpace(item.Body), strings.TrimSpace(item.Version)
	item.DownloadURL, item.ImageURL, item.Tags, item.Status = strings.TrimSpace(item.DownloadURL), strings.TrimSpace(item.ImageURL), strings.TrimSpace(item.Tags), strings.ToLower(strings.TrimSpace(item.Status))
	if item.Kind != "addon" && item.Kind != "weakaura" || len(item.Title) < 2 || len(item.Title) > 160 || !arenaSeasonSlug.MatchString(item.Slug) || len(item.Slug) > 160 || len(item.Summary) > 1000 || item.Body == "" || len(item.Body) > 100000 || len(item.Version) > 40 || len(item.Tags) > 500 {
		return fmt.Errorf("invalid resource fields")
	}
	if item.Status != "draft" && item.Status != "published" && item.Status != "archived" {
		return fmt.Errorf("invalid resource status")
	}
	for _, raw := range []string{item.DownloadURL, item.ImageURL} {
		if raw == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
			return fmt.Errorf("resource links must use absolute HTTPS URLs")
		}
	}
	return nil
}

func (s *Server) publicTools(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if kind != "" && kind != "addon" && kind != "weakaura" || len(query) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Invalid resource filter")
		return
	}
	if s.c.MockMode {
		items := []toolResource{}
		for _, item := range mockToolResources() {
			if (kind == "" || item.Kind == kind) && (query == "" || strings.Contains(strings.ToLower(item.Title+" "+item.Summary+" "+item.Tags), strings.ToLower(query))) {
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"resources": items})
		return
	}
	where, args := "realm_key=? AND status='published'", []any{s.c.RealmKey}
	if kind != "" {
		where += " AND kind=?"
		args = append(args, kind)
	}
	if query != "" {
		where += " AND (title LIKE ? OR summary LIKE ? OR tags LIKE ?)"
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT `+toolResourceSelect+` FROM portal_resources WHERE `+where+` ORDER BY sort_order,updated_at DESC LIMIT 100`, args...)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load resources")
		return
	}
	defer rows.Close()
	items := []toolResource{}
	for rows.Next() {
		var item toolResource
		if rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Slug, &item.Summary, &item.Body, &item.Version, &item.DownloadURL, &item.ImageURL, &item.Tags, &item.Status, &item.SortOrder, &item.UpdatedAt) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"resources": items})
}

func (s *Server) adminTools(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			jsonOut(w, http.StatusOK, map[string]any{"resources": mockToolResources()})
			return
		}
		rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT `+toolResourceSelect+` FROM portal_resources WHERE realm_key=? ORDER BY sort_order,updated_at DESC LIMIT 500`, s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load resources")
			return
		}
		defer rows.Close()
		items := []toolResource{}
		for rows.Next() {
			var item toolResource
			if rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Slug, &item.Summary, &item.Body, &item.Version, &item.DownloadURL, &item.ImageURL, &item.Tags, &item.Status, &item.SortOrder, &item.UpdatedAt) == nil {
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"resources": items})
		return
	}
	var item toolResource
	if !decode(w, r, &item) {
		return
	}
	if err := validateToolResource(&item); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]any{"id": 3})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_resources(realm_key,kind,title,slug,summary,body,version,download_url,image_url,tags,status,sort_order,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, item.Kind, item.Title, item.Slug, item.Summary, item.Body, item.Version, item.DownloadURL, item.ImageURL, item.Tags, item.Status, item.SortOrder, actor.ID)
	if err != nil {
		problem(w, http.StatusConflict, "Could not create resource; check that the slug is unique")
		return
	}
	id, _ := result.LastInsertId()
	jsonOut(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) adminToolItem(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid resource")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodDelete {
		_, err = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_resources WHERE id=? AND realm_key=?`, id, s.c.RealmKey)
	} else {
		var item toolResource
		if !decode(w, r, &item) {
			return
		}
		if validationErr := validateToolResource(&item); validationErr != nil {
			problem(w, http.StatusUnprocessableEntity, validationErr.Error())
			return
		}
		_, err = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_resources SET kind=?,title=?,slug=?,summary=?,body=?,version=?,download_url=?,image_url=?,tags=?,status=?,sort_order=?,created_by=? WHERE id=? AND realm_key=?`, item.Kind, item.Title, item.Slug, item.Summary, item.Body, item.Version, item.DownloadURL, item.ImageURL, item.Tags, item.Status, item.SortOrder, actor.ID, id, s.c.RealmKey)
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update resource")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) itemDatabase(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Search with 2–100 characters")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"items": []map[string]any{{"id": 49623, "name": "Shadowmourne", "quality": 5, "itemLevel": 284, "requiredLevel": 80, "inventoryType": 17}, {"id": 50734, "name": "Royal Scepter of Terenas II", "quality": 4, "itemLevel": 284, "requiredLevel": 80, "inventoryType": 21}}})
		return
	}
	rows, err := s.s.World.QueryContext(r.Context(), fmt.Sprintf(`SELECT entry,name,Quality,ItemLevel,RequiredLevel,InventoryType FROM %s.item_template WHERE name LIKE ? ORDER BY ItemLevel DESC,entry LIMIT 50`, s.c.WorldDB), "%"+query+"%")
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not search the item database")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uint32
		var name string
		var quality, level, required, inventory uint16
		if rows.Scan(&id, &name, &quality, &level, &required, &inventory) == nil {
			items = append(items, map[string]any{"id": id, "name": name, "quality": quality, "itemLevel": level, "requiredLevel": required, "inventoryType": inventory})
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) talentCalculator(w http.ResponseWriter, r *http.Request) {
	classID, _ := strconv.Atoi(r.URL.Query().Get("class"))
	if classID < 1 || classID > 11 || classID == 10 {
		problem(w, http.StatusUnprocessableEntity, "Choose a valid WotLK class")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"class": classID, "pointCap": 71, "trees": []map[string]any{{"id": 383, "name": "Retribution", "talents": []map[string]any{{"spellId": 20042, "name": "Improved Blessing of Might", "tier": 0, "column": 1, "maxRank": 5}, {"spellId": 53385, "name": "Divine Storm", "tier": 1, "column": 2, "maxRank": 1}}}}})
		return
	}
	mask := uint32(1 << (classID - 1))
	rows, err := s.s.World.QueryContext(r.Context(), fmt.Sprintf(`SELECT ID,COALESCE(Name_Lang_enUS,'') FROM %s.talenttab_dbc WHERE (ClassMask & ?)<>0 ORDER BY OrderIndex`, s.c.WorldDB), mask)
	if err != nil {
		problem(w, http.StatusNotImplemented, "Talent DBC metadata is not installed for this realm")
		return
	}
	defer rows.Close()
	trees := []map[string]any{}
	for rows.Next() {
		var treeID uint32
		var treeName string
		if rows.Scan(&treeID, &treeName) != nil {
			continue
		}
		talents := []map[string]any{}
		query := fmt.Sprintf(`SELECT TierID,ColumnIndex,SpellRank_1,SpellRank_2,SpellRank_3,SpellRank_4,SpellRank_5,SpellRank_6,SpellRank_7,SpellRank_8,SpellRank_9 FROM %s.talent_dbc WHERE TabID=? ORDER BY TierID,ColumnIndex`, s.c.WorldDB)
		if talentRows, talentErr := s.s.World.QueryContext(r.Context(), query, treeID); talentErr == nil {
			for talentRows.Next() {
				var tier, column uint8
				var ranks [9]uint32
				if talentRows.Scan(&tier, &column, &ranks[0], &ranks[1], &ranks[2], &ranks[3], &ranks[4], &ranks[5], &ranks[6], &ranks[7], &ranks[8]) != nil {
					continue
				}
				maxRank := 0
				for _, spell := range ranks {
					if spell > 0 {
						maxRank++
					}
				}
				meta, _ := s.loadSpellDBC(r.Context(), []uint32{ranks[0]})
				talents = append(talents, map[string]any{"spellId": ranks[0], "name": meta[ranks[0]].Name, "description": meta[ranks[0]].Description, "iconId": meta[ranks[0]].IconID, "tier": tier, "column": column, "maxRank": maxRank})
			}
			talentRows.Close()
		}
		trees = append(trees, map[string]any{"id": treeID, "name": treeName, "talents": talents})
	}
	jsonOut(w, http.StatusOK, map[string]any{"class": classID, "pointCap": 71, "trees": trees})
}
