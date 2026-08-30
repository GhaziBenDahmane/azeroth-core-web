package web

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var banDurationPattern = regexp.MustCompile(`^(?:-1|(?:[0-9]+[smhdwy]){1,4})$`)

func (s *Server) adminOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, http.StatusForbidden, "GM access required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_orders WHERE realm_key=?`, s.c.RealmKey).Scan(&total); err != nil {
		problem(w, 500, "Could not count orders")
		return
	}
	orderPagination := paginationMeta(page, perPage, total)
	page, offset = orderPagination.Page, (orderPagination.Page-1)*perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT o.id,a.username,o.character_guid,p.name,o.total,o.status,o.attempts,o.error_message,o.created_at,
		(SELECT COUNT(*) FROM portal_order_steps os WHERE os.order_id=o.id),
		(SELECT COUNT(*) FROM portal_order_steps os WHERE os.order_id=o.id AND os.status='completed')
		FROM portal_orders o JOIN portal_products p ON p.id=o.product_id JOIN `+"`"+s.c.AuthDB+"`"+`.account a ON a.id=o.account_id WHERE o.realm_key=? ORDER BY o.id DESC LIMIT ? OFFSET ?`, s.c.RealmKey, perPage, offset)
	if err != nil {
		problem(w, 500, "Could not load orders")
		return
	}
	defer rows.Close()
	type row struct {
		ID             uint64    `json:"id"`
		Username       string    `json:"username"`
		CharacterGUID  uint32    `json:"characterGuid"`
		Product        string    `json:"product"`
		Total          uint32    `json:"total"`
		Status         string    `json:"status"`
		Attempts       uint32    `json:"attempts"`
		Error          string    `json:"error"`
		Created        time.Time `json:"created"`
		Steps          uint32    `json:"steps"`
		CompletedSteps uint32    `json:"completedSteps"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Username, &x.CharacterGUID, &x.Product, &x.Total, &x.Status, &x.Attempts, &x.Error, &x.Created, &x.Steps, &x.CompletedSteps) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"orders": out, "pagination": orderPagination})
}

