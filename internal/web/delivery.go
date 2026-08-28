package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type queuedOrder struct {
	ID                       uint64
	AccountID, CharacterGUID uint32
	ServiceLevel             uint8
	Gold                     uint32
}

func (s *Server) deliveryLoop() {
	_, _ = s.s.Auth.Exec("UPDATE portal_orders SET status='review',error_message='Delivery interrupted; verify before retrying' WHERE status='delivering'")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		order, ok := s.claimOrder(ctx)
		if ok {
			s.fulfillOrder(ctx, order)
		}
		cancel()
	}
}

func (s *Server) claimOrder(ctx context.Context) (queuedOrder, bool) {
	var o queuedOrder
	tx, err := s.s.Auth.BeginTx(ctx, nil)
	if err != nil {
		return o, false
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, "SELECT id,account_id,character_guid,service_level,gold_amount FROM portal_orders WHERE status='pending' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED").Scan(&o.ID, &o.AccountID, &o.CharacterGUID, &o.ServiceLevel, &o.Gold)
	if err != nil {
		return o, false
	}
	if _, err = tx.ExecContext(ctx, "UPDATE portal_orders SET status='delivering',attempts=attempts+1,delivery_started_at=NOW(),error_message='' WHERE id=?", o.ID); err != nil {
		return o, false
	}
	if tx.Commit() != nil {
		return o, false
	}
	return o, true
}

func (s *Server) fulfillOrder(ctx context.Context, o queuedOrder) {
	var name string
	q := fmt.Sprintf("SELECT name FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Characters.QueryRowContext(ctx, q, o.CharacterGUID, o.AccountID).Scan(&name); err != nil {
		s.reviewOrder(ctx, o.ID, "Character no longer exists")
		return
	}
	if strings.ContainsAny(name, " \t\r\n\"\\") {
		s.reviewOrder(ctx, o.ID, "Unsafe character name")
		return
	}
	realm := strings.NewReplacer("\"", "", "\\", "", "\r", " ", "\n", " ").Replace(s.c.RealmName)
	if len(realm) > 80 {
		realm = realm[:80]
	}
	rows, err := s.s.Auth.QueryContext(ctx, "SELECT item_id,quantity FROM portal_order_items WHERE order_id=? ORDER BY item_id", o.ID)
	if err != nil {
		s.reviewOrder(ctx, o.ID, err.Error())
		return
	}
	items := []string{}
	for rows.Next() {
		var id, quantity uint32
		if rows.Scan(&id, &quantity) == nil {
			items = append(items, fmt.Sprintf("%d:%d", id, quantity))
		}
	}
	rows.Close()
	commands := []string{}
	for start := 0; start < len(items); start += 12 {
		end := start + 12
		if end > len(items) {
			end = len(items)
		}
		commands = append(commands, fmt.Sprintf(`send items %s "Portal order %d" "Thank you for supporting %s." %s`, name, o.ID, realm, strings.Join(items[start:end], " ")))
	}
	if o.ServiceLevel > 0 {
		commands = append(commands, fmt.Sprintf("character level %s %d", name, o.ServiceLevel))
	}
	if o.Gold > 0 {
		commands = append(commands, fmt.Sprintf(`send money %s "Portal order %d" "Thank you for supporting %s." %d`, name, o.ID, realm, uint64(o.Gold)*10000))
	}
	for _, cmd := range commands {
		if _, err = s.soap.Command(ctx, cmd); err != nil {
			s.reviewOrder(ctx, o.ID, err.Error())
			return
		}
	}
	if _, err = s.s.Auth.ExecContext(ctx, "UPDATE portal_orders SET status='delivered',delivered_at=NOW() WHERE id=? AND status='delivering'", o.ID); err != nil {
		slog.Error("record delivery", "order", o.ID, "error", err)
	}
}
func (s *Server) reviewOrder(ctx context.Context, id uint64, message string) {
	if len(message) > 500 {
		message = message[:500]
	}
	_, _ = s.s.Auth.ExecContext(ctx, "UPDATE portal_orders SET status='review',error_message=? WHERE id=? AND status='delivering'", message, id)
	slog.Error("order requires review", "order", id, "error", message)
}

func (s *Server) requireGM(r *http.Request) (account, bool) {
	a, e := s.auth(r)
	return a, e == nil && int(s.gmLevel(r.Context(), a.ID)) >= s.c.GMLevel
}
func (s *Server) adminRetryOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireGM(r); !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if e != nil {
		problem(w, 400, "Invalid order")
		return
	}
	res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_orders SET status='pending',error_message='' WHERE id=? AND status IN ('review','failed')", id)
	if e != nil {
		problem(w, 500, "Could not retry order")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 409, "Order is not retryable")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (s *Server) adminRefundOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if e != nil {
		problem(w, 400, "Invalid order")
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var accountID, total uint32
	var status string
	if e = tx.QueryRowContext(r.Context(), "SELECT account_id,total,status FROM portal_orders WHERE id=? FOR UPDATE", id).Scan(&accountID, &total, &status); e != nil {
		problem(w, 404, "Order not found")
		return
	}
	if status != "pending" && status != "review" && status != "failed" {
		problem(w, 409, "Only pending or reviewed orders can be refunded")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance+? WHERE account_id=?", total, accountID); e == nil {
		_, e = tx.ExecContext(r.Context(), "UPDATE portal_orders SET status='refunded' WHERE id=?", id)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)", actor.ID, accountID, total, "Order "+strconv.FormatUint(id, 10)+" refund")
	}
	if e != nil || tx.Commit() != nil {
		problem(w, 500, "Could not refund order")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
