package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/soap"
	"github.com/example/azeroth-portal/internal/srp"
	"github.com/example/azeroth-portal/internal/store"
)

type Server struct {
	s       *store.Store
	c       config.Config
	soap    *soap.Client
	static  fs.FS
	limiter *limiter
	mock    *mockState
}
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func New(s *store.Store, c config.Config, static fs.FS) *Server {
	return &Server{s: s, c: c, soap: soap.New(c.SOAPURL, c.SOAPUser, c.SOAPPassword), static: static, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
}

func (s *Server) Handler() http.Handler {
	if s.c.MockMode {
		return s.middleware(s.mockHandler())
	}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("POST /api/auth/register", s.rate(5, time.Hour, s.register))
	m.HandleFunc("POST /api/auth/login", s.rate(10, 10*time.Minute, s.login))
	m.HandleFunc("POST /api/auth/logout", s.logout)
	m.HandleFunc("GET /api/me", s.me)
	m.HandleFunc("GET /api/characters", s.characters)
	m.HandleFunc("GET /api/armory", s.armorySearch)
	m.HandleFunc("GET /api/armory/{name}", s.armoryCharacter)
	m.HandleFunc("GET /api/arena", s.arenaRankings)
	m.HandleFunc("GET /api/progression/{name}", s.raidProgression)
	m.HandleFunc("GET /api/shop", s.shop)
	m.HandleFunc("POST /api/shop/purchase", s.rate(10, time.Minute, s.purchase))
	m.HandleFunc("GET /api/orders", s.orders)
	m.HandleFunc("POST /api/admin/products", s.adminProduct)
	m.HandleFunc("POST /api/admin/credits", s.rate(30, time.Minute, s.adminCredits))
	m.Handle("/", spaHandler(s.static))
	return s.middleware(m)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self' blob:; img-src 'self' https: data: blob:; style-src 'self' 'unsafe-inline' https://wow.zamimg.com; script-src 'self' https://wow.zamimg.com; connect-src 'self' https://nether.wowhead.com https://wow.zamimg.com; worker-src 'self' blob:")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			problem(w, http.StatusForbidden, "Invalid request origin")
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
func (s *Server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	a, e1 := url.Parse(o)
	b, e2 := url.Parse(s.c.PublicURL)
	return e1 == nil && e2 == nil && strings.EqualFold(a.Host, b.Host) && a.Scheme == b.Scheme
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"online": true, "realm": s.c.RealmName, "address": s.c.RealmAddress, "shopDelivery": s.soap.Enabled()})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password, Email string }
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Email = strings.ToUpper(strings.TrimSpace(in.Email))
	if err := srp.Validate(in.Username, in.Password); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if len(in.Email) > 255 || !strings.Contains(in.Email, "@") {
		problem(w, 422, "Enter a valid email address")
		return
	}
	salt, verifier, err := srp.Registration(in.Username, in.Password)
	if err != nil {
		problem(w, 500, "Could not secure account")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	q := fmt.Sprintf("INSERT INTO `%s`.account (username,salt,verifier,email,reg_mail,expansion) VALUES (?,?,?,?,?,?)", s.c.AuthDB)
	res, err := tx.ExecContext(r.Context(), q, in.Username, salt, verifier, in.Email, in.Email, s.c.Expansion)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			problem(w, 409, "That username is already taken")
		} else {
			problem(w, 500, "Could not create account")
		}
		return
	}
	id, _ := res.LastInsertId()
	_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets (account_id,balance) VALUES (?,?)", id, s.c.StartingCredits)
	if err != nil {
		problem(w, 500, "Could not initialize account")
		return
	}
	realms := fmt.Sprintf("INSERT IGNORE INTO `%s`.realmcharacters (realmid,acctid,numchars) SELECT id,?,0 FROM `%s`.realmlist", s.c.AuthDB, s.c.AuthDB)
	if _, err = tx.ExecContext(r.Context(), realms, id); err != nil {
		problem(w, 500, "Could not initialize realms")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not create account")
		return
	}
	jsonOut(w, 201, map[string]any{"ok": true, "message": "Account created. You can now sign in."})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password string }
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	var a account
	var salt, verifier []byte
	q := fmt.Sprintf("SELECT id,username,email,salt,verifier FROM `%s`.account a WHERE username=? AND locked=0 AND NOT EXISTS (SELECT 1 FROM `%s`.account_banned b WHERE b.id=a.id AND b.active=1)", s.c.AuthDB, s.c.AuthDB)
	if err := s.s.Auth.QueryRowContext(r.Context(), q, in.Username).Scan(&a.ID, &a.Username, &a.Email, &salt, &verifier); err != nil || !srp.Verify(a.Username, in.Password, salt, verifier) {
		problem(w, 401, "Invalid username or password")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		problem(w, 500, "Could not create session")
		return
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(7 * 24 * time.Hour)
	if _, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_sessions (token_hash,account_id,expires_at) VALUES (?,?,?)", hash[:], a.ID, expires); err != nil {
		problem(w, 500, "Could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: token, Path: "/", Expires: expires, MaxAge: 604800, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]any{"account": a})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("portal_session"); e == nil {
		h := sha256.Sum256([]byte(c.Value))
		_, _ = s.s.Auth.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE token_hash=?", h[:])
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) auth(r *http.Request) (account, error) {
	var a account
	c, err := r.Cookie("portal_session")
	if err != nil {
		return a, err
	}
	h := sha256.Sum256([]byte(c.Value))
	q := fmt.Sprintf("SELECT a.id,a.username,a.email FROM portal_sessions s JOIN `%s`.account a ON a.id=s.account_id WHERE s.token_hash=? AND s.expires_at>NOW() AND a.locked=0 AND NOT EXISTS (SELECT 1 FROM `%s`.account_banned b WHERE b.id=a.id AND b.active=1)", s.c.AuthDB, s.c.AuthDB)
	err = s.s.Auth.QueryRowContext(r.Context(), q, h[:]).Scan(&a.ID, &a.Username, &a.Email)
	return a, err
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance)
	a.GMLevel = s.gmLevel(r.Context(), a.ID)
	jsonOut(w, 200, map[string]any{"account": a, "balance": balance})
}

