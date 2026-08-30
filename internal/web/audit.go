package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type auditEntry struct {
	EventID   string          `json:"eventId"`
	Source    string          `json:"source"`
	Actor     string          `json:"actor"`
	Target    string          `json:"target"`
	Action    string          `json:"action"`
	Reason    string          `json:"reason"`
	Status    string          `json:"status"`
	Realm     string          `json:"realm,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
	IPAddress string          `json:"ipAddress,omitempty"`
	UserAgent string          `json:"userAgent,omitempty"`
	Before    json.RawMessage `json:"before,omitempty"`
	After     json.RawMessage `json:"after,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	Created   time.Time       `json:"created"`
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "audit"); !ok {
		problem(w, http.StatusForbidden, "Audit access required")
		return
	}
	entries, err := s.loadAuditEntries(r, 10000)
	if err != nil {
		slog.Error("load admin audit", "realm", s.c.RealmKey, "error", err)
		problem(w, http.StatusInternalServerError, "Could not load audit history")
		return
	}
	entries = filterAuditEntries(entries, r)
	page, perPage, _ := requestPage(r, 50, 100)
	entries, meta := slicePage(entries, page, perPage)
	jsonOut(w, http.StatusOK, map[string]any{"entries": entries, "pagination": meta, "retentionDays": s.c.AuditRetentionDays, "ipRetentionDays": s.c.AuditIPRetentionDays})
}

func (s *Server) adminAuditExport(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "audit"); !ok {
		problem(w, http.StatusForbidden, "Audit access required")
		return
	}
	entries, err := s.loadAuditEntries(r, 10000)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not export audit history")
		return
	}
	entries = filterAuditEntries(entries, r)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="staff-audit.csv"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"Event ID", "Source", "Created", "Realm", "Actor", "Action", "Target", "Status", "Reason", "Request ID", "IP address", "User agent", "Before", "After", "Metadata"})
	for _, entry := range entries {
		_ = writer.Write([]string{entry.EventID, entry.Source, entry.Created.UTC().Format(time.RFC3339), entry.Realm, entry.Actor, entry.Action, entry.Target, entry.Status, entry.Reason, entry.RequestID, entry.IPAddress, entry.UserAgent, string(entry.Before), string(entry.After), string(entry.Metadata)})
	}
	writer.Flush()
}

