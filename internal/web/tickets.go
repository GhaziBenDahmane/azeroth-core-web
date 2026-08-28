package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type supportTicket struct {
	ID            uint64    `json:"id"`
	AccountID     uint32    `json:"accountId,omitempty"`
	Username      string    `json:"username,omitempty"`
	CharacterGUID uint32    `json:"characterGuid,omitempty"`
	Subject       string    `json:"subject"`
	Message       string    `json:"message"`
	Status        string    `json:"status"`
	Response      string    `json:"response,omitempty"`
	GM            string    `json:"gm,omitempty"`
	Created       time.Time `json:"created"`
	Updated       time.Time `json:"updated"`
}

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,character_guid,subject,message,status,response,created_at,updated_at FROM portal_support_tickets WHERE account_id=? ORDER BY id DESC LIMIT 50", a.ID)
	if err != nil {
		problem(w, 500, "Could not load tickets")
		return
	}
	defer rows.Close()
	out := []supportTicket{}
	for rows.Next() {
		var x supportTicket
		if rows.Scan(&x.ID, &x.CharacterGUID, &x.Subject, &x.Message, &x.Status, &x.Response, &x.Created, &x.Updated) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"tickets": out})
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
	}
	if !decode(w, r, &in) {
		return
	}
	in.Subject, in.Message = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Message)
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
	res, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_support_tickets(account_id,character_guid,subject,message,response) VALUES(?,?,?,?,?)", a.ID, in.CharacterGUID, in.Subject, in.Message, "")
	if err != nil {
		problem(w, 500, "Could not create ticket")
		return
	}
	id, _ := res.LastInsertId()
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) adminTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireGM(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT t.id,t.account_id,a.username,t.character_guid,t.subject,t.message,t.status,t.response,COALESCE(g.username,''),t.created_at,t.updated_at FROM portal_support_tickets t JOIN %s.account a ON a.id=t.account_id LEFT JOIN %s.account g ON g.id=t.gm_account_id ORDER BY FIELD(t.status,'open','answered','closed'),t.id DESC LIMIT 100`, s.c.AuthDB, s.c.AuthDB))
	if err != nil {
		problem(w, 500, "Could not load tickets")
		return
	}
	defer rows.Close()
	out := []supportTicket{}
	for rows.Next() {
		var x supportTicket
		if rows.Scan(&x.ID, &x.AccountID, &x.Username, &x.CharacterGUID, &x.Subject, &x.Message, &x.Status, &x.Response, &x.GM, &x.Created, &x.Updated) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"tickets": out})
}

func (s *Server) adminTicketUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid ticket")
		return
	}
	var in struct{ Status, Response string }
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.Response = strings.TrimSpace(in.Response)
	if in.Status != "open" && in.Status != "answered" && in.Status != "closed" {
		problem(w, 422, "Status must be open, answered, or closed")
		return
	}
	if len(in.Response) > 4000 || (in.Status == "answered" && len(in.Response) < 2) {
		problem(w, 422, "An answer is required and must not exceed 4000 characters")
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_support_tickets SET status=?,response=?,gm_account_id=? WHERE id=?", in.Status, in.Response, a.ID, id)
	if err != nil {
		problem(w, 500, "Could not update ticket")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "Ticket not found")
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true})
}
