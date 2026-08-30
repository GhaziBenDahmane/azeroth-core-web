package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type investigationMatch struct {
	AccountID     uint32    `json:"accountId"`
	Username      string    `json:"username"`
	SharedNetwork bool      `json:"sharedNetwork"`
	SharedDevice  bool      `json:"sharedDevice"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
}

type investigationEvidence struct {
	ID            uint64    `json:"id"`
	CaseReference string    `json:"caseReference,omitempty"`
	Note          string    `json:"note"`
	EvidenceURL   string    `json:"evidenceUrl,omitempty"`
	Actor         string    `json:"actor"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (s *Server) investigationRetentionDays() int {
	if s.c.AuditIPRetentionDays > 0 {
		return s.c.AuditIPRetentionDays
	}
	return 30
}

func (s *Server) adminInvestigationPolicy(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "moderation"); !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{
		"networkRetentionDays":  s.investigationRetentionDays(),
		"evidenceRetentionDays": s.c.AuditRetentionDays,
		"access":                "Moderator or administrator permission plus a recent step-up authentication is required. Every lookup and evidence change is audited.",
		"disclosure":            "Results show correlation signals only. Raw IP addresses, session tokens, and complete browser fingerprints are never returned.",
	})
}

func (s *Server) adminInvestigationSearch(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "moderation")
	if !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	if !s.stepUpValid(r) {
		problem(w, http.StatusUnauthorized, "Recent authentication required")
		return
	}
	var in struct {
		Account string `json:"account"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Account, in.Reason = strings.ToUpper(strings.TrimSpace(in.Account)), strings.TrimSpace(in.Reason)
	if len(in.Account) < 2 || len(in.Account) > 32 || len(in.Reason) < 10 || len(in.Reason) > 500 {
		problem(w, http.StatusUnprocessableEntity, "An account and a 10–500 character investigation reason are required")
		return
	}
	retention := s.investigationRetentionDays()
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{
			"target":               map[string]any{"accountId": 1, "username": in.Account},
			"matches":              []investigationMatch{{AccountID: 2, Username: "HELPER", SharedNetwork: true, LastSeenAt: time.Now().Add(-2 * time.Hour)}},
			"evidence":             []investigationEvidence{},
			"networkRetentionDays": retention,
			"requestId":            RequestID(r.Context()),
		})
		return
	}
	var targetID uint32
	var username string
	accountQuery := fmt.Sprintf("SELECT id,username FROM `%s`.account WHERE username=? LIMIT 1", s.c.AuthDB)
	if err := s.s.Auth.QueryRowContext(r.Context(), accountQuery, in.Account).Scan(&targetID, &username); err != nil {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	matchQuery := fmt.Sprintf(`SELECT other.id,other.username,
		MAX(candidate.ip_address<>'' AND candidate.ip_address=subject.ip_address),
		MAX(candidate.user_agent<>'' AND candidate.user_agent=subject.user_agent),MAX(candidate.last_seen_at)
		FROM portal_sessions subject JOIN portal_sessions candidate
		 ON candidate.account_id<>subject.account_id
		 AND ((candidate.ip_address<>'' AND candidate.ip_address=subject.ip_address)
		  OR (candidate.user_agent<>'' AND candidate.user_agent=subject.user_agent))
		JOIN %s.account other ON other.id=candidate.account_id
		WHERE subject.account_id=? AND subject.last_seen_at>=TIMESTAMPADD(DAY,-?,NOW())
		 AND candidate.last_seen_at>=TIMESTAMPADD(DAY,-?,NOW())
		GROUP BY other.id,other.username ORDER BY MAX(candidate.last_seen_at) DESC LIMIT 100`, s.c.AuthDB)
	rows, err := s.s.Auth.QueryContext(r.Context(), matchQuery, targetID, retention, retention)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load investigation signals")
		return
	}
	matches := []investigationMatch{}
	for rows.Next() {
		var match investigationMatch
		if rows.Scan(&match.AccountID, &match.Username, &match.SharedNetwork, &match.SharedDevice, &match.LastSeenAt) == nil {
			matches = append(matches, match)
		}
	}
	rows.Close()
	evidenceRows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT e.id,e.case_reference,e.note,e.evidence_url,COALESCE(a.username,'SYSTEM'),e.created_at FROM portal_moderation_evidence e LEFT JOIN %s.account a ON a.id=e.actor_account_id WHERE e.realm_key=? AND e.target_account_id=? ORDER BY e.id DESC LIMIT 100`, s.c.AuthDB), s.c.RealmKey, targetID)
	evidence := []investigationEvidence{}
	if err == nil {
		defer evidenceRows.Close()
		for evidenceRows.Next() {
			var item investigationEvidence
			if evidenceRows.Scan(&item.ID, &item.CaseReference, &item.Note, &item.EvidenceURL, &item.Actor, &item.CreatedAt) == nil {
				evidence = append(evidence, item)
			}
		}
	}
	metadata, _ := json.Marshal(map[string]any{"reason": in.Reason, "matches": len(matches), "retentionDays": retention})
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id,ip_address,user_agent,metadata_json) VALUES(?,'moderation.investigate',?,'Privacy-controlled account correlation lookup',?,?,?,?,?)`, actor.ID, username, s.c.RealmKey, RequestID(r.Context()), s.clientIP(r), truncate(r.UserAgent(), 500), metadata)
	jsonOut(w, http.StatusOK, map[string]any{"target": map[string]any{"accountId": targetID, "username": username}, "matches": matches, "evidence": evidence, "networkRetentionDays": retention, "requestId": RequestID(r.Context())})
}

func validEvidenceURL(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.HasPrefix(raw, "/media/") {
		return !strings.Contains(raw, "..")
	}
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func (s *Server) adminInvestigationEvidence(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "moderation")
	if !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	if !s.stepUpValid(r) {
		problem(w, http.StatusUnauthorized, "Recent authentication required")
		return
	}
	var in struct {
		AccountID     uint32 `json:"accountId"`
		CaseReference string `json:"caseReference"`
		Note          string `json:"note"`
		EvidenceURL   string `json:"evidenceUrl"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.CaseReference, in.Note, in.EvidenceURL = strings.TrimSpace(in.CaseReference), strings.TrimSpace(in.Note), strings.TrimSpace(in.EvidenceURL)
	if in.AccountID == 0 || len(in.CaseReference) > 80 || len(in.Note) < 10 || len(in.Note) > 4000 || len(in.EvidenceURL) > 500 || !validEvidenceURL(in.EvidenceURL) {
		problem(w, http.StatusUnprocessableEntity, "Valid target, note, case reference, and HTTPS or managed-media evidence URL required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": 1, "requestId": RequestID(r.Context())})
		return
	}
	var exists uint32
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.account WHERE id=?", s.c.AuthDB)
	if s.s.Auth.QueryRowContext(r.Context(), query, in.AccountID).Scan(&exists) != nil || exists == 0 {
		problem(w, http.StatusNotFound, "Account not found")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_moderation_evidence(realm_key,target_account_id,case_reference,note,evidence_url,actor_account_id) VALUES(?,?,?,?,?,?)`, s.c.RealmKey, in.AccountID, in.CaseReference, in.Note, in.EvidenceURL, actor.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not attach evidence")
		return
	}
	id, _ := result.LastInsertId()
	metadata, _ := json.Marshal(map[string]any{"evidenceId": id, "caseReference": in.CaseReference, "hasURL": in.EvidenceURL != ""})
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id,ip_address,user_agent,metadata_json) VALUES(?,'moderation.evidence',?,'Evidence attached to investigation',?,?,?,?,?)`, actor.ID, fmt.Sprint(in.AccountID), s.c.RealmKey, RequestID(r.Context()), s.clientIP(r), truncate(r.UserAgent(), 500), metadata)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id, "requestId": RequestID(r.Context())})
}
