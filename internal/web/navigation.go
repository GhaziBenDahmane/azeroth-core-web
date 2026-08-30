package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type navigationItem struct {
	ID        uint64 `json:"id"`
	Area      string `json:"area"`
	Label     string `json:"label"`
	URL       string `json:"url"`
	SortOrder int    `json:"sortOrder"`
	NewTab    bool   `json:"newTab"`
	Active    bool   `json:"active"`
}

func (s *Server) loadNavigation(r *http.Request, includeInactive bool) (bool, []navigationItem) {
	if s.c.MockMode {
		return false, []navigationItem{}
	}
	where := ""
	if !includeInactive {
		where = " AND active=1"
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,area,label,url,sort_order,new_tab,active FROM portal_navigation_items WHERE realm_key=?`+where+` ORDER BY area,sort_order,id`, s.c.RealmKey)
	if err != nil {
		return false, []navigationItem{}
	}
	defer rows.Close()
	out := []navigationItem{}
	for rows.Next() {
		var item navigationItem
		if rows.Scan(&item.ID, &item.Area, &item.Label, &item.URL, &item.SortOrder, &item.NewTab, &item.Active) == nil {
			out = append(out, item)
		}
	}
	var configured int
	_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_navigation_items WHERE realm_key=?`, s.c.RealmKey).Scan(&configured)
	return configured > 0, out
}

func (s *Server) adminNavigation(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	configured, items := s.loadNavigation(r, true)
	jsonOut(w, http.StatusOK, map[string]any{"configured": configured, "items": items})
}

func (s *Server) adminNavigationCreate(w http.ResponseWriter, r *http.Request) {
	s.adminNavigationSave(w, r, 0)
}

func (s *Server) adminNavigationUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid navigation item")
		return
	}
	s.adminNavigationSave(w, r, id)
}

func (s *Server) adminNavigationSave(w http.ResponseWriter, r *http.Request, id uint64) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	var input navigationItem
	if !decode(w, r, &input) {
		return
	}
	input.Area = strings.ToLower(strings.TrimSpace(input.Area))
	input.Label = strings.TrimSpace(input.Label)
	input.URL = strings.TrimSpace(input.URL)
	if (input.Area != "primary" && input.Area != "footer") || input.Label == "" || len(input.Label) > 80 || !validNavigationURL(input.URL) || input.SortOrder < -10000 || input.SortOrder > 10000 {
		problem(w, http.StatusUnprocessableEntity, "Navigation area, label, URL, or display order is invalid")
		return
	}
	if s.c.MockMode {
		if id == 0 {
			id = 1
		}
		jsonOut(w, http.StatusOK, map[string]any{"id": id})
		return
	}
	if id == 0 {
		result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_navigation_items(realm_key,area,label,url,sort_order,new_tab,active,created_by) VALUES(?,?,?,?,?,?,?,?)`, s.c.RealmKey, input.Area, input.Label, input.URL, input.SortOrder, input.NewTab, input.Active, actor.ID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not create navigation item")
			return
		}
		created, _ := result.LastInsertId()
		id = uint64(created)
	} else {
		result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_navigation_items SET area=?,label=?,url=?,sort_order=?,new_tab=?,active=? WHERE id=? AND realm_key=?`, input.Area, input.Label, input.URL, input.SortOrder, input.NewTab, input.Active, id, s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not update navigation item")
			return
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			var exists int
			_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_navigation_items WHERE id=? AND realm_key=?`, id, s.c.RealmKey).Scan(&exists)
			if exists == 0 {
				problem(w, http.StatusNotFound, "Navigation item not found")
				return
			}
		}
	}
	s.auditIdentity(r, actor.ID, "navigation.save", id, input.Area+": "+input.Label)
	jsonOut(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) adminNavigationDelete(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid navigation item")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"archived": true})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_navigation_items SET active=0 WHERE id=? AND realm_key=? AND active=1`, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not archive navigation item")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusNotFound, "Navigation item not found")
		return
	}
	s.auditIdentity(r, actor.ID, "navigation.archive", id, "Navigation item archived")
	jsonOut(w, http.StatusOK, map[string]bool{"archived": true})
}

func validNavigationURL(value string) bool {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "\r\n\\") {
		return len(value) <= 500
	}
	parsed, err := url.ParseRequestURI(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == "" && len(value) <= 500
}
