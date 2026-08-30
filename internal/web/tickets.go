package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type supportTicket struct {
	ID            uint64          `json:"id"`
	AccountID     uint32          `json:"accountId,omitempty"`
	Username      string          `json:"username,omitempty"`
	CharacterGUID uint32          `json:"characterGuid,omitempty"`
	Subject       string          `json:"subject"`
	Message       string          `json:"message"`
	Status        string          `json:"status"`
	Response      string          `json:"response,omitempty"`
	GM            string          `json:"gm,omitempty"`
	Category      string          `json:"category"`
	Priority      string          `json:"priority"`
	Tags          string          `json:"tags"`
	AssignedTo    uint32          `json:"assignedTo,omitempty"`
	AssignedName  string          `json:"assignedName,omitempty"`
	DueAt         *time.Time      `json:"dueAt,omitempty"`
	FirstResponse *time.Time      `json:"firstResponseAt,omitempty"`
	ResolvedAt    *time.Time      `json:"resolvedAt,omitempty"`
	Created       time.Time       `json:"created"`
	Updated       time.Time       `json:"updated"`
	Messages      []ticketMessage `json:"messages,omitempty"`
}

type ticketEvent struct {
	ID        uint64    `json:"id"`
	ActorID   uint32    `json:"actorId"`
	Type      string    `json:"type"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"createdAt"`
}

type ticketMessage struct {
	ID         uint64    `json:"id"`
	AuthorID   uint64    `json:"authorId"`
	AuthorRole string    `json:"authorRole"`
	Message    string    `json:"message"`
	Created    time.Time `json:"created"`
}