func (s *Server) loadAuditEntries(r *http.Request, limit int) ([]auditEntry, error) {
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		out := []auditEntry{
			{EventID: "portal-2", Source: "portal", Actor: "DEMO", Target: "Product 1", Action: "product.update", Reason: "Featured sale and stock updated", Status: "executed", Realm: s.c.RealmKey, RequestID: "demo-request-2", Created: time.Now().Add(-35 * time.Minute)},
			{EventID: "portal-1", Source: "portal", Actor: "DEMO", Target: "Welcome to Azeroth", Action: "news.publish", Reason: "Homepage feature", Status: "executed", Realm: s.c.RealmKey, RequestID: "demo-request-1", Created: time.Now().Add(-2 * time.Hour)},
		}
		for index, value := range s.mock.moderation {
			out = append(out, auditEntry{EventID: "moderation-" + strconv.Itoa(index+1), Source: "moderation", Actor: fmt.Sprint(value["Actor"]), Target: fmt.Sprint(value["Target"]), Action: fmt.Sprint(value["Action"]), Reason: fmt.Sprint(value["Reason"]), Status: fmt.Sprint(value["Status"]), Realm: s.c.RealmKey, Created: time.Now()})
		}
		return out, nil
	}
	if limit < 1 || limit > 10000 {
		limit = 500
	}
	type sourceQuery struct {
		name, realm, query string
		args               []any
		structured         bool
	}
	queries := []sourceQuery{
		{"portal", "", fmt.Sprintf(`SELECT x.id,COALESCE(a.username,'SYSTEM'),x.target,x.action,x.details,'executed',x.created_at,x.realm_key,x.request_id,x.ip_address,x.user_agent,x.before_json,x.after_json,x.metadata_json FROM portal_admin_audit x LEFT JOIN %s.account a ON a.id=x.actor_account_id WHERE x.realm_key IN ('',?) ORDER BY x.created_at DESC LIMIT %d`, s.c.AuthDB, limit), []any{s.c.RealmKey}, true},
		{"moderation", s.c.RealmKey, fmt.Sprintf(`SELECT m.id,COALESCE(a.username,'SYSTEM'),m.target,m.action,m.reason,m.status,m.created_at FROM portal_moderation_log m LEFT JOIN %s.account a ON a.id=m.actor_account_id WHERE m.realm_key=? ORDER BY m.created_at DESC LIMIT %d`, s.c.AuthDB, limit), []any{s.c.RealmKey}, false},
		{"console", s.c.RealmKey, fmt.Sprintf(`SELECT c.id,COALESCE(a.username,'SYSTEM'),c.realm_key,'console.command',c.command,IF(c.success=1,'executed','failed'),c.created_at FROM portal_command_log c LEFT JOIN %s.account a ON a.id=c.actor_account_id WHERE c.realm_key=? ORDER BY c.created_at DESC LIMIT %d`, s.c.AuthDB, limit), []any{s.c.RealmKey}, false},
		{"support", s.c.RealmKey, fmt.Sprintf(`SELECT t.id,COALESCE(a.username,'SYSTEM'),CAST(t.id AS CHAR),'support.update',t.response,t.status,t.updated_at FROM portal_support_tickets t LEFT JOIN %s.account a ON a.id=t.gm_account_id WHERE t.realm_key=? AND t.gm_account_id>0 ORDER BY t.updated_at DESC LIMIT %d`, s.c.AuthDB, limit), []any{s.c.RealmKey}, false},
		{"credits", "", fmt.Sprintf(`SELECT l.id,COALESCE(a.username,'SYSTEM'),target.username,'credits.grant',l.reason,'executed',l.created_at FROM portal_credit_ledger l LEFT JOIN %s.account a ON a.id=l.actor_account_id JOIN %s.account target ON target.id=l.target_account_id ORDER BY l.created_at DESC LIMIT %d`, s.c.AuthDB, s.c.AuthDB, limit), nil, false},
	}
	out := []auditEntry{}
	for _, source := range queries {
		rows, err := s.s.Auth.QueryContext(r.Context(), source.query, source.args...)
		if err != nil {
			return nil, fmt.Errorf("%s source: %w", source.name, err)
		}
		for rows.Next() {
			var id uint64
			var entry auditEntry
			if source.structured {
				var before, after, metadata []byte
				err = rows.Scan(&id, &entry.Actor, &entry.Target, &entry.Action, &entry.Reason, &entry.Status, &entry.Created, &entry.Realm, &entry.RequestID, &entry.IPAddress, &entry.UserAgent, &before, &after, &metadata)
				entry.Before, entry.After, entry.Metadata = validJSON(before), validJSON(after), validJSON(metadata)
			} else {
				err = rows.Scan(&id, &entry.Actor, &entry.Target, &entry.Action, &entry.Reason, &entry.Status, &entry.Created)
				entry.Realm = source.realm
			}
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("%s source scan: %w", source.name, err)
			}
			entry.Source = source.name
			entry.EventID = source.name + "-" + strconv.FormatUint(id, 10)
			out = append(out, entry)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("%s source rows: %w", source.name, err)
		}
		rows.Close()
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func validJSON(value []byte) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return nil
	}
	return json.RawMessage(value)
}

func filterAuditEntries(entries []auditEntry, r *http.Request) []auditEntry {
	query := r.URL.Query()
	q := strings.ToLower(strings.TrimSpace(query.Get("q")))
	status := strings.ToLower(strings.TrimSpace(query.Get("status")))
	source := strings.ToLower(strings.TrimSpace(query.Get("source")))
	action := strings.ToLower(strings.TrimSpace(query.Get("action")))
	actor := strings.ToLower(strings.TrimSpace(query.Get("actor")))
	realm := strings.ToLower(strings.TrimSpace(query.Get("realm")))
	from, _ := time.Parse("2006-01-02", query.Get("from"))
	to, _ := time.Parse("2006-01-02", query.Get("to"))
	if !to.IsZero() {
		to = to.Add(24*time.Hour - time.Nanosecond)
	}
	out := make([]auditEntry, 0, len(entries))
	for _, entry := range entries {
		haystack := strings.ToLower(entry.EventID + " " + entry.Actor + " " + entry.Target + " " + entry.Action + " " + entry.Reason + " " + entry.RequestID)
		if status != "" && strings.ToLower(entry.Status) != status || source != "" && strings.ToLower(entry.Source) != source || action != "" && !strings.Contains(strings.ToLower(entry.Action), action) || actor != "" && !strings.Contains(strings.ToLower(entry.Actor), actor) || realm != "" && strings.ToLower(entry.Realm) != realm || q != "" && !strings.Contains(haystack, q) || !from.IsZero() && entry.Created.Before(from) || !to.IsZero() && entry.Created.After(to) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