func (s *Server) characterRows(ctx context.Context, accountID uint32) ([]character, error) {
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,c.money,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.account=? AND c.deleteDate IS NULL ORDER BY c.level DESC,c.name`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(ctx, q, accountID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []character{}
	for rows.Next() {
		var c character
		if e = rows.Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Money, &c.Guild); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Server) characters(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	cs, e := s.characterRows(r.Context(), a.ID)
	if e != nil {
		problem(w, 500, "Could not load characters")
		return
	}
	jsonOut(w, 200, map[string]any{"characters": cs})
}

func (s *Server) armorySearch(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(term) > 12 {
		term = term[:12]
	}
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.deleteDate IS NULL AND c.name LIKE ? ORDER BY c.level DESC,c.name LIMIT 24`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), q, "%"+term+"%")
	if e != nil {
		problem(w, 500, "Could not search armory")
		return
	}
	defer rows.Close()
	out := []character{}
	for rows.Next() {
		var c character
		if rows.Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild) == nil {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}

func (s *Server) armoryCharacter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var c character
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.name=? AND c.deleteDate IS NULL`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	if e := s.s.Characters.QueryRowContext(r.Context(), q, name).Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild); e != nil {
		problem(w, 404, "Character not found")
		return
	}
	type item struct {
		Slot          uint8  `json:"slot"`
		Entry         uint32 `json:"entry"`
		Name          string `json:"name"`
		Quality       uint8  `json:"quality"`
		DisplayID     uint32 `json:"displayId"`
		ItemLevel     uint16 `json:"itemLevel"`
		RequiredLevel uint8  `json:"requiredLevel"`
		Armor         uint32 `json:"armor"`
		InventoryType uint8  `json:"inventoryType"`
		Icon          string `json:"icon"`
		Stats         []struct {
			Type  int16 `json:"type"`
			Value int16 `json:"value"`
		} `json:"stats"`
	}
	items := []item{}
	iq := fmt.Sprintf(`SELECT ci.slot,ii.itemEntry,it.name,it.Quality,it.displayid,it.ItemLevel,it.RequiredLevel,it.armor,it.InventoryType,it.stat_type1,it.stat_value1,it.stat_type2,it.stat_value2,it.stat_type3,it.stat_value3,it.stat_type4,it.stat_value4,it.stat_type5,it.stat_value5,it.stat_type6,it.stat_value6,it.stat_type7,it.stat_value7,it.stat_type8,it.stat_value8,it.stat_type9,it.stat_value9,it.stat_type10,it.stat_value10 FROM %s.character_inventory ci JOIN %s.item_instance ii ON ii.guid=ci.item JOIN %s.item_template it ON it.entry=ii.itemEntry WHERE ci.guid=? AND ci.bag=0 AND ci.slot<19 ORDER BY ci.slot`, s.c.CharactersDB, s.c.CharactersDB, s.c.WorldDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), iq, c.GUID)
	if e == nil {
		defer rows.Close()
		for rows.Next() {
			var i item
			var statTypes, statValues [10]int16
			if rows.Scan(&i.Slot, &i.Entry, &i.Name, &i.Quality, &i.DisplayID, &i.ItemLevel, &i.RequiredLevel, &i.Armor, &i.InventoryType,
				&statTypes[0], &statValues[0], &statTypes[1], &statValues[1], &statTypes[2], &statValues[2], &statTypes[3], &statValues[3], &statTypes[4], &statValues[4],
				&statTypes[5], &statValues[5], &statTypes[6], &statValues[6], &statTypes[7], &statValues[7], &statTypes[8], &statValues[8], &statTypes[9], &statValues[9]) == nil {
				for n := range statTypes {
					if statTypes[n] != 0 && statValues[n] != 0 {
						i.Stats = append(i.Stats, struct {
							Type  int16 `json:"type"`
							Value int16 `json:"value"`
						}{statTypes[n], statValues[n]})
					}
				}
				items = append(items, i)
			}
		}
	}
	jsonOut(w, 200, map[string]any{"character": c, "equipment": items})
}

func (s *Server) shop(w http.ResponseWriter, r *http.Request) {
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount FROM portal_products WHERE active=1 ORDER BY category,class_id,price,name")
	if e != nil {
		problem(w, 500, "Could not load shop")
		return
	}
	defer rows.Close()
	out := []product{}
	for rows.Next() {
		var p product
		if rows.Scan(&p.ID, &p.Name, &p.Description, &p.ItemID, &p.Quantity, &p.Price, &p.Category, &p.ImageURL, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold) == nil {
			out = append(out, p)
		}
	}
	jsonOut(w, 200, map[string]any{"products": out, "deliveryEnabled": s.soap.Enabled()})
}

func (s *Server) purchase(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct{ ProductID, CharacterGUID uint32 }
	if !decode(w, r, &in) {
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var p product
	if e = tx.QueryRowContext(r.Context(), "SELECT id,name,item_id,quantity,price,class_id,tier_label,service_level,gold_amount FROM portal_products WHERE id=? AND active=1 FOR UPDATE", in.ProductID).Scan(&p.ID, &p.Name, &p.ItemID, &p.Quantity, &p.Price, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold); e != nil {
		problem(w, 404, "Product not found")
		return
	}
	var balance uint32
	if e = tx.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=? FOR UPDATE", a.ID).Scan(&balance); e != nil || balance < p.Price {
		problem(w, 422, "Not enough credits")
		return
	}
	var characterName string
	var online bool
	var characterClass uint8
	cq := fmt.Sprintf("SELECT name,online,class FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if e = s.s.Characters.QueryRowContext(r.Context(), cq, in.CharacterGUID, a.ID).Scan(&characterName, &online, &characterClass); e != nil {
		problem(w, 422, "Choose one of your characters")
		return
	}
	if online {
		problem(w, 409, "Character must be offline for delivery")
		return
	}
	if p.ClassID != 0 && characterClass != p.ClassID {
		problem(w, 422, "This package does not match the selected character's class")
		return
	}
	if !s.soap.Enabled() {
		problem(w, 503, "Shop delivery is not configured")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance-? WHERE account_id=?", p.Price, a.ID); e != nil {
		problem(w, 500, "Could not debit wallet")
		return
	}
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_orders(account_id,character_guid,product_id,item_id,quantity,total) VALUES(?,?,?,?,?,?)", a.ID, in.CharacterGUID, p.ID, p.ItemID, p.Quantity, p.Price)
	if e != nil {
		problem(w, 500, "Could not create order")
		return
	}
	orderID, _ := res.LastInsertId()
	if strings.ContainsAny(characterName, " \t\r\n\"\\") {
		problem(w, 422, "Character name cannot be used for delivery")
		return
	}
	realmLabel := strings.NewReplacer("\"", "", "\\", "", "\r", " ", "\n", " ").Replace(s.c.RealmName)
	if len(realmLabel) > 80 {
		realmLabel = realmLabel[:80]
	}
	items := []bundleItem{}
	itemRows, itemErr := tx.QueryContext(r.Context(), "SELECT item_id,quantity FROM portal_product_items WHERE product_id=? ORDER BY item_id", p.ID)
	if itemErr == nil {
		for itemRows.Next() {
			var item bundleItem
			if itemRows.Scan(&item.ItemID, &item.Quantity) == nil {
				items = append(items, item)
			}
		}
		itemRows.Close()
	}
	if len(items) == 0 && p.ItemID != 0 {
		items = append(items, bundleItem{ItemID: p.ItemID, Quantity: p.Quantity})
	}
	if len(items) > 0 {
		args := make([]string, 0, len(items))
		for _, item := range items {
			args = append(args, fmt.Sprintf("%d:%d", item.ItemID, item.Quantity))
		}
		cmd := fmt.Sprintf(`send items %s "Portal order %d" "Thank you for supporting %s." %s`, characterName, orderID, realmLabel, strings.Join(args, " "))
		_, e = s.soap.Command(r.Context(), cmd)
	}
	if e == nil && p.ServiceLevel > 0 {
		_, e = s.soap.Command(r.Context(), fmt.Sprintf("character level %s %d", characterName, p.ServiceLevel))
	}
	if e == nil && p.Gold > 0 {
		_, e = s.soap.Command(r.Context(), fmt.Sprintf(`send money %s "Portal order %d" "Thank you for supporting %s." %d`, characterName, orderID, realmLabel, uint64(p.Gold)*10000))
	}
	if e != nil {
		_, _ = tx.ExecContext(r.Context(), "UPDATE portal_orders SET status='failed',error_message=? WHERE id=?", e.Error(), orderID)
		problem(w, 502, "Delivery failed; your balance was not charged")
		return
	}
	_, e = tx.ExecContext(r.Context(), "UPDATE portal_orders SET status='delivered',delivered_at=NOW() WHERE id=?", orderID)
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		problem(w, 500, "Delivery completed but order recording failed; contact staff with the current time")
		return
	}
	jsonOut(w, 201, map[string]any{"ok": true, "orderId": orderID, "message": "Item sent to the character's in-game mailbox."})
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,item_id,quantity,total,status,created_at FROM portal_orders WHERE account_id=? ORDER BY id DESC LIMIT 50", a.ID)
	if e != nil {
		problem(w, 500, "Could not load orders")
		return
	}
	defer rows.Close()
	type order struct {
		ID                      uint64 `json:"id"`
		ItemID, Quantity, Total uint32
		Status                  string
		Created                 time.Time
	}
	out := []order{}
	for rows.Next() {
		var o order
		if rows.Scan(&o.ID, &o.ItemID, &o.Quantity, &o.Total, &o.Status, &o.Created) == nil {
			out = append(out, o)
		}
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

func (s *Server) adminProduct(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if s.c.AdminToken == "" || len(provided) != len(s.c.AdminToken) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.c.AdminToken)) != 1 {
		problem(w, 401, "Admin token required")
		return
	}
	var p product
	if !decode(w, r, &p) {
		return
	}
	if p.Name == "" || p.Price == 0 || (p.ItemID == 0 && len(p.Items) == 0 && p.ServiceLevel == 0 && p.Gold == 0) || (p.ItemID != 0 && p.Quantity == 0) {
		problem(w, 422, "name, price, and an item bundle or service level are required")
		return
	}
	if p.ImageURL != "" {
		u, err := url.ParseRequestURI(p.ImageURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			problem(w, 422, "imageUrl must be an absolute HTTP or HTTPS URL")
			return
		}
	}
	if len(p.Items) > 12 {
		problem(w, 422, "A mail bundle supports at most 12 distinct items")
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	if p.Gold > 200000 {
		problem(w, 422, "Gold amount exceeds the WotLK safe limit")
		return
	}
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_products(name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount) VALUES(?,?,?,?,?,?,?,?,?,?,?)", p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ImageURL, p.ClassID, p.Tier, p.ServiceLevel, p.Gold)
	if e != nil {
		problem(w, 500, "Could not create product")
		return
	}
	id, _ := res.LastInsertId()
	for _, item := range p.Items {
		if item.ItemID == 0 || item.Quantity == 0 {
			problem(w, 422, "Bundle item IDs and quantities must be positive")
			return
		}
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)", id, item.ItemID, item.Quantity); e != nil {
			problem(w, 500, "Could not create product bundle")
			return
		}
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "Could not create product")
		return
	}
	jsonOut(w, 201, map[string]any{"id": id})
}

func (s *Server) gmLevel(ctx context.Context, accountID uint32) uint8 {
	q := fmt.Sprintf("SELECT COALESCE(MAX(gmlevel),0) FROM `%s`.account_access WHERE id=? AND (RealmID=-1 OR RealmID=?)", s.c.AuthDB)
	var level uint8
	_ = s.s.Auth.QueryRowContext(ctx, q, accountID, s.c.RealmID).Scan(&level)
	return level
}

func (s *Server) adminCredits(w http.ResponseWriter, r *http.Request) {
	actor, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	if int(s.gmLevel(r.Context(), actor.ID)) < s.c.GMLevel {
		problem(w, 403, "GM access required")
		return
	}
	var in struct {
		Username string `json:"username"`
		Amount   uint32 `json:"amount"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Username == "" || in.Amount == 0 || in.Amount > 1000000 || len(in.Reason) < 3 || len(in.Reason) > 255 {
		problem(w, 422, "Username, 1–1,000,000 credits, and a reason are required")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var targetID uint32
	q := fmt.Sprintf("SELECT id FROM `%s`.account WHERE username=?", s.c.AuthDB)
	if err = tx.QueryRowContext(r.Context(), q, in.Username).Scan(&targetID); err != nil {
		problem(w, 404, "Account not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", targetID, in.Amount); err != nil {
		problem(w, 500, "Could not update wallet")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)", actor.ID, targetID, in.Amount, in.Reason); err != nil {
		problem(w, 500, "Could not record credit grant")
		return
	}
	var balance uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", targetID).Scan(&balance); err != nil {
		problem(w, 500, "Could not read wallet")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit credit grant")
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "username": in.Username, "amount": in.Amount, "balance": balance})
}

