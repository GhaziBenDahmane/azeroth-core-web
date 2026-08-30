package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var rolePermissions = map[string][]string{
	"support":         {"support", "monitoring"},
	"shop_manager":    {"commerce"},
	"moderator":       {"support", "monitoring", "players", "moderation", "audit"},
	"content_manager": {"content"},
	"realm_operator":  {"monitoring", "realm", "console"},
	"administrator":   {"overview", "monitoring", "players", "moderation", "audit", "commerce", "content", "realm", "settings", "support", "admin"},
}

func roleLabel(role string) string {
	switch role {
	case "shop_manager":
		return "Shop manager"
	case "support":
		return "Support"
	case "moderator":
		return "Moderator"
	case "content_manager":
		return "Content manager"
	case "realm_operator":
		return "Realm operator"
	case "administrator":
		return "Administrator"
	default:
		return "Player"
	}
}

func (s *Server) effectiveStaff(ctx context.Context, a account) (string, []string) {
	if !s.c.MockMode && s.s != nil {
		var role, custom string
		if err := s.s.Auth.QueryRowContext(ctx, "SELECT role,permissions_json FROM portal_staff_roles WHERE account_id=? AND realm_key IN (?, '*') AND (expires_at IS NULL OR expires_at>NOW()) ORDER BY (realm_key=?) DESC LIMIT 1", a.ID, s.c.RealmKey, s.c.RealmKey).Scan(&role, &custom); err == nil {
			permissions := append([]string(nil), rolePermissions[role]...)
			if custom != "" {
				var requested []string
				if json.Unmarshal([]byte(custom), &requested) == nil {
					permissions = validStaffPermissions(requested)
				}
			}
			if role == "administrator" && s.c.EnableGMConsole {
				permissions = append(permissions, "console")
			}
			return roleLabel(role), permissions
		}
	}
	return staffRole(a, s.c), s.staffPermissionsFor(a.GMLevel, a.Username)
}

func (s *Server) adminStaff(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "admin")
	if !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT sr.account_id,a.username,sr.role,sr.realm_key,sr.expires_at,sr.permissions_json,COALESCE(g.username,'SYSTEM'),sr.updated_at FROM portal_staff_roles sr JOIN %s.account a ON a.id=sr.account_id LEFT JOIN %s.account g ON g.id=sr.granted_by WHERE sr.realm_key IN (?, '*') ORDER BY a.username,sr.realm_key`, s.c.AuthDB, s.c.AuthDB), s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load staff roles")
			return
		}
		defer rows.Close()
		type staffEntry struct {
			AccountID   uint32     `json:"accountId"`
			Username    string     `json:"username"`
			Role        string     `json:"role"`
			RealmKey    string     `json:"realmKey"`
			ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
			Permissions []string   `json:"permissions,omitempty"`
			GrantedBy   string     `json:"grantedBy"`
			Updated     time.Time  `json:"updated"`
		}
		items := []staffEntry{}
		for rows.Next() {
			var item staffEntry
			var custom string
			if rows.Scan(&item.AccountID, &item.Username, &item.Role, &item.RealmKey, &item.ExpiresAt, &custom, &item.GrantedBy, &item.Updated) == nil {
				if custom != "" {
					_ = json.Unmarshal([]byte(custom), &item.Permissions)
				}
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"staff": items})
		return
	}
	var in struct {
		Username    string
		Role        string
		RealmKey    string
		ExpiresAt   *time.Time
		Permissions []string
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.RealmKey = strings.TrimSpace(in.RealmKey)
	if in.RealmKey == "" {
		in.RealmKey = s.c.RealmKey
	}
	if _, valid := rolePermissions[in.Role]; !valid || in.Username == "" {
		problem(w, http.StatusUnprocessableEntity, "Choose a valid account and staff role")
		return
	}
	if in.RealmKey != "*" && in.RealmKey != s.c.RealmKey {
		problem(w, http.StatusUnprocessableEntity, "Role scope must be this realm or all realms")
		return
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		problem(w, http.StatusUnprocessableEntity, "Temporary access must expire in the future")
		return
	}
	permissions := validStaffPermissions(in.Permissions)
	customJSON := ""
	if len(in.Permissions) > 0 {
		encoded, _ := json.Marshal(permissions)
		customJSON = string(encoded)
	}
	var accountID uint32
	if err := s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf("SELECT id FROM %s.account WHERE username=?", s.c.AuthDB), in.Username).Scan(&accountID); err != nil {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	if _, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_staff_roles(account_id,role,realm_key,expires_at,permissions_json,granted_by) VALUES(?,?,?,?,?,?) ON DUPLICATE KEY UPDATE role=VALUES(role),expires_at=VALUES(expires_at),permissions_json=VALUES(permissions_json),granted_by=VALUES(granted_by)", accountID, in.Role, in.RealmKey, in.ExpiresAt, customJSON, actor.ID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not save staff role")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'staff.role',?,?)", actor.ID, in.Username, "Assigned "+in.Role+" scope="+in.RealmKey)
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminStaffDelete(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "admin")
	if !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || uint32(id) == actor.ID {
		problem(w, http.StatusUnprocessableEntity, "You cannot remove your own role")
		return
	}
	realmKey := strings.TrimSpace(r.URL.Query().Get("realm"))
	if realmKey == "" {
		realmKey = s.c.RealmKey
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "DELETE FROM portal_staff_roles WHERE account_id=? AND realm_key=?", id, realmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not remove staff role")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Staff role not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'staff.remove',?,?)", actor.ID, strconv.FormatUint(id, 10), "Removed explicit staff role")
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func validStaffPermissions(requested []string) []string {
	allowed := map[string]bool{"overview": true, "monitoring": true, "players": true, "moderation": true, "audit": true, "commerce": true, "content": true, "realm": true, "settings": true, "support": true, "admin": true, "console": true}
	out := []string{}
	seen := map[string]bool{}
	for _, permission := range requested {
		permission = strings.TrimSpace(permission)
		if allowed[permission] && !seen[permission] {
			out = append(out, permission)
			seen[permission] = true
		}
	}
	return out
}