func (s *Server) attachTicketMessages(r *http.Request, tickets []supportTicket, includeInternal bool) {
	if len(tickets) == 0 {
		return
	}
	args, indexes := make([]any, 0, len(tickets)), make(map[uint64]int, len(tickets))
	for index := range tickets {
		tickets[index].Messages = []ticketMessage{}
		args = append(args, tickets[index].ID)
		indexes[tickets[index].ID] = index
	}
	query := "SELECT ticket_id,id,author_account_id,author_role,message,created_at FROM portal_ticket_messages WHERE ticket_id IN (" + placeholders(len(tickets)) + ")"
	if !includeInternal {
		query += " AND author_role<>'internal'"
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), query+" ORDER BY ticket_id,id", args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var ticketID uint64
		var message ticketMessage
		if rows.Scan(&ticketID, &message.ID, &message.AuthorID, &message.AuthorRole, &message.Message, &message.Created) == nil {
			if index, exists := indexes[ticketID]; exists {
				tickets[index].Messages = append(tickets[index].Messages, message)
			}
		}
	}
}

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	page, perPage, offset := requestPage(r, 20, 50)
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_support_tickets WHERE account_id=? AND realm_key=?", a.ID, s.c.RealmKey).Scan(&total); err != nil {
		problem(w, 500, "Could not count tickets")
		return
	}
	ticketPagination := paginationMeta(page, perPage, total)
	offset = (ticketPagination.Page - 1) * perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,character_guid,subject,message,status,response,category,priority,tags,due_at,first_response_at,resolved_at,created_at,updated_at FROM portal_support_tickets WHERE account_id=? AND realm_key=? ORDER BY id DESC LIMIT ? OFFSET ?", a.ID, s.c.RealmKey, perPage, offset)
	if err != nil {
		problem(w, 500, "Could not load tickets")
		return
	}
	defer rows.Close()
	out := []supportTicket{}
	for rows.Next() {
		var x supportTicket
		if rows.Scan(&x.ID, &x.CharacterGUID, &x.Subject, &x.Message, &x.Status, &x.Response, &x.Category, &x.Priority, &x.Tags, &x.DueAt, &x.FirstResponse, &x.ResolvedAt, &x.Created, &x.Updated) == nil {
			out = append(out, x)
		}
	}
	s.attachTicketMessages(r, out, false)
	jsonOut(w, 200, map[string]any{"tickets": out, "pagination": ticketPagination})
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		CharacterGUID    uint32
		Subject, Message string
		Category         string
	}
	if !decode(w, r, &in) {
		return
	}
	in.Subject, in.Message = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Message)
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	if in.Category == "" {
		in.Category = "general"
	}
	if !validTicketCategory(in.Category) {
		problem(w, 422, "Choose a valid ticket category")
		return
	}
	if len(in.Subject) < 3 || len(in.Subject) > 100 || len(in.Message) < 10 || len(in.Message) > 4000 {
		problem(w, 422, "Subject must be 3–100 characters and message 10–4000 characters")
		return
	}
	if in.CharacterGUID != 0 {
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
		var count int
		if s.s.Characters.QueryRowContext(r.Context(), q, in.CharacterGUID, a.ID).Scan(&count) != nil || count != 1 {
			problem(w, 422, "Choose one of your characters")
			return
		}
	}
	res, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_support_tickets(account_id,character_guid,realm_key,subject,message,response,category,priority,due_at) VALUES(?,?,?,?,?,?,?,'normal',DATE_ADD(NOW(),INTERVAL 72 HOUR))", a.ID, in.CharacterGUID, s.c.RealmKey, in.Subject, in.Message, "", in.Category)
	if err != nil {
		problem(w, 500, "Could not create ticket")
		return
	}
	id, _ := res.LastInsertId()
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message) VALUES(?,?,'player',?)", id, a.ID, in.Message)
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_ticket_events(ticket_id,actor_account_id,event_type,details) VALUES(?,?,'created',?)", id, a.ID, in.Category)
	s.notifyDiscordAsync("New support ticket", "Ticket **#%d** from **%s** on **%s**: %s", id, a.Username, s.c.RealmName, in.Subject)
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) ticketMessage(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid ticket")
		return
	}
	var in struct {
		Message string `json:"message"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Message) < 2 || len(in.Message) > 4000 {
		problem(w, http.StatusUnprocessableEntity, "Message must be 2–4000 characters")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(r.Context(), "SELECT status FROM portal_support_tickets WHERE id=? AND account_id=? AND realm_key=? FOR UPDATE", id, a.ID, s.c.RealmKey).Scan(&status); err != nil {
		problem(w, http.StatusNotFound, "Ticket not found")
		return
	}
	if status == "closed" {
		problem(w, http.StatusConflict, "Closed tickets cannot receive replies")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message) VALUES(?,?,'player',?)", id, a.ID, in.Message); err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE portal_support_tickets SET status='pending_staff',resolved_at=NULL,updated_at=NOW() WHERE id=?", id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_ticket_events(ticket_id,actor_account_id,event_type,details) VALUES(?,?,'player_reply','')", id, a.ID)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not send reply")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) adminTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "support"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	where := " WHERE t.realm_key=?"
	args := []any{s.c.RealmKey}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		where += " AND t.status=?"
		args = append(args, status)
	}
	if priority := strings.TrimSpace(r.URL.Query().Get("priority")); priority != "" {
		where += " AND t.priority=?"
		args = append(args, priority)
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Search is too long")
		return
	}
	if search != "" {
		where += " AND (t.subject LIKE ? OR a.username LIKE ? OR CAST(t.id AS CHAR)=?)"
		pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(search, "\\", "\\\\"), "%", "\\%"), "_", "\\_") + "%"
		args = append(args, pattern, pattern, search)
	}
	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM portal_support_tickets t JOIN %s.account a ON a.id=t.account_id%s`, s.c.AuthDB, where)
	if err := s.s.Auth.QueryRowContext(r.Context(), countQuery, args...).Scan(&total); err != nil {
		problem(w, 500, "Could not count tickets")
		return
	}
	ticketPagination := paginationMeta(page, perPage, total)
	offset = (ticketPagination.Page - 1) * perPage
	query := fmt.Sprintf(`SELECT t.id,t.account_id,a.username,t.character_guid,t.subject,t.message,t.status,t.response,COALESCE(g.username,''),t.category,t.priority,t.tags,t.assigned_to,COALESCE(assignee.username,''),t.due_at,t.first_response_at,t.resolved_at,t.created_at,t.updated_at FROM portal_support_tickets t JOIN %s.account a ON a.id=t.account_id LEFT JOIN %s.account g ON g.id=t.gm_account_id LEFT JOIN %s.account assignee ON assignee.id=t.assigned_to%s ORDER BY FIELD(t.status,'open','pending_staff','answered','pending_player','resolved','closed'),FIELD(t.priority,'urgent','high','normal','low'),t.id DESC LIMIT ? OFFSET ?`, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB, where)
	queryArgs := append(append([]any{}, args...), perPage, offset)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, queryArgs...)
	if err != nil {
		problem(w, 500, "Could not load tickets")
		return
	}
	defer rows.Close()
	out := []supportTicket{}
	for rows.Next() {
		var x supportTicket
		if rows.Scan(&x.ID, &x.AccountID, &x.Username, &x.CharacterGUID, &x.Subject, &x.Message, &x.Status, &x.Response, &x.GM, &x.Category, &x.Priority, &x.Tags, &x.AssignedTo, &x.AssignedName, &x.DueAt, &x.FirstResponse, &x.ResolvedAt, &x.Created, &x.Updated) == nil {
			out = append(out, x)
		}
	}
	s.attachTicketMessages(r, out, true)
	jsonOut(w, 200, map[string]any{"tickets": out, "pagination": ticketPagination})
}

