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

var level80WeaponSkills = map[uint8][]uint16{
	1:  {43, 44, 45, 46, 54, 55, 95, 136, 160, 162, 172, 173, 176, 226, 229, 473}, // Warrior
	2:  {43, 44, 54, 55, 95, 160, 162, 172, 229},                                  // Paladin
	3:  {43, 44, 45, 46, 55, 95, 136, 162, 172, 173, 226, 229, 473},               // Hunter
	4:  {43, 45, 46, 54, 95, 162, 173, 176, 226, 473},                             // Rogue
	5:  {54, 95, 136, 162, 173, 228},                                              // Priest
	6:  {43, 44, 54, 55, 95, 160, 162, 172, 229},                                  // Death Knight
	7:  {44, 54, 95, 136, 162, 173, 473},                                          // Shaman
	8:  {43, 95, 136, 162, 173, 228},                                              // Mage
	9:  {43, 95, 136, 162, 173, 228},                                              // Warlock
	11: {54, 95, 136, 160, 162, 173, 229, 473},                                    // Druid
}

var weaponProficiencySpells = map[uint16]uint32{
	43: 201, 44: 196, 45: 264, 46: 266, 54: 198, 55: 202, 136: 227,
	160: 199, 172: 197, 173: 1180, 176: 2567, 226: 5011, 228: 5009,
	229: 200, 473: 15590,
}

var level80RidingSpells = []uint32{33388, 33391, 34090, 34091, 54197}

func (s *Server) deliveryLoop() {
	defer s.deliveryWG.Done()
	_, _ = s.s.Auth.Exec("UPDATE portal_orders SET status='review',error_message='Delivery interrupted; verify before retrying' WHERE status='delivering' AND realm_key=?", s.c.RealmKey)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopDelivery:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			order, ok := s.claimOrder(ctx)
			if ok {
				s.fulfillOrder(ctx, order)
			}
			cancel()
		}
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
	var classID uint8
	q := fmt.Sprintf("SELECT name,online,class FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Characters.QueryRowContext(ctx, q, o.CharacterGUID, o.AccountID).Scan(&name, &online, &classID); err != nil {
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
	type deliveryCommand struct {
		key, kind, command string
	}
	commands := []deliveryCommand{}
	for index, mailItems := range chunkMailStacks(attachments) {
		commands = append(commands, deliveryCommand{
			key:     fmt.Sprintf("items-%02d", index+1),
			kind:    "item_mail",
			command: fmt.Sprintf(`send items %s "Portal order %d" "Thank you for supporting %s." %s`, name, o.ID, realm, strings.Join(mailItems, " ")),
		})
	}
	if o.ServiceLevel > 0 {
		commands = append(commands, deliveryCommand{"level", "character_level", fmt.Sprintf("character level %s %d", name, o.ServiceLevel)})
	}
	if o.Gold > 0 {
		commands = append(commands, deliveryCommand{"money", "money_mail", fmt.Sprintf(`send money %s "Portal order %d" "Thank you for supporting %s." %d`, name, o.ID, realm, uint64(o.Gold)*10000)})
	}
	service, serviceErr := serviceCommand(o.ServiceAction, name)
	if serviceErr != nil {
		s.reviewOrder(ctx, o.ID, "Unsupported service action")
		return
	}
	if service != "" {
		commands = append(commands, deliveryCommand{"service", "character_service", service})
	}
	for _, step := range commands {
		if err = s.runOrderStep(ctx, o.ID, step.key, step.kind, func() (string, error) {
			return s.soapCommand(ctx, step.command)
		}); err != nil {
			s.reviewOrder(ctx, o.ID, err.Error())
			return
		}
	}
	if o.ServiceLevel == 80 {
		if err = s.runOrderStep(ctx, o.ID, "training", "level_80_training", func() (string, error) {
			err := s.prepareLevel80Character(ctx, o.CharacterGUID, classID)
			return "Level 80 training applied", err
		}); err != nil {
			s.reviewOrder(ctx, o.ID, "Items delivered, but level 80 training failed: "+err.Error())
			return
		}
	}
	if _, err = s.s.Auth.ExecContext(ctx, "UPDATE portal_orders SET status='delivered',delivered_at=NOW() WHERE id=? AND status='delivering'", o.ID); err != nil {
		slog.Error("record delivery", "order", o.ID, "error", err)
	} else {
		s.metrics.deliverySuccess.Add(1)
		s.notifyAccount(ctx, o.AccountID, "order", "Order delivered", fmt.Sprintf("Order #%d was delivered to %s.", o.ID, name), "/account/orders")
	}
}

