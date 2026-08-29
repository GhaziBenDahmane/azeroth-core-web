package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type auditEntry struct {
	Actor, Target, Action, Reason, Status string
	Created                               time.Time
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "audit"); !ok {
		problem(w, http.StatusForbidden, "Audit access required")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		out := []auditEntry{
			{Actor: "DEMO", Target: "Product 1", Action: "product.update", Reason: "Featured sale and stock updated", Status: "executed", Created: time.Now().Add(-35 * time.Minute)},
			{Actor: "DEMO", Target: "Welcome to Azeroth", Action: "news.publish", Reason: "Homepage feature", Status: "executed", Created: time.Now().Add(-2 * time.Hour)},
		}
		for _, value := range s.mock.moderation {
			out = append(out, auditEntry{Actor: fmt.Sprint(value["Actor"]), Target: fmt.Sprint(value["Target"]), Action: fmt.Sprint(value["Action"]), Reason: fmt.Sprint(value["Reason"]), Status: fmt.Sprint(value["Status"]), Created: time.Now()})
		}
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]any{"entries": filterAuditEntries(out, r)})
		return
	}
	query := fmt.Sprintf(`SELECT actor,target,action,reason,status,created FROM (
		SELECT COALESCE(a.username,'SYSTEM') actor,x.target,x.action,x.details reason,'executed' status,x.created_at created
		FROM portal_admin_audit x LEFT JOIN %s.account a ON a.id=x.actor_account_id
		UNION ALL
		SELECT COALESCE(a.username,'SYSTEM'),m.target,m.action,m.reason,m.status,m.created_at
		FROM portal_moderation_log m LEFT JOIN %s.account a ON a.id=m.actor_account_id WHERE m.realm_key=?
		UNION ALL
		SELECT COALESCE(a.username,'SYSTEM'),c.realm_key,'console.command',c.command,IF(c.success=1,'executed','failed'),c.created_at
		FROM portal_command_log c LEFT JOIN %s.account a ON a.id=c.actor_account_id WHERE c.realm_key=?
		UNION ALL
		SELECT COALESCE(a.username,'SYSTEM'),CAST(t.id AS CHAR),'support.update',t.response,t.status,t.updated_at
		FROM portal_support_tickets t LEFT JOIN %s.account a ON a.id=t.gm_account_id WHERE t.realm_key=? AND t.gm_account_id>0
		UNION ALL
		SELECT COALESCE(a.username,'SYSTEM'),target.username,'credits.grant',l.reason,'executed',l.created_at
		FROM portal_credit_ledger l LEFT JOIN %s.account a ON a.id=l.actor_account_id JOIN %s.account target ON target.id=l.target_account_id
	) audit ORDER BY created DESC LIMIT 500`, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, s.c.RealmKey, s.c.RealmKey, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load audit history")
		return
	}
	defer rows.Close()
	out := []auditEntry{}
	for rows.Next() {
		var entry auditEntry
		if rows.Scan(&entry.Actor, &entry.Target, &entry.Action, &entry.Reason, &entry.Status, &entry.Created) == nil {
			out = append(out, entry)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"entries": filterAuditEntries(out, r)})
}

func filterAuditEntries(entries []auditEntry, r *http.Request) []auditEntry {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	from, _ := time.Parse("2006-01-02", r.URL.Query().Get("from"))
	to, _ := time.Parse("2006-01-02", r.URL.Query().Get("to"))
	if !to.IsZero() {
		to = to.Add(24*time.Hour - time.Nanosecond)
	}
	out := make([]auditEntry, 0, len(entries))
	for _, entry := range entries {
		haystack := strings.ToLower(entry.Actor + " " + entry.Target + " " + entry.Action + " " + entry.Reason)
		if status != "" && strings.ToLower(entry.Status) != status || q != "" && !strings.Contains(haystack, q) || !from.IsZero() && entry.Created.Before(from) || !to.IsZero() && entry.Created.After(to) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
