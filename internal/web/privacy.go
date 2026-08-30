package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type privacyRequest struct {
	ID         uint64     `json:"id"`
	AccountID  uint32     `json:"accountId"`
	Username   string     `json:"username,omitempty"`
	RealmKey   string     `json:"realmKey"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	PlayerNote string     `json:"playerNote,omitempty"`
	StaffNote  string     `json:"staffNote,omitempty"`
	HandledBy  uint32     `json:"handledBy,omitempty"`
	Created    time.Time  `json:"created"`
	Updated    time.Time  `json:"updated"`
	Completed  *time.Time `json:"completed,omitempty"`
}

func (s *Server) privacyExport(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		username, ok := s.mockUser(r)
		if !ok {
			problem(w, 401, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		w.Header().Set("Content-Disposition", `attachment; filename="portal-data-export.json"`)
		jsonOut(w, 200, map[string]any{"generatedAt": time.Now(), "account": map[string]any{"username": username, "email": "demo@example.com"}, "characters": mockCharacters, "orders": s.mock.orders, "wallet": s.mock.ledger, "notifications": s.mock.notifications, "privacyRequests": s.mock.privacy})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	characters, _ := s.characterRows(r.Context(), a.ID)
	type exportRow struct {
		ID      uint64    `json:"id"`
		Label   string    `json:"label"`
		Status  string    `json:"status"`
		Created time.Time `json:"created"`
	}
	orders := []exportRow{}
	rows, queryErr := s.s.Auth.QueryContext(r.Context(), "SELECT id,CONCAT('Product ',product_id),status,created_at FROM portal_orders WHERE account_id=? AND realm_key=? ORDER BY id", a.ID, s.c.RealmKey)
	if queryErr == nil {
		for rows.Next() {
			var x exportRow
			if rows.Scan(&x.ID, &x.Label, &x.Status, &x.Created) == nil {
				orders = append(orders, x)
			}
		}
		rows.Close()
	}
	w.Header().Set("Content-Disposition", `attachment; filename="portal-data-export.json"`)
	jsonOut(w, 200, map[string]any{"generatedAt": time.Now(), "account": map[string]any{"id": a.ID, "username": a.Username, "email": a.Email}, "realm": s.c.RealmKey, "characters": characters, "orders": orders, "notice": "This export excludes authentication secrets and session tokens."})
}

func (s *Server) privacyRequests(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, 401, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		jsonOut(w, 200, map[string]any{"requests": s.mock.privacy})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,account_id,realm_key,request_type,status,player_note,staff_note,handled_by,created_at,updated_at,completed_at FROM portal_privacy_requests WHERE account_id=? ORDER BY id DESC", a.ID)
	if err != nil {
		problem(w, 500, "Could not load privacy requests")
		return
	}
	defer rows.Close()
	out := []privacyRequest{}
	for rows.Next() {
		var x privacyRequest
		if rows.Scan(&x.ID, &x.AccountID, &x.RealmKey, &x.Type, &x.Status, &x.PlayerNote, &x.StaffNote, &x.HandledBy, &x.Created, &x.Updated, &x.Completed) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"requests": out})
}

func (s *Server) privacyDeletionRequest(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, 401, "Sign in required")
			return
		}
		var in struct{ Confirmation, Note string }
		if !decode(w, r, &in) {
			return
		}
		if in.Confirmation != "DELETE" {
			problem(w, 422, "Type DELETE to confirm")
			return
		}
		s.mock.mu.Lock()
		id := uint64(len(s.mock.privacy) + 1)
		now := time.Now()
		s.mock.privacy = append([]privacyRequest{{ID: id, AccountID: 1, Username: "DEMO", RealmKey: s.c.RealmKey, Type: "deletion", Status: "pending", PlayerNote: strings.TrimSpace(in.Note), Created: now, Updated: now}}, s.mock.privacy...)
		s.mock.mu.Unlock()
		jsonOut(w, 201, map[string]any{"ok": true, "id": id})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct{ Confirmation, Note string }
	if !decode(w, r, &in) {
		return
	}
	if in.Confirmation != "DELETE" || len(in.Note) > 500 {
		problem(w, 422, "Type DELETE to confirm")
		return
	}
	var exists int
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_privacy_requests WHERE account_id=? AND request_type='deletion' AND status IN ('pending','processing')", a.ID).Scan(&exists)
	if exists > 0 {
		problem(w, 409, "A deletion request is already active")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_privacy_requests(account_id,realm_key,request_type,player_note) VALUES(?,?,'deletion',?)", a.ID, s.c.RealmKey, strings.TrimSpace(in.Note))
	if err != nil {
		problem(w, 500, "Could not create deletion request")
		return
	}
	id, _ := result.LastInsertId()
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) privacyRequestCancel(w http.ResponseWriter, r *http.Request) {
	id, parseErr := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, 401, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.privacy {
			if s.mock.privacy[i].ID == id && s.mock.privacy[i].Status == "pending" {
				s.mock.privacy[i].Status = "cancelled"
				s.mock.privacy[i].Updated = time.Now()
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, 409, "Request cannot be cancelled")
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	if parseErr != nil {
		problem(w, 400, "Invalid request")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_privacy_requests SET status='cancelled' WHERE id=? AND account_id=? AND status='pending'", id, a.ID)
	if err != nil {
		problem(w, 500, "Could not cancel request")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, 409, "Request cannot be cancelled")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminPrivacyRequests(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "players"); !ok {
		problem(w, 403, "Player management access required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	status, requestType, search := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("type")), strings.TrimSpace(r.URL.Query().Get("q"))
	if status != "" && !map[string]bool{"pending": true, "processing": true, "completed": true, "rejected": true, "cancelled": true}[status] || requestType != "" && !map[string]bool{"deletion": true, "anonymization": true}[requestType] || len(search) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Invalid privacy request filters")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		items := append([]privacyRequest(nil), s.mock.privacy...)
		s.mock.mu.Unlock()
		filtered := items[:0]
		for _, item := range items {
			if status != "" && item.Status != status || requestType != "" && item.Type != requestType || search != "" && !strings.Contains(strings.ToLower(item.Username+" "+item.PlayerNote), strings.ToLower(search)) {
				continue
			}
			filtered = append(filtered, item)
		}
		filtered, meta := slicePage(filtered, page, perPage)
		jsonOut(w, 200, map[string]any{"requests": filtered, "pagination": meta})
		return
	}
	where, args := " WHERE 1=1", []any{}
	if status != "" {
		where += " AND p.status=?"
		args = append(args, status)
	}
	if requestType != "" {
		where += " AND p.request_type=?"
		args = append(args, requestType)
	}
	if search != "" {
		where += " AND (a.username LIKE ? OR p.player_note LIKE ?)"
		args = append(args, likePattern(search), likePattern(search))
	}
	base := fmt.Sprintf(" FROM portal_privacy_requests p LEFT JOIN `%s`.account a ON a.id=p.account_id", s.c.AuthDB) + where
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*)"+base, args...).Scan(&total); err != nil {
		problem(w, 500, "Could not count privacy requests")
		return
	}
	meta := paginationMeta(page, perPage, total)
	offset = (meta.Page - 1) * perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT p.id,p.account_id,COALESCE(a.username,''),p.realm_key,p.request_type,p.status,p.player_note,p.staff_note,p.handled_by,p.created_at,p.updated_at,p.completed_at"+base+" ORDER BY p.id DESC LIMIT ? OFFSET ?", append(args, perPage, offset)...)
	if err != nil {
		problem(w, 500, "Could not load privacy requests")
		return
	}
	defer rows.Close()
	out := []privacyRequest{}
	for rows.Next() {
		var x privacyRequest
		if rows.Scan(&x.ID, &x.AccountID, &x.Username, &x.RealmKey, &x.Type, &x.Status, &x.PlayerNote, &x.StaffNote, &x.HandledBy, &x.Created, &x.Updated, &x.Completed) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"requests": out, "pagination": meta})
}

func (s *Server) adminPrivacyRequestUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "players")
	if !ok {
		problem(w, 403, "Player management access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid request")
		return
	}
	var in struct{ Status, Note string }
	if !decode(w, r, &in) {
		return
	}
	if in.Status != "processing" && in.Status != "completed" && in.Status != "rejected" {
		problem(w, 422, "Invalid status")
		return
	}
	if len(in.Note) > 500 {
		problem(w, 422, "Note is too long")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.privacy {
			if s.mock.privacy[i].ID == id {
				s.mock.privacy[i].Status = in.Status
				s.mock.privacy[i].StaffNote = strings.TrimSpace(in.Note)
				s.mock.privacy[i].HandledBy = a.ID
				s.mock.privacy[i].Updated = time.Now()
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, 404, "Request not found")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_privacy_requests SET status=?,staff_note=?,handled_by=?,completed_at=IF(?='completed',NOW(),NULL) WHERE id=?", in.Status, strings.TrimSpace(in.Note), a.ID, in.Status, id)
	if err != nil {
		problem(w, 500, "Could not update privacy request")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, 404, "Request not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'privacy.update',?,?)", a.ID, strconv.FormatUint(id, 10), in.Status+": "+strings.TrimSpace(in.Note))
	jsonOut(w, 200, map[string]bool{"ok": true})
}
