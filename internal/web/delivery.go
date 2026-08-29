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
		if err = s.prepareLevel80Character(ctx, o.CharacterGUID, classID); err != nil {
			s.reviewOrder(ctx, o.ID, "Items delivered, but level 80 training failed: "+err.Error())
			return
		}
	}
	if _, err = s.s.Auth.ExecContext(ctx, "UPDATE portal_orders SET status='delivered',delivered_at=NOW() WHERE id=? AND status='delivering'", o.ID); err != nil {
		slog.Error("record delivery", "order", o.ID, "error", err)
	}
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
	for _, allowed := range s.staffPermissionsFor(a.GMLevel, a.Username) {
		if allowed == permission || allowed == "admin" {
			return a, true
		}
	}
	return account{}, false
}
func (s *Server) adminRetryOrder(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
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
