package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type savedAuditFilter struct {
	ID      uint64            `json:"id"`
	Name    string            `json:"name"`
	Query   map[string]string `json:"query"`
	Updated time.Time         `json:"updated"`
}

var auditFilterFields = map[string]bool{"q": true, "actor": true, "action": true, "source": true, "status": true, "realm": true, "from": true, "to": true}

func sanitizeAuditFilter(input map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range input {
		value = strings.TrimSpace(value)
		if auditFilterFields[key] && value != "" && len(value) <= 200 {
			out[key] = value
		}
	}
	return out
}

func (s *Server) adminAuditFilters(w http.ResponseWriter, r *http.Request) {
	account, ok := s.requireStaffPermission(r, "audit")
	if !ok {
		problem(w, http.StatusForbidden, "Audit access required")
		return
	}
	if s.c.MockMode {
		if r.Method == http.MethodPost {
			jsonOut(w, http.StatusCreated, map[string]any{"id": 1})
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"filters": []savedAuditFilter{{ID: 1, Name: "Failed operations", Query: map[string]string{"status": "failed"}, Updated: time.Now()}}})
		return
	}
	if r.Method == http.MethodPost {
		var input struct {
			Name  string            `json:"name"`
			Query map[string]string `json:"query"`
		}
		if !decode(w, r, &input) {
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		if input.Name == "" || len(input.Name) > 80 {
			problem(w, http.StatusUnprocessableEntity, "Filter name must contain 1–80 characters")
			return
		}
		payload, _ := json.Marshal(sanitizeAuditFilter(input.Query))
		result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_audit_filters(account_id,name,query_json) VALUES(?,?,?) ON DUPLICATE KEY UPDATE query_json=VALUES(query_json),updated_at=NOW()`, account.ID, input.Name, payload)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not save audit filter")
			return
		}
		id, _ := result.LastInsertId()
		jsonOut(w, http.StatusCreated, map[string]any{"id": id})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,name,query_json,updated_at FROM portal_audit_filters WHERE account_id=? ORDER BY name LIMIT 100`, account.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load audit filters")
		return
	}
	defer rows.Close()
	filters := []savedAuditFilter{}
	for rows.Next() {
		var item savedAuditFilter
		var payload []byte
		if rows.Scan(&item.ID, &item.Name, &payload, &item.Updated) == nil {
			_ = json.Unmarshal(payload, &item.Query)
			filters = append(filters, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"filters": filters})
}

func (s *Server) adminAuditFilterDelete(w http.ResponseWriter, r *http.Request) {
	account, ok := s.requireStaffPermission(r, "audit")
	if !ok {
		problem(w, http.StatusForbidden, "Audit access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid saved filter")
		return
	}
	if !s.c.MockMode {
		if _, err = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_audit_filters WHERE id=? AND account_id=?`, id, account.ID); err != nil {
			problem(w, http.StatusInternalServerError, "Could not delete audit filter")
			return
		}
	}
	jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
}