func (s *Server) soapCommand(ctx context.Context, command string) (string, error) {
	start := time.Now()
	s.metrics.soapRequests.Add(1)
	response, err := s.soap.Command(ctx, command)
	s.metrics.soapLatencyMicros.Store(time.Since(start).Microseconds())
	if err != nil {
		s.metrics.soapFaults.Add(1)
	} else {
		s.metrics.soapLastSuccessUnix.Store(time.Now().Unix())
	}
	return response, err
}

// runOrderStep makes retries resume after completed fulfillment work instead of
// replaying the entire order. An executing step is deliberately never retried
// automatically: the process may have lost the SOAP response after the realm
// already applied the command, so an operator must reconcile it explicitly.
func (s *Server) runOrderStep(ctx context.Context, orderID uint64, key, kind string, run func() (string, error)) error {
	if _, err := s.s.Auth.ExecContext(ctx, `INSERT IGNORE INTO portal_order_steps(order_id,step_key,kind) VALUES(?,?,?)`, orderID, key, kind); err != nil {
		return fmt.Errorf("create fulfillment step %s: %w", key, err)
	}
	var status string
	if err := s.s.Auth.QueryRowContext(ctx, `SELECT status FROM portal_order_steps WHERE order_id=? AND step_key=?`, orderID, key).Scan(&status); err != nil {
		return fmt.Errorf("load fulfillment step %s: %w", key, err)
	}
	if status == "completed" {
		return nil
	}
	if status == "executing" {
		return fmt.Errorf("fulfillment step %s has an uncertain result; reconcile it before retrying", key)
	}
	result, err := s.s.Auth.ExecContext(ctx, `UPDATE portal_order_steps SET status='executing',attempts=attempts+1,response='' WHERE order_id=? AND step_key=? AND status IN ('pending','failed')`, orderID, key)
	if err != nil {
		return fmt.Errorf("start fulfillment step %s: %w", key, err)
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return fmt.Errorf("fulfillment step %s could not be claimed", key)
	}
	response, runErr := run()
	if len(response) > 500 {
		response = response[:500]
	}
	if runErr != nil {
		message := runErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		_, _ = s.s.Auth.ExecContext(ctx, `UPDATE portal_order_steps SET status='failed',response=? WHERE order_id=? AND step_key=? AND status='executing'`, message, orderID, key)
		return fmt.Errorf("fulfillment step %s failed: %w", key, runErr)
	}
	if _, err = s.s.Auth.ExecContext(ctx, `UPDATE portal_order_steps SET status='completed',response=?,completed_at=NOW() WHERE order_id=? AND step_key=? AND status='executing'`, response, orderID, key); err != nil {
		return fmt.Errorf("record fulfillment step %s: %w", key, err)
	}
	return nil
}

