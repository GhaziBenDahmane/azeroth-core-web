package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type transferRequest struct {
	ID               uint64     `json:"id"`
	AccountID        uint32     `json:"accountId,omitempty"`
	Username         string     `json:"username,omitempty"`
	SourceRealm      string     `json:"sourceRealm"`
	CharacterName    string     `json:"characterName"`
	SourceProfileURL string     `json:"sourceProfileUrl"`
	PlayerNote       string     `json:"playerNote"`
	Status           string     `json:"status"`
	StaffNote        string     `json:"staffNote,omitempty"`
	HandledBy        uint32     `json:"handledBy,omitempty"`
	Handler          string     `json:"handler,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
}

func (s *Server) transfers(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		username, ok := s.mockUser(r)
		if !ok {
			problem(w, 401, "Sign in required")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		out := []transferRequest{}
		for _, item := range s.mock.transfers {
			if item.AccountID == 1 && username != "" {
				item.Username = ""
				item.HandledBy = 0
				item.Handler = ""
				out = append(out, item)
			}
		}
		jsonOut(w, 200, map[string]any{"requests": out})
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,source_realm,character_name,source_profile_url,player_note,status,staff_note,created_at,updated_at,completed_at FROM portal_transfer_requests WHERE account_id=? AND realm_key=? ORDER BY id DESC`, a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not load transfer requests")
		return
	}
	defer rows.Close()
	out := []transferRequest{}
	for rows.Next() {
		var item transferRequest
		if rows.Scan(&item.ID, &item.SourceRealm, &item.CharacterName, &item.SourceProfileURL, &item.PlayerNote, &item.Status, &item.StaffNote, &item.CreatedAt, &item.UpdatedAt, &item.CompletedAt) == nil {
			out = append(out, item)
		}
	}
	jsonOut(w, 200, map[string]any{"requests": out})
}