func (s *Server) adminLedger(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_credit_ledger`).Scan(&total); err != nil {
		problem(w, 500, "Could not count credit ledger")
		return
	}
	ledgerPagination := paginationMeta(page, perPage, total)
	page, offset = ledgerPagination.Page, (ledgerPagination.Page-1)*perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT l.id,COALESCE(actor.username,'SYSTEM'),target.username,l.amount,l.reason,l.created_at FROM portal_credit_ledger l LEFT JOIN `+"`"+s.c.AuthDB+"`"+`.account actor ON actor.id=l.actor_account_id JOIN `+"`"+s.c.AuthDB+"`"+`.account target ON target.id=l.target_account_id ORDER BY l.id DESC LIMIT ? OFFSET ?`, perPage, offset)
	if err != nil {
		problem(w, 500, "Could not load credit ledger")
		return
	}
	defer rows.Close()
	type row struct {
		ID      uint64    `json:"id"`
		Actor   string    `json:"actor"`
		Target  string    `json:"target"`
		Amount  int32     `json:"amount"`
		Reason  string    `json:"reason"`
		Created time.Time `json:"created"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		if rows.Scan(&x.ID, &x.Actor, &x.Target, &x.Amount, &x.Reason, &x.Created) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"entries": out, "pagination": ledgerPagination})
}

func (s *Server) adminProducts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	page, perPage, offset := requestPage(r, 50, 200)
	where, args := " WHERE realm_key=?", []any{s.c.RealmKey}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Search is too long")
		return
	}
	if search != "" {
		where += " AND (name LIKE ? OR category LIKE ? OR tier_label LIKE ? OR tags LIKE ?)"
		pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(search, "\\", "\\\\"), "%", "\\%"), "_", "\\_") + "%"
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status == "active" || status == "archived" {
		where += " AND active=?"
		args = append(args, status == "active")
	}
	order := "category_order ASC,category ASC,id DESC"
	if column, exists := map[string]string{"id": "id", "name": "name", "category": "category", "price": "price", "active": "active"}[r.URL.Query().Get("sort")]; exists {
		direction := "ASC"
		if strings.EqualFold(r.URL.Query().Get("direction"), "desc") {
			direction = "DESC"
		}
		order = column + " " + direction + ",id DESC"
	}
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_products"+where, args...).Scan(&total); err != nil {
		problem(w, 500, "Could not count products")
		return
	}
	productPagination := paginationMeta(page, perPage, total)
	page, offset = productPagination.Page, (productPagination.Page-1)*perPage
	query := "SELECT id,name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order,tags,visibility_segment,variant_required,bundle_template_id FROM portal_products" + where + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any{}, args...), perPage, offset)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, queryArgs...)
	if err != nil {
		problem(w, 500, "Could not load products")
		return
	}
	defer rows.Close()
	out := []product{}
	for rows.Next() {
		var x product
		if rows.Scan(&x.ID, &x.Name, &x.Description, &x.ItemID, &x.Quantity, &x.Price, &x.Category, &x.ImageURL, &x.ClassID, &x.Tier, &x.ServiceLevel, &x.Gold, &x.ServiceAction, &x.Active, &x.StartsAt, &x.EndsAt, &x.PerAccountLimit, &x.Featured, &x.SalePrice, &x.StockLimit, &x.SoldCount, &x.CategoryOrder, &x.Tags, &x.Visibility, &x.VariantRequired, &x.BundleID) == nil {
			out = append(out, x)
		}
	}
	if err = s.loadProductMerchandising(r.Context(), out); err != nil {
		problem(w, http.StatusInternalServerError, "Could not load product variants")
		return
	}
	jsonOut(w, 200, map[string]any{"products": out, "schema": fmt.Sprintf("%s.portal_products", s.c.AuthDB), "pagination": productPagination})
}

func (s *Server) adminAccounts(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "players"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 64 || strings.ContainsAny(query, "\r\n") {
		problem(w, 422, "Invalid account search")
		return
	}
	escaped := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(query, "\\", "\\\\"), "%", "\\%"), "_", "\\_")
	pattern := "%" + escaped + "%"
	page, perPage, offset := requestPage(r, 25, 100)
	var total int
	if err := s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf(`SELECT COUNT(*) FROM %s.account a WHERE a.username LIKE ? OR a.email LIKE ?`, s.c.AuthDB), pattern, pattern).Scan(&total); err != nil {
		problem(w, 500, "Could not count accounts")
		return
	}
	accountPagination := paginationMeta(page, perPage, total)
	page, offset = accountPagination.Page, (accountPagination.Page-1)*perPage
	rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT a.id,a.username,a.email,a.locked,
		EXISTS(SELECT 1 FROM %s.account_banned b WHERE b.id=a.id AND b.active=1),
		COALESCE((SELECT b.unbandate FROM %s.account_banned b WHERE b.id=a.id AND b.active=1 ORDER BY b.bandate DESC LIMIT 1),0),
		COALESCE((SELECT b.banreason FROM %s.account_banned b WHERE b.id=a.id AND b.active=1 ORDER BY b.bandate DESC LIMIT 1),'')
		FROM %s.account a WHERE a.username LIKE ? OR a.email LIKE ? ORDER BY a.id DESC LIMIT ? OFFSET ?`, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB, s.c.AuthDB), pattern, pattern, perPage, offset)
	if err != nil {
		problem(w, 500, "Could not search accounts")
		return
	}
	defer rows.Close()
	type adminCharacter struct {
		Name   string `json:"name"`
		Level  uint8  `json:"level"`
		Class  uint8  `json:"class"`
		Online bool   `json:"online"`
	}
	type adminAccount struct {
		ID         uint32           `json:"id"`
		Username   string           `json:"username"`
		Email      string           `json:"email"`
		Locked     bool             `json:"locked"`
		Banned     bool             `json:"banned"`
		BanUntil   uint64           `json:"banUntil"`
		BanReason  string           `json:"banReason"`
		Characters []adminCharacter `json:"characters"`
	}
	accounts := []adminAccount{}
	for rows.Next() {
		var a adminAccount
		if rows.Scan(&a.ID, &a.Username, &a.Email, &a.Locked, &a.Banned, &a.BanUntil, &a.BanReason) == nil {
			a.Characters = []adminCharacter{}
			accounts = append(accounts, a)
		}
	}
	if len(accounts) > 0 {
		ids, args, indexes := make([]uint32, 0, len(accounts)), make([]any, 0, len(accounts)), map[uint32]int{}
		for index := range accounts {
			ids = append(ids, accounts[index].ID)
			args = append(args, accounts[index].ID)
			indexes[accounts[index].ID] = index
		}
		charQuery := fmt.Sprintf("SELECT account,name,level,class,online FROM %s.characters WHERE account IN (%s) AND deleteDate IS NULL ORDER BY account,level DESC,name", s.c.CharactersDB, placeholders(len(ids)))
		if chars, charErr := s.s.Characters.QueryContext(r.Context(), charQuery, args...); charErr == nil {
			perAccount := map[uint32]int{}
			for chars.Next() {
				var accountID uint32
				var c adminCharacter
				if chars.Scan(&accountID, &c.Name, &c.Level, &c.Class, &c.Online) == nil && perAccount[accountID] < 20 {
					if index, exists := indexes[accountID]; exists {
						accounts[index].Characters = append(accounts[index].Characters, c)
						perAccount[accountID]++
					}
				}
			}
			chars.Close()
		}
	}
	jsonOut(w, 200, map[string]any{"accounts": accounts, "pagination": accountPagination})
}

