package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type cannedReply struct {
	ID        uint64    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}

func (s *Server) adminTicketEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "support"); !ok {
		problem(w, http.StatusForbidden, "Support permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid ticket")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for _, ticket := range s.mock.tickets {
			if ticket.ID != id {
				continue
			}
			events := []ticketEvent{{ID: 1, ActorID: ticket.AccountID, Type: "created", Details: ticket.Category, CreatedAt: ticket.Created}}
			for i, message := range ticket.Messages {
				events = append(events, ticketEvent{ID: uint64(i + 2), ActorID: uint32(message.AuthorID), Type: message.AuthorRole + "_message", Details: message.Message, CreatedAt: message.Created})
			}
			jsonOut(w, http.StatusOK, map[string]any{"events": events})
			return
		}
		problem(w, http.StatusNotFound, "Ticket not found")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT e.id,e.actor_account_id,e.event_type,e.details,e.created_at FROM portal_ticket_events e JOIN portal_support_tickets t ON t.id=e.ticket_id WHERE e.ticket_id=? AND t.realm_key=? ORDER BY e.id`, id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not load ticket history")
		return
	}
	defer rows.Close()
	events := []ticketEvent{}
	for rows.Next() {
		var event ticketEvent
		if rows.Scan(&event.ID, &event.ActorID, &event.Type, &event.Details, &event.CreatedAt) == nil {
			events = append(events, event)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) adminCannedReplies(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "support")
	if !ok {
		problem(w, 403, "Support permission required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			replies := append([]cannedReply(nil), s.mock.cannedReplies...)
			s.mock.mu.Unlock()
			jsonOut(w, 200, map[string]any{"replies": replies})
			return
		}
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,title,body,active,created_at FROM portal_canned_replies WHERE realm_key=? AND active=1 ORDER BY title", s.c.RealmKey)
		if err != nil {
			problem(w, 500, "Could not load canned replies")
			return
		}
		defer rows.Close()
		replies := []cannedReply{}
		for rows.Next() {
			var reply cannedReply
			if rows.Scan(&reply.ID, &reply.Title, &reply.Body, &reply.Active, &reply.CreatedAt) == nil {
				replies = append(replies, reply)
			}
		}
		jsonOut(w, 200, map[string]any{"replies": replies})
		return
	}
	var reply cannedReply
	if !decode(w, r, &reply) {
		return
	}
	reply.Title, reply.Body = strings.TrimSpace(reply.Title), strings.TrimSpace(reply.Body)
	if len(reply.Title) < 2 || len(reply.Title) > 100 || len(reply.Body) < 2 || len(reply.Body) > 4000 {
		problem(w, 422, "Title and reply body are required")
		return
	}
	reply.Active = true
	if s.c.MockMode {
		s.mock.mu.Lock()
		reply.ID = uint64(len(s.mock.cannedReplies) + 1)
		reply.CreatedAt = time.Now()
		s.mock.cannedReplies = append(s.mock.cannedReplies, reply)
		s.mock.mu.Unlock()
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_canned_replies(realm_key,title,body,active,created_by) VALUES(?,?,?,1,?)", s.c.RealmKey, reply.Title, reply.Body, a.ID)
		if err != nil {
			problem(w, 500, "Could not create canned reply")
			return
		}
		id, _ := res.LastInsertId()
		reply.ID = uint64(id)
	}
	jsonOut(w, 201, reply)
}

func (s *Server) adminCannedReplyDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "support"); !ok {
		problem(w, 403, "Support permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid canned reply")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.cannedReplies {
			if s.mock.cannedReplies[i].ID == id {
				s.mock.cannedReplies[i].Active = false
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_canned_replies SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
		if err == nil {
			if changed, _ := res.RowsAffected(); changed > 0 {
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	}
	problem(w, 404, "Canned reply not found")
}