func validateTransfer(item *transferRequest) error {
	item.SourceRealm = strings.TrimSpace(item.SourceRealm)
	item.CharacterName = strings.TrimSpace(item.CharacterName)
	item.SourceProfileURL = strings.TrimSpace(item.SourceProfileURL)
	item.PlayerNote = strings.TrimSpace(item.PlayerNote)
	if len(item.SourceRealm) < 2 || len(item.SourceRealm) > 120 || len(item.CharacterName) < 2 || len(item.CharacterName) > 32 || len(item.PlayerNote) > 1000 {
		return fmt.Errorf("source realm and character are required")
	}
	if item.SourceProfileURL != "" {
		parsed, err := url.ParseRequestURI(item.SourceProfileURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("source profile must be an HTTP(S) URL")
		}
	}
	return nil
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request) {
	var a account
	if s.c.MockMode {
		username, ok := s.mockUser(r)
		if !ok {
			problem(w, 401, "Sign in required")
			return
		}
		a = account{ID: 1, Username: username}
	} else {
		var err error
		a, err = s.auth(r)
		if err != nil {
			problem(w, 401, "Sign in required")
			return
		}
	}
	var item transferRequest
	if !decode(w, r, &item) {
		return
	}
	if err := validateTransfer(&item); err != nil {
		problem(w, 422, err.Error())
		return
	}
	item.AccountID = a.ID
	item.Username = a.Username
	item.Status = "submitted"
	item.CreatedAt = time.Now()
	item.UpdatedAt = item.CreatedAt
	if s.c.MockMode {
		s.mock.mu.Lock()
		item.ID = uint64(len(s.mock.transfers) + 1)
		s.mock.transfers = append([]transferRequest{item}, s.mock.transfers...)
		s.mock.mu.Unlock()
	} else {
		var active int
		if s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_transfer_requests WHERE account_id=? AND realm_key=? AND character_name=? AND status IN ('submitted','reviewing','approved')", a.ID, s.c.RealmKey, item.CharacterName).Scan(&active) != nil {
			problem(w, 500, "Could not validate transfer request")
			return
		}
		if active > 0 {
			problem(w, 409, "An active transfer request already exists for this character")
			return
		}
		res, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_transfer_requests(account_id,realm_key,source_realm,character_name,source_profile_url,player_note) VALUES(?,?,?,?,?,?)`, a.ID, s.c.RealmKey, item.SourceRealm, item.CharacterName, item.SourceProfileURL, item.PlayerNote)
		if err != nil {
			problem(w, 500, "Could not submit transfer request")
			return
		}
		id, _ := res.LastInsertId()
		item.ID = uint64(id)
	}
	s.notifyDiscordAsync("Character transfer request", "Request **#%d** from **%s** for **%s** on **%s**", item.ID, a.Username, item.CharacterName, item.SourceRealm)
	jsonOut(w, 201, item)
}

func (s *Server) adminTransfers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "players"); !ok {
		problem(w, 403, "Player management permission required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	status, search := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("q"))
	if status != "" && !map[string]bool{"submitted": true, "reviewing": true, "approved": true, "rejected": true, "completed": true}[status] || len(search) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Invalid transfer filters")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		items := append([]transferRequest(nil), s.mock.transfers...)
		s.mock.mu.Unlock()
		filtered := items[:0]
		for _, item := range items {
			if status != "" && item.Status != status || search != "" && !strings.Contains(strings.ToLower(item.Username+" "+item.CharacterName+" "+item.SourceRealm), strings.ToLower(search)) {
				continue
			}
			filtered = append(filtered, item)
		}
		filtered, meta := slicePage(filtered, page, perPage)
		jsonOut(w, 200, map[string]any{"requests": filtered, "pagination": meta})
		return
	}
	where, args := " WHERE t.realm_key=?", []any{s.c.RealmKey}
	if status != "" {
		where += " AND t.status=?"
		args = append(args, status)
	}
	if search != "" {
		where += " AND (a.username LIKE ? OR t.character_name LIKE ? OR t.source_realm LIKE ?)"
		pattern := likePattern(search)
		args = append(args, pattern, pattern, pattern)
	}
	base := fmt.Sprintf(` FROM portal_transfer_requests t JOIN %s.account a ON a.id=t.account_id LEFT JOIN %s.account h ON h.id=t.handled_by`, s.c.AuthDB, s.c.AuthDB) + where
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*)"+base, args...).Scan(&total); err != nil {
		problem(w, 500, "Could not count transfer queue")
		return
	}
	meta := paginationMeta(page, perPage, total)
	offset = (meta.Page - 1) * perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT t.id,t.account_id,a.username,t.source_realm,t.character_name,t.source_profile_url,t.player_note,t.status,t.staff_note,t.handled_by,COALESCE(h.username,''),t.created_at,t.updated_at,t.completed_at`+base+` ORDER BY FIELD(t.status,'submitted','reviewing','approved','rejected','completed'),t.id DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
	if err != nil {
		problem(w, 500, "Could not load transfer queue")
		return
	}
	defer rows.Close()
	items := []transferRequest{}
	for rows.Next() {
		var item transferRequest
		if rows.Scan(&item.ID, &item.AccountID, &item.Username, &item.SourceRealm, &item.CharacterName, &item.SourceProfileURL, &item.PlayerNote, &item.Status, &item.StaffNote, &item.HandledBy, &item.Handler, &item.CreatedAt, &item.UpdatedAt, &item.CompletedAt) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, 200, map[string]any{"requests": items, "pagination": meta})
}

func (s *Server) adminTransferUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "players")
	if !ok {
		problem(w, 403, "Player management permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid request")
		return
	}
	var in struct{ Status, StaffNote string }
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.StaffNote = strings.TrimSpace(in.StaffNote)
	if !map[string]bool{"submitted": true, "reviewing": true, "approved": true, "rejected": true, "completed": true}[in.Status] || len(in.StaffNote) > 1000 {
		problem(w, 422, "Invalid transfer status or note")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.transfers {
			if s.mock.transfers[i].ID == id {
				s.mock.transfers[i].Status = in.Status
				s.mock.transfers[i].StaffNote = in.StaffNote
				s.mock.transfers[i].HandledBy = a.ID
				s.mock.transfers[i].Handler = a.Username
				s.mock.transfers[i].UpdatedAt = time.Now()
				if in.Status == "completed" {
					now := time.Now()
					s.mock.transfers[i].CompletedAt = &now
				}
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, 404, "Request not found")
		return
	}
	var target uint32
	var character string
	if s.s.Auth.QueryRowContext(r.Context(), "SELECT account_id,character_name FROM portal_transfer_requests WHERE id=? AND realm_key=?", id, s.c.RealmKey).Scan(&target, &character) != nil {
		problem(w, 404, "Request not found")
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_transfer_requests SET status=?,staff_note=?,handled_by=?,completed_at=CASE WHEN ?='completed' THEN NOW() ELSE completed_at END WHERE id=? AND realm_key=?`, in.Status, in.StaffNote, a.ID, in.Status, id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not update transfer request")
		return
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		problem(w, 404, "Request not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'transfer.update',?,?)", a.ID, strconv.FormatUint(id, 10), in.Status+": "+in.StaffNote)
	s.notifyAccount(r.Context(), target, "transfer", "Character transfer updated", character+" is now "+in.Status+".", "/account/transfers")
	jsonOut(w, 200, map[string]bool{"ok": true})
}
