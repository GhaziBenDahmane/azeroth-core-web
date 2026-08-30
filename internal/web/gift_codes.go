package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type giftCode struct {
	ID        uint64     `json:"id"`
	CodeHash  [32]byte   `json:"-"`
	CodeHint  string     `json:"codeHint"`
	Credits   uint32     `json:"credits"`
	MaxUses   uint32     `json:"maxUses"`
	UsedCount uint32     `json:"usedCount"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"createdAt,omitempty"`
}

func normalizeGiftCode(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}
func giftCodeHash(value string) [32]byte { return sha256.Sum256([]byte(normalizeGiftCode(value))) }
func generateGiftCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := strings.ToUpper(hex.EncodeToString(raw))
	return "GIFT-" + encoded[:5] + "-" + encoded[5:10] + "-" + encoded[10:15] + "-" + encoded[15:], nil
}

func (s *Server) adminGiftCodes(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	if r.Method == http.MethodGet {
		page, perPage, offset := requestPage(r, 25, 100)
		state, search := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("q"))
		if state != "" && state != "active" && state != "disabled" || len(search) > 100 {
			problem(w, http.StatusUnprocessableEntity, "Invalid gift-code filters")
			return
		}
		if s.c.MockMode {
			s.mock.mu.Lock()
			codes := append([]giftCode(nil), s.mock.giftCodes...)
			s.mock.mu.Unlock()
			filtered := codes[:0]
			for _, code := range codes {
				if state == "active" && !code.Active || state == "disabled" && code.Active || search != "" && !strings.Contains(strings.ToLower(code.CodeHint), strings.ToLower(search)) {
					continue
				}
				filtered = append(filtered, code)
			}
			filtered, meta := slicePage(filtered, page, perPage)
			jsonOut(w, 200, map[string]any{"codes": filtered, "pagination": meta})
			return
		}
		where, args := " WHERE realm_key=?", []any{s.c.RealmKey}
		if state != "" {
			where += " AND active=?"
			args = append(args, state == "active")
		}
		if search != "" {
			where += " AND code_hint LIKE ?"
			args = append(args, likePattern(search))
		}
		var total int
		if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_gift_codes"+where, args...).Scan(&total); err != nil {
			problem(w, 500, "Could not count gift codes")
			return
		}
		meta := paginationMeta(page, perPage, total)
		offset = (meta.Page - 1) * perPage
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,code_hint,credits,max_uses,used_count,expires_at,active,created_at FROM portal_gift_codes"+where+" ORDER BY id DESC LIMIT ? OFFSET ?", append(args, perPage, offset)...)
		if err != nil {
			problem(w, 500, "Could not load gift codes")
			return
		}
		defer rows.Close()
		codes := []giftCode{}
		for rows.Next() {
			var code giftCode
			if rows.Scan(&code.ID, &code.CodeHint, &code.Credits, &code.MaxUses, &code.UsedCount, &code.ExpiresAt, &code.Active, &code.CreatedAt) == nil {
				codes = append(codes, code)
			}
		}
		jsonOut(w, 200, map[string]any{"codes": codes, "pagination": meta})
		return
	}
	var in struct {
		Credits, MaxUses uint32
		ExpiresAt        *time.Time
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Credits == 0 || in.Credits > 1000000 || in.MaxUses == 0 || in.MaxUses > 100000 {
		problem(w, 422, "Credits and maximum uses are required")
		return
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		problem(w, 422, "Expiry must be in the future")
		return
	}
	plain, err := generateGiftCode()
	if err != nil {
		problem(w, 500, "Could not generate gift code")
		return
	}
	hash := giftCodeHash(plain)
	hint := "…" + plain[len(plain)-5:]
	code := giftCode{CodeHash: hash, CodeHint: hint, Credits: in.Credits, MaxUses: in.MaxUses, ExpiresAt: in.ExpiresAt, Active: true, CreatedAt: time.Now()}
	if s.c.MockMode {
		s.mock.mu.Lock()
		code.ID = uint64(len(s.mock.giftCodes) + 1)
		s.mock.giftCodes = append([]giftCode{code}, s.mock.giftCodes...)
		s.mock.mu.Unlock()
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_gift_codes(realm_key,code_hash,code_hint,credits,max_uses,expires_at,active,created_by) VALUES(?,?,?,?,?,?,1,?)", s.c.RealmKey, hash[:], hint, in.Credits, in.MaxUses, in.ExpiresAt, a.ID)
		if err != nil {
			problem(w, 500, "Could not create gift code")
			return
		}
		id, _ := res.LastInsertId()
		code.ID = uint64(id)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'gift_code.create',?,?)", a.ID, hint, in.Credits)
	}
	jsonOut(w, 201, map[string]any{"giftCode": code, "code": plain})
}

func (s *Server) redeemGiftCode(w http.ResponseWriter, r *http.Request) {
	var a account
	if s.c.MockMode {
		username, ok := s.mockUser(r)
		if !ok {
			problem(w, 401, "Sign in required")
			return
		}
		a = account{ID: 1, Username: username}
	} else {
		var err error
		a, err = s.auth(r)
		if err != nil {
			problem(w, 401, "Sign in required")
			return
		}
	}
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	hash := giftCodeHash(in.Code)
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		key := hex.EncodeToString(hash[:])
		if s.mock.giftCodeUses == nil {
			s.mock.giftCodeUses = map[string]bool{}
		}
		if s.mock.giftCodeUses[key] {
			problem(w, 409, "This code was already redeemed by your account")
			return
		}
		for i := range s.mock.giftCodes {
			code := &s.mock.giftCodes[i]
			if code.CodeHash == hash && code.Active && (code.ExpiresAt == nil || code.ExpiresAt.After(time.Now())) && code.UsedCount < code.MaxUses {
				code.UsedCount++
				s.mock.giftCodeUses[key] = true
				s.mock.balance += code.Credits
				jsonOut(w, 200, map[string]any{"credits": code.Credits, "balance": s.mock.balance})
				return
			}
		}
		problem(w, 422, "Gift code is invalid, expired, or exhausted")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var code giftCode
	if err = tx.QueryRowContext(r.Context(), "SELECT id,credits,max_uses,used_count,expires_at,active FROM portal_gift_codes WHERE code_hash=? AND realm_key=? FOR UPDATE", hash[:], s.c.RealmKey).Scan(&code.ID, &code.Credits, &code.MaxUses, &code.UsedCount, &code.ExpiresAt, &code.Active); err != nil || !code.Active || (code.ExpiresAt != nil && !code.ExpiresAt.After(time.Now())) || code.UsedCount >= code.MaxUses {
		problem(w, 422, "Gift code is invalid, expired, or exhausted")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_gift_code_uses(gift_code_id,account_id,credits) VALUES(?,?,?)", code.ID, a.ID, code.Credits); err != nil {
		problem(w, 409, "This code was already redeemed by your account")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE portal_gift_codes SET used_count=used_count+1 WHERE id=?", code.ID); err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", a.ID, code.Credits)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)", a.ID, code.Credits, "Gift code "+code.CodeHint)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, 500, "Could not redeem gift code")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance)
	jsonOut(w, 200, map[string]any{"credits": code.Credits, "balance": balance})
}

func (s *Server) adminGiftCodeDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid gift code")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.giftCodes {
			if s.mock.giftCodes[i].ID == id {
				s.mock.giftCodes[i].Active = false
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_gift_codes SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
		if err == nil {
			if changed, _ := res.RowsAffected(); changed > 0 {
				_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'gift_code.disable',?)", a.ID, strconv.FormatUint(id, 10))
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	}
	problem(w, 404, "Gift code not found")
}
