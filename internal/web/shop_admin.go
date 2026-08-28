package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validateManagedProduct(p product) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 || len(p.Description) > 500 || p.Price == 0 {
		return fmt.Errorf("name, price, and valid field lengths are required")
	}
	if p.Price > 10_000_000 || p.Quantity > 1000 || len(p.Category) > 40 || p.PerAccountLimit > 100_000 {
		return fmt.Errorf("product price, quantity, category, or purchase limit is out of range")
	}
	if p.ItemID == 0 && len(p.Items) == 0 && p.ServiceLevel == 0 && p.Gold == 0 && p.ServiceAction == "" {
		return fmt.Errorf("an item or service is required")
	}
	if p.ItemID > 0 && p.Quantity == 0 {
		return fmt.Errorf("item quantity is required")
	}
	if p.Gold > 200000 {
		return fmt.Errorf("gold amount exceeds the WotLK safe limit")
	}
	if p.ServiceAction != "" && p.ServiceAction != "race_change" && p.ServiceAction != "faction_change" {
		return fmt.Errorf("invalid service action")
	}
	if p.StartsAt != nil && p.EndsAt != nil && !p.EndsAt.After(*p.StartsAt) {
		return fmt.Errorf("product end must be after its start")
	}
	if p.ImageURL != "" {
		u, e := url.ParseRequestURI(p.ImageURL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("invalid image URL")
		}
	}
	for _, item := range p.Items {
		if item.ItemID == 0 || item.Quantity == 0 || item.Quantity > 1000 {
			return fmt.Errorf("bundle item IDs and quantities must be valid")
		}
	}
	return nil
}