func (s *Server) adminTicketUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "support")
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid ticket")
		return
	}
	var in struct {
		Status, Response, InternalNote, Category, Priority, Tags string
		AssignToSelf, Unassign                                   bool
	}
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.Response = strings.TrimSpace(in.Response)
	validStatuses := map[string]bool{"open": true, "answered": true, "pending_player": true, "pending_staff": true, "resolved": true, "closed": true}
	if in.Status != "" && !validStatuses[in.Status] {
		problem(w, 422, "Invalid ticket status")
		return
	}
	if len(in.Response) > 4000 || len(in.InternalNote) > 4000 || len(in.Tags) > 500 {
		problem(w, 422, "Reply, internal note, or tags exceed their limit")
		return
	}
	var targetAccount, assignedTo uint32
	var subject, status, category, priority, tags string
	if err = s.s.Auth.QueryRowContext(r.Context(), "SELECT account_id,subject,status,category,priority,tags,assigned_to FROM portal_support_tickets WHERE id=? AND realm_key=?", id, s.c.RealmKey).Scan(&targetAccount, &subject, &status, &category, &priority, &tags, &assignedTo); err != nil {
		problem(w, http.StatusNotFound, "Ticket not found")
		return
	}
	if in.Status != "" {
		status = in.Status
	}
	if in.Category != "" {
		in.Category = strings.ToLower(strings.TrimSpace(in.Category))
		if !validTicketCategory(in.Category) {
			problem(w, 422, "Invalid ticket category")
			return
		}
		category = in.Category
	}
	if in.Priority != "" {
		in.Priority = strings.ToLower(strings.TrimSpace(in.Priority))
		if !map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}[in.Priority] {
			problem(w, 422, "Invalid priority")
			return
		}
		priority = in.Priority
	}
	if in.Tags != "" {
		tags = strings.TrimSpace(in.Tags)
	}
	if in.AssignToSelf {
		assignedTo = a.ID
	}
	if in.Unassign {
		assignedTo = 0
	}
	dueHours := map[string]int{"low": 120, "normal": 72, "high": 24, "urgent": 4}[priority]
	res, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_support_tickets SET status=?,response=CASE WHEN ?<>'' THEN ? ELSE response END,gm_account_id=?,category=?,priority=?,tags=?,assigned_to=?,due_at=DATE_ADD(created_at,INTERVAL ? HOUR),first_response_at=CASE WHEN ?<>'' THEN COALESCE(first_response_at,NOW()) ELSE first_response_at END,resolved_at=CASE WHEN ? IN ('resolved','closed') THEN COALESCE(resolved_at,NOW()) ELSE NULL END WHERE id=? AND realm_key=?`, status, in.Response, in.Response, a.ID, category, priority, tags, assignedTo, dueHours, in.Response, status, id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not update ticket")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "Ticket not found")
		return
	}
	if in.Response != "" {
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message) VALUES(?,?,'staff',?)", id, a.ID, in.Response)
	}
	if strings.TrimSpace(in.InternalNote) != "" {
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message) VALUES(?,?,'internal',?)", id, a.ID, strings.TrimSpace(in.InternalNote))
	}
	details := fmt.Sprintf("status=%s priority=%s category=%s assigned_to=%d", status, priority, category, assignedTo)
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_ticket_events(ticket_id,actor_account_id,event_type,details) VALUES(?,?,'staff_update',?)", id, a.ID, details)
	s.notifyAccount(r.Context(), targetAccount, "support", "Support ticket updated", subject+" is now "+in.Status+".", "/account/support")
	jsonOut(w, 200, map[string]any{"ok": true})
}

func validTicketCategory(category string) bool {
	return map[string]bool{"general": true, "account": true, "character": true, "billing": true, "bug": true, "report": true, "appeal": true}[category]
}