func (s *Server) prepareLevel80Character(ctx context.Context, characterGUID uint32, classID uint8) error {
	skills, ok := level80WeaponSkills[classID]
	if !ok {
		return fmt.Errorf("unsupported class %d", classID)
	}

	// AzerothCore marks the convenient in-game class-learning and maxskill
	// commands as Console::No, so SOAP cannot use them. Resolve the same class
	// trainer spell list from the world DB and persist it while the character is
	// offline. INSERT IGNORE makes retries safe and preserves existing spec masks.
	rows, err := s.s.World.QueryContext(ctx, `SELECT DISTINCT ts.SpellId,
		COALESCE(sd.Effect_1,0),COALESCE(sd.EffectTriggerSpell_1,0),
		COALESCE(sd.Effect_2,0),COALESCE(sd.EffectTriggerSpell_2,0),
		COALESCE(sd.Effect_3,0),COALESCE(sd.EffectTriggerSpell_3,0)
		FROM trainer_spell ts JOIN trainer t ON t.Id=ts.TrainerId
		LEFT JOIN spell_dbc sd ON sd.ID=ts.SpellId
		WHERE t.Type=0 AND t.Requirement=? AND ts.ReqLevel<=80`, classID)
	if err != nil {
		return fmt.Errorf("load class trainer spells: %w", err)
	}
	spells := append([]uint32(nil), level80RidingSpells...)
	classSpellCount := 0
	for rows.Next() {
		var spell uint32
		var effects, triggers [3]uint32
		if err = rows.Scan(&spell, &effects[0], &triggers[0], &effects[1], &triggers[1], &effects[2], &triggers[2]); err != nil {
			rows.Close()
			return fmt.Errorf("read class trainer spell: %w", err)
		}
		learned := learnedTrainerSpells(spell, effects, triggers)
		classSpellCount += len(learned)
		spells = append(spells, learned...)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read class trainer spells: %w", err)
	}
	rows.Close()
	if classSpellCount == 0 {
		return fmt.Errorf("no trainer spells found for class %d", classID)
	}
	for _, skill := range skills {
		if spell := weaponProficiencySpells[skill]; spell != 0 {
			spells = append(spells, spell)
		}
	}

	tx, err := s.s.Characters.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, skill := range skills {
		if _, err = tx.ExecContext(ctx, `INSERT INTO character_skills(guid,skill,value,max) VALUES(?,?,400,400)
			ON DUPLICATE KEY UPDATE value=GREATEST(value,400),max=GREATEST(max,400)`, characterGUID, skill); err != nil {
			return fmt.Errorf("train weapon skill %d: %w", skill, err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO character_skills(guid,skill,value,max) VALUES(?,762,300,300)
		ON DUPLICATE KEY UPDATE value=GREATEST(value,300),max=GREATEST(max,300)`, characterGUID); err != nil {
		return fmt.Errorf("train riding: %w", err)
	}
	seen := make(map[uint32]bool, len(spells))
	for _, spell := range spells {
		if spell == 0 || seen[spell] {
			continue
		}
		seen[spell] = true
		if _, err = tx.ExecContext(ctx, "INSERT IGNORE INTO character_spell(guid,spell,specMask) VALUES(?,?,3)", characterGUID, spell); err != nil {
			return fmt.Errorf("learn spell %d: %w", spell, err)
		}
	}
	return tx.Commit()
}

func learnedTrainerSpells(spell uint32, effects, triggers [3]uint32) []uint32 {
	learned := make([]uint32, 0, 3)
	for i, effect := range effects {
		if effect == 36 && triggers[i] != 0 { // SPELL_EFFECT_LEARN_SPELL in 3.3.5a.
			learned = append(learned, triggers[i])
		}
	}
	if len(learned) == 0 {
		learned = append(learned, spell)
	}
	return learned
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
	s.metrics.deliveryReview.Add(1)
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

func (s *Server) staffPermissions(level uint8) []string {
	return s.staffPermissionsFor(level, "")
}

func (s *Server) staffPermissionsFor(level uint8, username string) []string {
	permissions := []string{}
	add := func(values ...string) {
		for _, value := range values {
			found := false
			for _, existing := range permissions {
				if existing == value {
					found = true
					break
				}
			}
			if !found {
				permissions = append(permissions, value)
			}
		}
	}
	if int(level) >= s.c.SupportGMLevel {
		add("support", "monitoring")
	}
	if int(level) >= s.c.ModeratorGMLevel {
		add("players", "moderation", "audit")
	}
	if int(level) >= s.c.GMLevel {
		add("overview", "commerce", "content", "realm", "settings", "admin")
	}
	if s.c.StaffShopManagers[strings.ToUpper(username)] {
		add("commerce")
	}
	if s.c.EnableGMConsole && int(level) >= s.c.GMConsoleLevel {
		add("console", "realm")
	}
	return permissions
}

func (s *Server) requireStaffPermission(r *http.Request, permission string) (account, bool) {
	var a account
	if s.c.MockMode {
		if username, ok := s.mockUser(r); ok {
			a = account{ID: 1, Username: username, Email: "demo@example.com", GMLevel: 3}
		} else {
			return account{}, false
		}
	} else {
		var err error
		a, err = s.auth(r)
		if err != nil {
			return account{}, false
		}
		a.GMLevel = s.gmLevel(r.Context(), a.ID)
	}
	_, permissions := s.effectiveStaff(r.Context(), a)
	for _, allowed := range permissions {
		if allowed == permission || allowed == "admin" {
			return a, true
		}
	}
	return account{}, false
}
func (s *Server) adminRetryOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
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
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'order.retry',?,?)", actor.ID, strconv.FormatUint(id, 10), "Order queued for resumable delivery")
	jsonOut(w, 200, map[string]bool{"ok": true})
}

type bulkOrderResult struct {
	ID      uint64 `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (s *Server) adminBulkRetryOrders(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	var in struct {
		IDs []uint64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Select between 1 and 100 orders")
		return
	}
	seen, results, succeeded := map[uint64]bool{}, make([]bulkOrderResult, 0, len(in.IDs)), 0
	for _, id := range in.IDs {
		if id == 0 || seen[id] {
			results = append(results, bulkOrderResult{ID: id, Status: "skipped", Message: "Invalid or duplicate order ID"})
			continue
		}
		seen[id] = true
		if s.c.MockMode {
			results = append(results, bulkOrderResult{ID: id, Status: "queued", Message: "Queued for resumable delivery"})
			succeeded++
			continue
		}
		result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_orders SET status='pending',error_message='' WHERE id=? AND realm_key=? AND status IN ('review','failed')", id, s.c.RealmKey)
		if err != nil {
			results = append(results, bulkOrderResult{ID: id, Status: "failed", Message: "Database update failed"})
			continue
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			results = append(results, bulkOrderResult{ID: id, Status: "skipped", Message: "Order is not retryable"})
			continue
		}
		succeeded++
		results = append(results, bulkOrderResult{ID: id, Status: "queued", Message: "Queued for resumable delivery"})
		_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id) VALUES(?,'order.retry',?,'Bulk resumable retry',?,?)`, actor.ID, strconv.FormatUint(id, 10), s.c.RealmKey, RequestID(r.Context()))
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": succeeded == len(in.IDs), "requested": len(in.IDs), "succeeded": succeeded, "failed": len(in.IDs) - succeeded, "results": results, "requestId": RequestID(r.Context())})
}
func (s *Server) adminRefundOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
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
	var accountID, total, completedSteps, productID, stockLimit uint32
	var status string
	if e = tx.QueryRowContext(r.Context(), `SELECT o.account_id,o.total,o.status,(SELECT COUNT(*) FROM portal_order_steps WHERE order_id=o.id AND status='completed'),o.product_id,p.stock_limit FROM portal_orders o JOIN portal_products p ON p.id=o.product_id WHERE o.id=? AND o.realm_key=? FOR UPDATE`, id, s.c.RealmKey).Scan(&accountID, &total, &status, &completedSteps, &productID, &stockLimit); e != nil {
		problem(w, 404, "Order not found")
		return
	}
	if status != "pending" && status != "review" && status != "failed" {
		problem(w, 409, "Only pending or reviewed orders can be refunded")
		return
	}
	if completedSteps > 0 {
		problem(w, 409, "This order was partially fulfilled; reconcile delivered steps before issuing a refund")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance+? WHERE account_id=?", total, accountID); e == nil {
		_, e = tx.ExecContext(r.Context(), "UPDATE portal_orders SET status='refunded' WHERE id=?", id)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)", actor.ID, accountID, total, "Order "+strconv.FormatUint(id, 10)+" refund")
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'order.refund',?,?)", actor.ID, strconv.FormatUint(id, 10), "Credits returned after unfulfilled order")
	}
	if e == nil && stockLimit > 0 {
		if _, e = tx.ExecContext(r.Context(), `UPDATE portal_products SET sold_count=GREATEST(sold_count-1,0) WHERE id=? AND realm_key=?`, productID, s.c.RealmKey); e == nil {
			_, e = tx.ExecContext(r.Context(), `INSERT INTO portal_stock_movements(realm_key,product_id,quantity_delta,movement_type,reference_id,reason,actor_account_id) VALUES(?,?,1,'refund',?,'Unfulfilled order stock released',?)`, s.c.RealmKey, productID, strconv.FormatUint(id, 10), actor.ID)
		}
	}
	if e != nil || tx.Commit() != nil {
		problem(w, 500, "Could not refund order")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

type orderStepView struct {
	Key         string     `json:"key"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	Attempts    uint32     `json:"attempts"`
	Response    string     `json:"response"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

func (s *Server) adminOrderSteps(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, http.StatusForbidden, "Commerce access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid order")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT os.step_key,os.kind,os.status,os.attempts,os.response,os.updated_at,os.completed_at
		FROM portal_order_steps os JOIN portal_orders o ON o.id=os.order_id
		WHERE os.order_id=? AND o.realm_key=? ORDER BY os.created_at,os.step_key`, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load fulfillment steps")
		return
	}
	defer rows.Close()
	steps := []orderStepView{}
	for rows.Next() {
		var step orderStepView
		if err := rows.Scan(&step.Key, &step.Kind, &step.Status, &step.Attempts, &step.Response, &step.UpdatedAt, &step.CompletedAt); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read fulfillment steps")
			return
		}
		steps = append(steps, step)
	}
	jsonOut(w, http.StatusOK, map[string]any{"steps": steps})
}

func validOrderStepKey(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func (s *Server) adminResolveOrderStep(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	key := r.PathValue("key")
	if err != nil || !validOrderStepKey(key) {
		problem(w, http.StatusBadRequest, "Invalid fulfillment step")
		return
	}
	var in struct {
		Resolution string `json:"resolution"`
		Reason     string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Resolution = strings.ToLower(strings.TrimSpace(in.Resolution))
	in.Reason = strings.TrimSpace(in.Reason)
	if (in.Resolution != "retry" && in.Resolution != "completed") || len(in.Reason) < 3 || len(in.Reason) > 255 {
		problem(w, http.StatusUnprocessableEntity, "Choose retry or completed and provide a 3–255 character reconciliation reason")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var current, orderStatus string
	if err = tx.QueryRowContext(r.Context(), `SELECT os.status,o.status FROM portal_order_steps os JOIN portal_orders o ON o.id=os.order_id WHERE os.order_id=? AND os.step_key=? AND o.realm_key=? FOR UPDATE`, id, key, s.c.RealmKey).Scan(&current, &orderStatus); err != nil {
		problem(w, http.StatusNotFound, "Fulfillment step not found")
		return
	}
	if orderStatus == "delivered" || orderStatus == "refunded" {
		problem(w, http.StatusConflict, "Completed or refunded orders cannot be reconciled")
		return
	}
	next := "pending"
	completedAt := "NULL"
	if in.Resolution == "completed" {
		next = "completed"
		completedAt = "NOW()"
	}
	query := fmt.Sprintf("UPDATE portal_order_steps SET status=?,response=?,completed_at=%s WHERE order_id=? AND step_key=?", completedAt)
	if _, err = tx.ExecContext(r.Context(), query, next, "Staff reconciliation: "+in.Reason, id, key); err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE portal_orders SET status='pending',error_message='' WHERE id=? AND realm_key=?", id, s.c.RealmKey)
	}
	if err == nil {
		details := fmt.Sprintf("Step %s marked %s from %s: %s", key, next, current, in.Reason)
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'order.step.reconcile',?,?)", actor.ID, strconv.FormatUint(id, 10), details)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not reconcile fulfillment step")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "status": next})
}
