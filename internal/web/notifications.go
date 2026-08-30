package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type notification struct {
	ID        uint64     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	ActionURL string     `json:"actionUrl"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
	Created   time.Time  `json:"created"`
}

func (s *Server) notifyAccount(ctx context.Context, accountID uint32, kind, title, message, actionURL string) {
	if accountID == 0 || s.c.MockMode {
		return
	}
	_, _ = s.s.Auth.ExecContext(ctx, `INSERT INTO portal_notifications(account_id,realm_key,kind,title,message,action_url) VALUES(?,?,?,?,?,?)`, accountID, s.c.RealmKey, kind, title, message, actionURL)
}

func (s *Server) notifyAllAccounts(ctx context.Context, kind, title, message, actionURL string) {
	if s.c.MockMode {
		s.mock.mu.Lock()
		s.mock.notifications = append([]notification{{ID: uint64(len(s.mock.notifications) + 1), Kind: kind, Title: title, Message: message, ActionURL: actionURL, Created: time.Now()}}, s.mock.notifications...)
		s.mock.mu.Unlock()
		return
	}
	_, _ = s.s.Auth.ExecContext(ctx, fmt.Sprintf(`INSERT INTO portal_notifications(account_id,realm_key,kind,title,message,action_url) SELECT id,?,?,?,?,? FROM %s.account`, s.c.AuthDB), s.c.RealmKey, kind, title, message, actionURL)
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,kind,title,message,action_url,read_at,created_at FROM portal_notifications WHERE account_id=? AND realm_key=? ORDER BY id DESC LIMIT 100`, a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load notifications")
		return
	}
	defer rows.Close()
	items := []notification{}
	unread := 0
	for rows.Next() {
		var item notification
		if err = rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Message, &item.ActionURL, &item.ReadAt, &item.Created); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read notifications")
			return
		}
		if item.ReadAt == nil {
			unread++
		}
		items = append(items, item)
	}
	jsonOut(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

func (s *Server) notificationRead(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "all" {
		_, err = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_notifications SET read_at=COALESCE(read_at,NOW()) WHERE account_id=? AND realm_key=?", a.ID, s.c.RealmKey)
	} else {
		parsed, parseErr := strconv.ParseUint(id, 10, 64)
		if parseErr != nil {
			problem(w, http.StatusBadRequest, "Invalid notification")
			return
		}
		var result interface{ RowsAffected() (int64, error) }
		result, err = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_notifications SET read_at=COALESCE(read_at,NOW()) WHERE id=? AND account_id=? AND realm_key=?", parsed, a.ID, s.c.RealmKey)
		if err == nil {
			if changed, _ := result.RowsAffected(); changed == 0 {
				problem(w, http.StatusNotFound, "Notification not found")
				return
			}
		}
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update notification")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