func (s *Server) adminProductUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid product")
		return
	}
	if s.c.MockMode {
		s.mockAdminProductUpdate(w, r)
		return
	}
	var p product
	if !decode(w, r, &p) {
		return
	}
	if e := validateManagedProduct(p); e != nil {
		problem(w, 422, e.Error())
		return
	}
	res, e := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_products SET name=?,description=?,item_id=?,quantity=?,price=?,category=?,image_url=?,class_id=?,tier_label=?,service_level=?,gold_amount=?,service_action=?,active=?,starts_at=?,ends_at=?,per_account_limit=? WHERE id=?`, p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ImageURL, p.ClassID, p.Tier, p.ServiceLevel, p.Gold, p.ServiceAction, p.Active, p.StartsAt, p.EndsAt, p.PerAccountLimit, id)
	if e != nil {
		problem(w, 500, "Could not update product")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "Product not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'product.update',?,?)", a.ID, strconv.FormatUint(id, 10), p.Name)
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminProductDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid product")
		return
	}
	if s.c.MockMode {
		s.mockAdminProductDelete(w, r)
		return
	}
	res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_products SET active=0 WHERE id=?", id)
	if e != nil {
		problem(w, 500, "Could not archive product")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		problem(w, 404, "Product not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'product.archive',?)", a.ID, strconv.FormatUint(id, 10))
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) applyCoupon(r *http.Request, tx *sql.Tx, accountID uint32, raw string, subtotal uint32) (uint32, uint64, string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return 0, 0, "", nil
	}
	if !couponCodePattern.MatchString(code) {
		return 0, 0, "", fmt.Errorf("invalid coupon")
	}
	var c coupon
	err := tx.QueryRowContext(r.Context(), `SELECT id,code,discount_percent,discount_credits,max_uses,per_account_limit FROM portal_coupons WHERE code=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) FOR UPDATE`, code).Scan(&c.ID, &c.Code, &c.DiscountPercent, &c.DiscountCredits, &c.MaxUses, &c.PerAccountLimit)
	if err != nil {
		return 0, 0, "", fmt.Errorf("coupon is invalid or expired")
	}
	var totalUses, accountUses uint32
	if tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_coupon_uses WHERE coupon_id=?", c.ID).Scan(&totalUses) != nil || tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_coupon_uses WHERE coupon_id=? AND account_id=?", c.ID, accountID).Scan(&accountUses) != nil {
		return 0, 0, "", fmt.Errorf("could not validate coupon")
	}
	if c.MaxUses > 0 && totalUses >= c.MaxUses {
		return 0, 0, "", fmt.Errorf("coupon usage limit reached")
	}
	if c.PerAccountLimit > 0 && accountUses >= c.PerAccountLimit {
		return 0, 0, "", fmt.Errorf("coupon already used")
	}
	discount := uint32(uint64(subtotal)*uint64(c.DiscountPercent)/100) + c.DiscountCredits
	if discount > subtotal {
		discount = subtotal
	}
	return discount, c.ID, c.Code, nil
}

func (s *Server) adminCoupons(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			out := append([]coupon{}, s.mock.coupons...)
			s.mock.mu.Unlock()
			jsonOut(w, 200, map[string]any{"coupons": out})
			return
		}
		rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,code,discount_percent,discount_credits,max_uses,per_account_limit,starts_at,ends_at,active FROM portal_coupons ORDER BY id DESC LIMIT 100")
		if e != nil {
			problem(w, 500, "Could not load coupons")
			return
		}
		defer rows.Close()
		out := []coupon{}
		for rows.Next() {
			var c coupon
			if rows.Scan(&c.ID, &c.Code, &c.DiscountPercent, &c.DiscountCredits, &c.MaxUses, &c.PerAccountLimit, &c.StartsAt, &c.EndsAt, &c.Active) == nil {
				out = append(out, c)
			}
		}
		jsonOut(w, 200, map[string]any{"coupons": out})
		return
	}
	var c coupon
	if !decode(w, r, &c) {
		return
	}
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	if !couponCodePattern.MatchString(c.Code) || (c.DiscountPercent == 0 && c.DiscountCredits == 0) || c.DiscountPercent > 100 || c.DiscountCredits > 10_000_000 || c.MaxUses > 10_000_000 || c.PerAccountLimit > 100_000 || (c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt)) {
		problem(w, 422, "Use a valid code, discount, and date range")
		return
	}
	if c.PerAccountLimit == 0 {
		c.PerAccountLimit = 1
	}
	c.Active = true
	if s.c.MockMode {
		s.mock.mu.Lock()
		c.ID = uint64(len(s.mock.coupons) + 1)
		s.mock.coupons = append([]coupon{c}, s.mock.coupons...)
		s.mock.mu.Unlock()
	} else {
		res, e := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_coupons(code,discount_percent,discount_credits,starts_at,ends_at,max_uses,per_account_limit,active,created_by) VALUES(?,?,?,?,?,?,?,?,?)", c.Code, c.DiscountPercent, c.DiscountCredits, c.StartsAt, c.EndsAt, c.MaxUses, c.PerAccountLimit, true, a.ID)
		if e != nil {
			if strings.Contains(e.Error(), "Duplicate") {
				problem(w, 409, "Coupon code already exists")
			} else {
				problem(w, 500, "Could not create coupon")
			}
			return
		}
		id, _ := res.LastInsertId()
		c.ID = uint64(id)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'coupon.create',?)", a.ID, c.Code)
	}
	jsonOut(w, 201, c)
}

func (s *Server) adminCouponDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireGM(r)
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if e != nil {
		problem(w, 400, "Invalid coupon")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		for i := range s.mock.coupons {
			if s.mock.coupons[i].ID == id {
				s.mock.coupons[i].Active = false
			}
		}
		s.mock.mu.Unlock()
	} else {
		res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_coupons SET active=0 WHERE id=?", id)
		if e != nil {
			problem(w, 500, "Could not disable coupon")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			problem(w, 404, "Coupon not found")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'coupon.disable',?)", a.ID, strconv.FormatUint(id, 10))
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) mockAdminProductUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 32)
	var p product
	if !decode(w, r, &p) {
		return
	}
	if e := validateManagedProduct(p); e != nil {
		problem(w, 422, e.Error())
		return
	}
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for i := range mockProducts {
		if uint64(mockProducts[i].ID) == id {
			p.ID = uint32(id)
			mockProducts[i] = p
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, "Product not found")
}
func (s *Server) mockAdminProductDelete(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 32)
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for i := range mockProducts {
		if uint64(mockProducts[i].ID) == id {
			mockProducts[i].Active = false
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, "Product not found")
}
