package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type sanctionCase struct {
	ID            uint64     `json:"id"`
	AccountID     uint32     `json:"accountId,omitempty"`
	Username      string     `json:"username,omitempty"`
	CharacterName string     `json:"characterName,omitempty"`
	Type          string     `json:"type"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	StartsAt      time.Time  `json:"startsAt"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
	LiftedAt      *time.Time `json:"liftedAt,omitempty"`
	Appeal        any        `json:"appeal,omitempty"`
}

func sanctionExpiry(duration string, started time.Time) *time.Time {
	if duration == "" || duration == "-1" {
		return nil
	}
	units := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour, 'y': 365 * 24 * time.Hour}
	total := time.Duration(0)
	number := 0
	for i := 0; i < len(duration); i++ {
		if duration[i] >= '0' && duration[i] <= '9' {
			number = number*10 + int(duration[i]-'0')
			continue
		}
		unit, ok := units[duration[i]]
		if !ok || number == 0 {
			return nil
		}
		total += time.Duration(number) * unit
		number = 0
	}
	if total <= 0 {
		return nil
	}
	expires := started.Add(total)
	return &expires
}

func (s *Server) recordSanction(r *http.Request, logID int64, actor, accountID uint32, characterName, action, duration, reason string) {
	if s.c.MockMode || accountID == 0 {
		return
	}
	now := time.Now().UTC()
	switch action {
	case "ban", "mute", "ip_ban":
		_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_sanctions(realm_key,moderation_log_id,account_id,character_name,sanction_type,reason,status,starts_at,expires_at,created_by) VALUES(?,?,?,?,?,?,'active',?,?,?)`, s.c.RealmKey, logID, accountID, characterName, action, reason, now, sanctionExpiry(duration, now), actor)
	case "unban":
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_sanctions SET status='lifted',lifted_at=NOW() WHERE realm_key=? AND account_id=? AND sanction_type='ban' AND status='active'`, s.c.RealmKey, accountID)
	case "unmute":
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_sanctions SET status='lifted',lifted_at=NOW() WHERE realm_key=? AND account_id=? AND sanction_type='mute' AND status='active'`, s.c.RealmKey, accountID)
	}
}

func (s *Server) playerSanctions(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"sanctions": []any{}})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT s.id,s.character_name,s.sanction_type,s.reason,s.status,s.starts_at,s.expires_at,s.lifted_at,
		a.id,a.status,a.message,a.staff_response,a.created_at,a.reviewed_at FROM portal_sanctions s LEFT JOIN portal_sanction_appeals a ON a.sanction_id=s.id AND a.account_id=? WHERE s.account_id=? AND s.realm_key=? ORDER BY s.id DESC LIMIT 100`, a.ID, a.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load account sanctions")
		return
	}
	defer rows.Close()
	out := []sanctionCase{}
	for rows.Next() {
		var item sanctionCase
		var expires, lifted, appealCreated, reviewed sql.NullTime
		var appealID sql.NullInt64
		var appealStatus, message, response sql.NullString
		if rows.Scan(&item.ID, &item.CharacterName, &item.Type, &item.Reason, &item.Status, &item.StartsAt, &expires, &lifted, &appealID, &appealStatus, &message, &response, &appealCreated, &reviewed) != nil {
			continue
		}
		if expires.Valid {
			item.ExpiresAt = &expires.Time
		}
		if lifted.Valid {
			item.LiftedAt = &lifted.Time
		}
		if appealID.Valid {
			item.Appeal = map[string]any{"id": appealID.Int64, "status": appealStatus.String, "message": message.String, "staffResponse": response.String, "createdAt": appealCreated.Time, "reviewedAt": nullableTime(reviewed)}
		}
		out = append(out, item)
	}
	jsonOut(w, http.StatusOK, map[string]any{"sanctions": out})
}

func nullableTime(value sql.NullTime) any {
	if value.Valid {
		return value.Time
	}
	return nil
}