func (s *Server) adminRevokeAccountSessions(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "moderation")
	if !ok {
		problem(w, http.StatusForbidden, "Moderation access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid account")
		return
	}
	var username string
	if err = s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf("SELECT username FROM %s.account WHERE id=?", s.c.AuthDB), id).Scan(&username); err != nil {
		if err == sql.ErrNoRows {
			problem(w, http.StatusNotFound, "Account not found")
		} else {
			problem(w, http.StatusInternalServerError, "Could not load account")
		}
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE account_id=?", id)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not revoke account sessions")
		return
	}
	revoked, _ := result.RowsAffected()
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'account.sessions.revoke',?,?)", actor.ID, username, fmt.Sprintf("Revoked %d portal sessions", revoked))
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "accountId": id, "username": username, "revoked": revoked, "requestId": RequestID(r.Context())})
}

func (s *Server) adminModeration(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action, Target, Duration, Reason string
		Level, RealmID                   int
	}
	if !decode(w, r, &in) {
		return
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	in.Target = strings.TrimSpace(in.Target)
	in.Duration = strings.ToLower(strings.TrimSpace(in.Duration))
	in.Reason = strings.TrimSpace(in.Reason)
	actor, ok := s.requireStaffPermission(r, moderationPermission(in.Action))
	if !ok {
		problem(w, 403, "Insufficient staff permission")
		return
	}
	if !validModerationReason(in.Reason) {
		problem(w, 422, "Reason must be 3–255 characters without quotes or line breaks")
		return
	}
	var command string
	useStartWebhook := false
	var targetAccountID uint32
	switch in.Action {
	case "ban", "unban":
		if !validAccountName(in.Target) {
			problem(w, 422, "Enter a valid account name")
			return
		}
		if err := s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf("SELECT id,username FROM %s.account WHERE username=?", s.c.AuthDB), strings.ToUpper(in.Target)).Scan(&targetAccountID, &in.Target); err != nil {
			problem(w, 404, "Account not found")
			return
		}
		if targetAccountID == actor.ID {
			problem(w, 409, "You cannot moderate your own account")
			return
		}
		if in.Action == "ban" {
			if !banDurationPattern.MatchString(in.Duration) {
				problem(w, 422, "Duration must look like 30m, 7d, 1w, or -1 for permanent")
				return
			}
			command = fmt.Sprintf("ban account %s %s %s", in.Target, in.Duration, in.Reason)
		} else {
			in.Duration = ""
			command = "unban account " + in.Target
		}
	case "kick", "mute", "unmute":
		if !validCharacterName(in.Target) {
			problem(w, 422, "Enter a valid character name")
			return
		}
		charQuery := fmt.Sprintf("SELECT account,name FROM %s.characters WHERE name=? AND deleteDate IS NULL", s.c.CharactersDB)
		if err := s.s.Characters.QueryRowContext(r.Context(), charQuery, in.Target).Scan(&targetAccountID, &in.Target); err != nil {
			problem(w, 404, "Character not found")
			return
		}
		switch in.Action {
		case "kick":
			command = "kick " + in.Target
		case "mute":
			minutes, parseErr := strconv.Atoi(in.Duration)
			if parseErr != nil || minutes < 1 || minutes > 525600 {
				problem(w, 422, "Mute duration must be 1–525600 minutes")
				return
			}
			command = fmt.Sprintf("mute %s %d %s", in.Target, minutes, in.Reason)
		case "unmute":
			in.Duration = ""
			command = "unmute " + in.Target
		}
	case "ip_ban", "ip_unban":
		if net.ParseIP(in.Target) == nil {
			problem(w, 422, "Enter a valid IPv4 or IPv6 address")
			return
		}
		if in.Action == "ip_ban" {
			if !banDurationPattern.MatchString(in.Duration) {
				problem(w, 422, "Duration must look like 30m, 7d, 1w, or -1 for permanent")
				return
			}
			command = fmt.Sprintf("ban ip %s %s %s", in.Target, in.Duration, in.Reason)
		} else {
			in.Duration = ""
			command = "unban ip " + in.Target
		}
	case "announce":
		in.Target = "realm"
		command = "announce " + in.Reason
	case "motd":
		in.Target = "realm"
		command = "server set motd " + in.Reason
	case "gm_level":
		if !validAccountName(in.Target) || in.Level < 0 || in.Level > int(s.gmLevel(r.Context(), actor.ID)) || in.RealmID < -1 {
			problem(w, 422, "Invalid account, GM level, or realm ID")
			return
		}
		if err := s.s.Auth.QueryRowContext(r.Context(), fmt.Sprintf("SELECT id,username FROM %s.account WHERE username=?", s.c.AuthDB), strings.ToUpper(in.Target)).Scan(&targetAccountID, &in.Target); err != nil {
			problem(w, 404, "Account not found")
			return
		}
		if targetAccountID == actor.ID {
			problem(w, 409, "You cannot change your own GM level")
			return
		}
		command = fmt.Sprintf("account set gmlevel %s %d %d", in.Target, in.Level, in.RealmID)
	case "restart", "shutdown":
		delay, parseErr := strconv.Atoi(in.Duration)
		if parseErr != nil || delay < 10 || delay > 3600 {
			problem(w, 422, "Server delay must be 10–3600 seconds")
			return
		}
		in.Target = "realm"
		command = fmt.Sprintf("server %s %d", in.Action, delay)
	case "cancel_shutdown":
		in.Target, in.Duration = "realm", ""
		command = "server shutdown cancel"
	case "start":
		in.Target, in.Duration = "realm", ""
		useStartWebhook = true
	default:
		problem(w, 422, "Unsupported moderation action")
		return
	}
	if !useStartWebhook && !s.soap.Enabled() {
		problem(w, 503, "Realm administration is not configured")
		return
	}
	logResult, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_moderation_log(actor_account_id,target_account_id,realm_key,target,action,duration,reason,status) VALUES(?,?,?,?,?,?,?,'review')", actor.ID, targetAccountID, s.c.RealmKey, in.Target, in.Action, in.Duration, in.Reason)
	if err != nil {
		problem(w, 500, "Could not create moderation audit entry")
		return
	}
	logID, _ := logResult.LastInsertId()
	if useStartWebhook {
		err = s.startRealm(r)
	} else {
		_, err = s.soap.Command(r.Context(), command)
	}
	if err != nil {
		message := err.Error()
		if len(message) > 500 {
			message = message[:500]
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_moderation_log SET error_message=? WHERE id=?", message, logID)
		problem(w, 502, "Realm response was uncertain; verify status before retrying")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_moderation_log SET status='executed' WHERE id=?", logID)
	s.recordSanction(r, logID, actor.ID, targetAccountID, in.Target, in.Action, in.Duration, in.Reason)
	s.notifyDiscordAsync("Moderation action", "**%s** applied `%s` to **%s** on **%s**. Reason: %s", actor.Username, in.Action, in.Target, s.c.RealmName, in.Reason)
	jsonOut(w, 200, map[string]any{"ok": true, "action": in.Action, "target": in.Target})
}

func moderationPermission(action string) string {
	switch action {
	case "start", "announce", "motd", "gm_level", "restart", "shutdown", "cancel_shutdown":
		return "realm"
	default:
		return "moderation"
	}
}

func (s *Server) adminModerationLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "audit"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), fmt.Sprintf(`SELECT m.id,a.username,m.target,m.action,m.duration,m.reason,m.status,m.error_message,m.created_at
		FROM portal_moderation_log m JOIN %s.account a ON a.id=m.actor_account_id WHERE m.realm_key=? ORDER BY m.id DESC LIMIT 100`, s.c.AuthDB), s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not load moderation history")
		return
	}
	defer rows.Close()
	type entry struct {
		ID                                                     uint64 `json:"id"`
		Actor, Target, Action, Duration, Reason, Status, Error string
		Created                                                time.Time
	}
	out := []entry{}
	for rows.Next() {
		var x entry
		if rows.Scan(&x.ID, &x.Actor, &x.Target, &x.Action, &x.Duration, &x.Reason, &x.Status, &x.Error, &x.Created) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"entries": out})
}

func (s *Server) startRealm(r *http.Request) error {
	if s.c.RealmStartWebhook == "" {
		return fmt.Errorf("REALM_START_WEBHOOK is not configured")
	}
	u, err := url.ParseRequestURI(s.c.RealmStartWebhook)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("invalid REALM_START_WEBHOOK")
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, u.String(), strings.NewReader("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.c.RealmControlToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.c.RealmControlToken)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("start webhook returned %s", response.Status)
	}
	return nil
}

func validAccountName(value string) bool {
	if len(value) < 3 || len(value) > 32 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validCharacterName(value string) bool {
	if len(value) < 2 || len(value) > 24 {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func validModerationReason(value string) bool {
	if len(value) < 3 || len(value) > 255 || strings.ContainsAny(value, "\"\\") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}
