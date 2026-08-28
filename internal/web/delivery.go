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
	ServiceAction            string
}

func (s *Server) deliveryLoop() {
	_, _ = s.s.Auth.Exec("UPDATE portal_orders SET status='review',error_message='Delivery interrupted; verify before retrying' WHERE status='delivering' AND realm_key=?", s.c.RealmKey)
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
	err = tx.QueryRowContext(ctx, "SELECT id,account_id,character_guid,service_level,gold_amount,service_action FROM portal_orders WHERE status='pending' AND realm_key=? ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED", s.c.RealmKey).Scan(&o.ID, &o.AccountID, &o.CharacterGUID, &o.ServiceLevel, &o.Gold, &o.ServiceAction)
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
	var online bool
	q := fmt.Sprintf("SELECT name,online FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Characters.QueryRowContext(ctx, q, o.CharacterGUID, o.AccountID).Scan(&name, &online); err != nil {
		s.reviewOrder(ctx, o.ID, "Character no longer exists")
		return
	}
	if online {
		s.reviewOrder(ctx, o.ID, "Character must be offline for delivery")
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
	attachments := []string{}
	for rows.Next() {
		var id, quantity uint32
		if err = rows.Scan(&id, &quantity); err != nil {
			rows.Close()
			s.reviewOrder(ctx, o.ID, "Could not read order items: "+err.Error())
			return
		}
		var stackable int64
		if err = s.s.World.QueryRowContext(ctx, "SELECT stackable FROM item_template WHERE entry=?", id).Scan(&stackable); err != nil {
			rows.Close()
			s.reviewOrder(ctx, o.ID, fmt.Sprintf("Item %d is unavailable in the world database: %v", id, err))
			return
		}
		attachments = append(attachments, splitItemStacks(id, quantity, stackable)...)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		s.reviewOrder(ctx, o.ID, "Could not read order items: "+err.Error())
		return
	}
	rows.Close()
	commands := []string{}
	for _, mailItems := range chunkMailStacks(attachments) {
		commands = append(commands, fmt.Sprintf(`send items %s "Portal order %d" "Thank you for supporting %s." %s`, name, o.ID, realm, strings.Join(mailItems, " ")))
	}
	if o.ServiceLevel > 0 {
		commands = append(commands, fmt.Sprintf("character level %s %d", name, o.ServiceLevel))
	}
	if o.Gold > 0 {
		commands = append(commands, fmt.Sprintf(`send money %s "Portal order %d" "Thank you for supporting %s." %d`, name, o.ID, realm, uint64(o.Gold)*10000))
	}
	service, serviceErr := serviceCommand(o.ServiceAction, name)
	if serviceErr != nil {
		s.reviewOrder(ctx, o.ID, "Unsupported service action")
		return
	}
	if service != "" {
		commands = append(commands, service)
	}
	for _, cmd := range commands {
		if _, err = s.soap.Command(ctx, cmd); err != nil {
			s.reviewOrder(ctx, o.ID, err.Error())
			return
		}
	}
	if o.ServiceLevel == 80 {
		if err = s.maxLevel80CombatSkills(ctx, o.CharacterGUID); err != nil {
			s.reviewOrder(ctx, o.ID, "Items delivered, but weapon training failed: "+err.Error())
			return
		}
	}
	if _, err = s.s.Auth.ExecContext(ctx, "UPDATE portal_orders SET status='delivered',delivered_at=NOW() WHERE id=? AND status='delivering'", o.ID); err != nil {
		slog.Error("record delivery", "order", o.ID, "error", err)
	}
}

func (s *Server) maxLevel80CombatSkills(ctx context.Context, characterGUID uint32) error {
	// AzerothCore's maxskill command cannot run from its console/SOAP interface.
	// Updating existing skill rows while the character is offline preserves the
	// class's learned weapon proficiencies without granting unsupported weapons.
	const combatSkills = "43,44,45,46,54,55,95,136,160,162,172,173,176,226,228,229,473"
	_, err := s.s.Characters.ExecContext(ctx, "UPDATE character_skills SET value=400,max=400 WHERE guid=? AND skill IN ("+combatSkills+")", characterGUID)
	return err
}

func splitItemStacks(itemID, quantity uint32, stackable int64) []string {
	if quantity == 0 {
		return nil
	}
	if stackable < 1 {
		stackable = 1
	}
	maxStack := uint64(stackable)
	remaining := uint64(quantity)
	stacks := make([]string, 0, (remaining+maxStack-1)/maxStack)
	for remaining > 0 {
		count := remaining
		if count > maxStack {
			count = maxStack
		}
		stacks = append(stacks, fmt.Sprintf("%d:%d", itemID, count))
		remaining -= count
	}
	return stacks
}

func chunkMailStacks(stacks []string) [][]string {
	var messages [][]string
	for start := 0; start < len(stacks); start += 12 {
		end := start + 12
		if end > len(stacks) {
			end = len(stacks)
		}
		messages = append(messages, stacks[start:end])
	}
	return messages
}

func serviceCommand(action, characterName string) (string, error) {
	switch action {
	case "":
		return "", nil
	case "race_change":
		return fmt.Sprintf("character changerace %s", characterName), nil
	case "faction_change":
		return fmt.Sprintf("character changefaction %s", characterName), nil
	default:
		return "", fmt.Errorf("unsupported service action %q", action)
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
	if s.c.MockMode {
		if username, ok := s.mockUser(r); ok {
			return account{ID: 1, Username: username, Email: "demo@example.com", GMLevel: 3}, true
		}
		return account{}, false
	}
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
	res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_orders SET status='pending',error_message='' WHERE id=? AND realm_key=? AND status IN ('review','failed')", id, s.c.RealmKey)
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
	if e = tx.QueryRowContext(r.Context(), "SELECT account_id,total,status FROM portal_orders WHERE id=? AND realm_key=? FOR UPDATE", id, s.c.RealmKey).Scan(&accountID, &total, &status); e != nil {
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
