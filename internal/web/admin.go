package web

import (
	"fmt"
	"net/http"
	"time"
)

func (s *Server) adminOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireGM(r); !ok {
		problem(w, http.StatusForbidden, "GM access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT o.id,a.username,o.character_guid,p.name,o.total,o.status,o.attempts,o.error_message,o.created_at FROM portal_orders o JOIN portal_products p ON p.id=o.product_id JOIN `+"`"+s.c.AuthDB+"`"+`.account a ON a.id=o.account_id ORDER BY o.id DESC LIMIT 100`)
	if err != nil {
		problem(w, 500, "Could not load orders")
		return
	}
	defer rows.Close()
	type row struct {
		ID            uint64    `json:"id"`
		Username      string    `json:"username"`
		CharacterGUID uint32    `json:"characterGuid"`
		Product       string    `json:"product"`
		Total         uint32    `json:"total"`
		Status        string    `json:"status"`
		Attempts      uint32    `json:"attempts"`
		Error         string    `json:"error"`
		Created       time.Time `json:"created"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Username, &x.CharacterGUID, &x.Product, &x.Total, &x.Status, &x.Attempts, &x.Error, &x.Created) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

func (s *Server) adminLedger(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireGM(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT l.id,COALESCE(actor.username,'SYSTEM'),target.username,l.amount,l.reason,l.created_at FROM portal_credit_ledger l LEFT JOIN `+"`"+s.c.AuthDB+"`"+`.account actor ON actor.id=l.actor_account_id JOIN `+"`"+s.c.AuthDB+"`"+`.account target ON target.id=l.target_account_id ORDER BY l.id DESC LIMIT 100`)
	if err != nil {
		problem(w, 500, "Could not load credit ledger")
		return
	}
	defer rows.Close()
	type row struct {
		ID            uint64 `json:"id"`
		Actor, Target string
		Amount        int32
		Reason        string
		Created       time.Time
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Actor, &x.Target, &x.Amount, &x.Reason, &x.Created) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"entries": out})
}

func (s *Server) adminProducts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireGM(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,name,price,category,active FROM portal_products ORDER BY id DESC LIMIT 200")
	if err != nil {
		problem(w, 500, "Could not load products")
		return
	}
	defer rows.Close()
	type row struct {
		ID       uint32 `json:"id"`
		Name     string `json:"name"`
		Price    uint32 `json:"price"`
		Category string `json:"category"`
		Active   bool   `json:"active"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Name, &x.Price, &x.Category, &x.Active) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"products": out, "schema": fmt.Sprintf("%s.portal_products", s.c.AuthDB)})
}