func (s *Server) rate(max int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		key := host + ":" + r.URL.Path
		now := time.Now()
		s.limiter.mu.Lock()
		old := s.limiter.hits[key]
		keep := old[:0]
		for _, t := range old {
			if now.Sub(t) < window {
				keep = append(keep, t)
			}
		}
		allowed := len(keep) < max
		if allowed {
			keep = append(keep, now)
		}
		s.limiter.hits[key] = keep
		s.limiter.mu.Unlock()
		if !allowed {
			problem(w, 429, "Too many attempts. Try again later.")
			return
		}
		next(w, r)
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		problem(w, 400, "Invalid request body")
		return false
	}
	return true
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func spaHandler(root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Trim(strings.TrimSpace(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if info, e := fs.Stat(root, p); e == nil {
			if info.IsDir() {
				p += "/index.html"
				info, e = fs.Stat(root, p)
				if e != nil {
					http.NotFound(w, r)
					return
				}
			}
			data, readErr := fs.ReadFile(root, p)
			if readErr != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, p, info.ModTime(), bytes.NewReader(data))
			return
		}
		if strings.Contains(p, ".") {
			http.NotFound(w, r)
			return
		}
		data, e := fs.ReadFile(root, "index.html")
		if e != nil {
			http.Error(w, "UI not built", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}