func (s *Server) createSanctionAppeal(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid sanction")
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Message) < 20 || len(in.Message) > 4000 {
		problem(w, http.StatusUnprocessableEntity, "Appeal must be 20–4,000 characters")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": 1})
		return
	}
	var exists uint32
	if s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_sanctions WHERE id=? AND account_id=? AND realm_key=?`, id, a.ID, s.c.RealmKey).Scan(&exists) != nil || exists == 0 {
		problem(w, http.StatusNotFound, "Sanction not found")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_sanction_appeals(sanction_id,account_id,message) VALUES(?,?,?)`, id, a.ID, in.Message)
	if err != nil {
		problem(w, http.StatusConflict, "An appeal already exists for this sanction")
		return
	}
	appealID, _ := result.LastInsertId()
	s.notifyDiscordAsync("Sanction appeal", "**%s** submitted appeal #%d for sanction #%d.", a.Username, appealID, id)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": appealID})
}

func (s *Server) adminSanctionAppeals(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "moderation"); !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"appeals": []any{}})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	where, args := "s.realm_key=?", []any{s.c.RealmKey}
	if status != "" {
		where += " AND ap.status=?"
		args = append(args, status)
	}
	query := fmt.Sprintf(`SELECT ap.id,ap.sanction_id,a.username,s.sanction_type,s.reason,ap.message,ap.status,COALESCE(ap.staff_response,''),ap.created_at,ap.reviewed_at FROM portal_sanction_appeals ap JOIN portal_sanctions s ON s.id=ap.sanction_id JOIN %s.account a ON a.id=ap.account_id WHERE %s ORDER BY ap.id DESC LIMIT 100`, s.c.AuthDB, where)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, args...)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load sanction appeals")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, sanctionID uint64
		var username, kind, reason, message, state, response string
		var created time.Time
		var reviewed sql.NullTime
		if rows.Scan(&id, &sanctionID, &username, &kind, &reason, &message, &state, &response, &created, &reviewed) == nil {
			out = append(out, map[string]any{"id": id, "sanctionId": sanctionID, "username": username, "type": kind, "reason": reason, "message": message, "status": state, "staffResponse": response, "createdAt": created, "reviewedAt": nullableTime(reviewed)})
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"appeals": out})
}

func (s *Server) adminSanctionAppeal(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "moderation")
	if !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid appeal")
		return
	}
	var in struct{ Status, Response, InternalNote, EvidenceURL string }
	if !decode(w, r, &in) {
		return
	}
	in.Status, in.Response, in.InternalNote, in.EvidenceURL = strings.TrimSpace(in.Status), strings.TrimSpace(in.Response), strings.TrimSpace(in.InternalNote), strings.TrimSpace(in.EvidenceURL)
	if in.Status != "reviewing" && in.Status != "accepted" && in.Status != "declined" || len(in.Response) > 4000 || len(in.InternalNote) > 4000 || len(in.EvidenceURL) > 500 {
		problem(w, http.StatusUnprocessableEntity, "Invalid appeal decision")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var sanctionID, accountID uint64
	if err = tx.QueryRowContext(r.Context(), `SELECT sanction_id,account_id FROM portal_sanction_appeals WHERE id=? FOR UPDATE`, id).Scan(&sanctionID, &accountID); err != nil {
		problem(w, http.StatusNotFound, "Appeal not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal_sanction_appeals SET status=?,staff_response=?,reviewed_by=?,reviewed_at=NOW() WHERE id=?`, in.Status, in.Response, actor.ID, id); err == nil && in.InternalNote != "" {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_sanction_notes(sanction_id,actor_account_id,body,evidence_url) VALUES(?,?,?,?)`, sanctionID, actor.ID, in.InternalNote, in.EvidenceURL)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not review appeal")
		return
	}
	s.notifyAccount(r.Context(), uint32(accountID), "moderation", "Your sanction appeal was reviewed", in.Response, "/account/support")
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
