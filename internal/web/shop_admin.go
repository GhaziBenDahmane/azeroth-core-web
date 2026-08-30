package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type catalogItem struct {
	ItemID        uint32 `json:"itemId"`
	Name          string `json:"name"`
	Quality       uint8  `json:"quality"`
	ItemLevel     uint16 `json:"itemLevel"`
	RequiredLevel uint8  `json:"requiredLevel"`
	InventoryType uint8  `json:"inventoryType"`
	DisplayID     uint32 `json:"displayId"`
}

func validateManagedProduct(p product) error {
	if strings.TrimSpace(p.Name) == "" || len(p.Name) > 100 || len(p.Description) > 500 || p.Price == 0 {
		return fmt.Errorf("name, price, and valid field lengths are required")
	}
	if p.Price > 10_000_000 || (p.SalePrice >= p.Price && p.SalePrice != 0) || p.Quantity > 1000 || strings.TrimSpace(p.Category) == "" || len(p.Category) > 40 || len(p.Tier) > 30 || p.PerAccountLimit > 100_000 || p.StockLimit > 1_000_000 || p.ServiceLevel > 80 {
		return fmt.Errorf("product price, quantity, category, or purchase limit is out of range")
	}
	if p.ClassID == 10 || p.ClassID > 11 {
		return fmt.Errorf("invalid WotLK class restriction")
	}
	if p.ItemID == 0 && len(p.Items) == 0 && p.BundleID == 0 && p.ServiceLevel == 0 && p.Gold == 0 && p.ServiceAction == "" {
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
	if len(p.Items) > 48 {
		return fmt.Errorf("a package supports at most 48 distinct items")
	}
	if p.Visibility == "" {
		p.Visibility = "all"
	}
	if p.Visibility != "all" && p.Visibility != "new" && p.Visibility != "returning" && p.Visibility != "veteran" {
		return fmt.Errorf("invalid visibility segment")
	}
	if len(p.Tags) > 500 || len(p.Variants) > 50 {
		return fmt.Errorf("too many tags or variants")
	}
	variantSKUs := map[string]bool{}
	for _, variant := range p.Variants {
		sku := strings.ToLower(strings.TrimSpace(variant.SKU))
		if strings.TrimSpace(variant.Name) == "" || len(variant.Name) > 100 || sku == "" || len(sku) > 80 || variantSKUs[sku] || int64(p.Price)+int64(variant.PriceAdjustment) < 0 || len(variant.Items) > 48 {
			return fmt.Errorf("variant names, SKUs, prices, and items must be valid")
		}
		variantSKUs[sku] = true
		seenVariantItems := map[uint32]bool{}
		for _, item := range variant.Items {
			if item.ItemID == 0 || item.Quantity == 0 || item.Quantity > 1000 || seenVariantItems[item.ItemID] {
				return fmt.Errorf("variant item IDs and quantities must be valid and unique")
			}
			seenVariantItems[item.ItemID] = true
		}
	}
	if p.VariantRequired && len(p.Variants) == 0 {
		return fmt.Errorf("a required variant product needs at least one variant")
	}
	seen := map[uint32]bool{}
	for _, item := range p.Items {
		if seen[item.ItemID] {
			return fmt.Errorf("duplicate bundle item %d", item.ItemID)
		}
		seen[item.ItemID] = true
	}
	return nil
}

func (s *Server) validateProductItems(ctx context.Context, p product) error {
	ids := []uint32{}
	if p.BundleID > 0 {
		items, err := s.loadBundleItems(ctx, p.BundleID)
		if err != nil {
			return fmt.Errorf("reusable bundle does not exist for this realm")
		}
		for _, item := range items {
			ids = append(ids, item.ItemID)
		}
	}
	if p.ItemID > 0 {
		ids = append(ids, p.ItemID)
	}
	for _, item := range p.Items {
		ids = append(ids, item.ItemID)
	}
	for _, variant := range p.Variants {
		for _, item := range variant.Items {
			ids = append(ids, item.ItemID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	unique := map[uint32]bool{}
	args := []any{}
	for _, id := range ids {
		if !unique[id] {
			unique[id] = true
			args = append(args, id)
		}
	}
	q := fmt.Sprintf("SELECT COUNT(*) FROM `%s`.item_template WHERE entry IN (?%s)", s.c.WorldDB, strings.Repeat(",?", len(args)-1))
	var count int
	if err := s.s.World.QueryRowContext(ctx, q, args...).Scan(&count); err != nil {
		return fmt.Errorf("could not validate catalog items")
	}
	if count != len(args) {
		return fmt.Errorf("one or more item IDs do not exist in the configured WotLK world database")
	}
	return nil
}

func (s *Server) adminItemSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	if s.c.MockMode {
		s.mockAdminItemSearch(w, r)
		return
	}
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(term) < 2 || len(term) > 64 || strings.ContainsAny(term, "\r\n\x00") {
		problem(w, 422, "Search requires 2–64 characters")
		return
	}
	numeric, _ := strconv.ParseUint(term, 10, 32)
	pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(term, "\\", "\\\\"), "%", "\\%"), "_", "\\_") + "%"
	q := fmt.Sprintf(`SELECT entry,name,Quality,ItemLevel,RequiredLevel,InventoryType,displayid FROM %s.item_template WHERE entry=? OR name LIKE ? ORDER BY (entry=?) DESC,ItemLevel DESC,name LIMIT 20`, s.c.WorldDB)
	rows, e := s.s.World.QueryContext(r.Context(), q, uint32(numeric), pattern, uint32(numeric))
	if e != nil {
		problem(w, 500, "Could not search world items")
		return
	}
	defer rows.Close()
	out := []catalogItem{}
	for rows.Next() {
		var x catalogItem
		if rows.Scan(&x.ItemID, &x.Name, &x.Quality, &x.ItemLevel, &x.RequiredLevel, &x.InventoryType, &x.DisplayID) == nil {
			out = append(out, x)
		}
	}
	jsonOut(w, 200, map[string]any{"items": out})
}

func (s *Server) adminProductDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, 403, "GM access required")
		return
	}
	id, e := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if e != nil {
		problem(w, 400, "Invalid product")
		return
	}
	if s.c.MockMode {
		s.mockAdminProductDetail(w, r)
		return
	}
	p, e := s.loadManagedProduct(r.Context(), uint32(id))
	if e == sql.ErrNoRows {
		problem(w, 404, "Product not found")
		return
	}
	if e != nil {
		problem(w, 500, "Could not load product")
		return
	}
	jsonOut(w, 200, map[string]any{"product": p})
}

func (s *Server) loadManagedProduct(ctx context.Context, id uint32) (product, error) {
	var p product
	err := s.s.Auth.QueryRowContext(ctx, "SELECT id,name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order,tags,visibility_segment,variant_required,bundle_template_id FROM portal_products WHERE id=? AND realm_key=?", id, s.c.RealmKey).Scan(&p.ID, &p.Name, &p.Description, &p.ItemID, &p.Quantity, &p.Price, &p.Category, &p.ImageURL, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold, &p.ServiceAction, &p.Active, &p.StartsAt, &p.EndsAt, &p.PerAccountLimit, &p.Featured, &p.SalePrice, &p.StockLimit, &p.SoldCount, &p.CategoryOrder, &p.Tags, &p.Visibility, &p.VariantRequired, &p.BundleID)
	if err != nil {
		return p, err
	}
	rows, e := s.s.Auth.QueryContext(ctx, "SELECT item_id,quantity FROM portal_product_items WHERE product_id=? ORDER BY item_id", id)
	if e != nil {
		return p, e
	}
	for rows.Next() {
		var item bundleItem
		if rows.Scan(&item.ItemID, &item.Quantity) == nil {
			p.Items = append(p.Items, item)
		}
	}
	rows.Close()
	if len(p.Items) == 0 && p.ItemID > 0 {
		p.Items = []bundleItem{{ItemID: p.ItemID, Quantity: p.Quantity}}
	}
	if p.BundleID > 0 {
		bundleItems, bundleErr := s.loadBundleItems(ctx, p.BundleID)
		if bundleErr != nil {
			return p, bundleErr
		}
		p.Items = append(p.Items, bundleItems...)
	}
	if e = s.enrichBundleItems(ctx, p.Items); e != nil {
		return p, e
	}
	products := []product{p}
	if e = s.loadProductMerchandising(ctx, products); e != nil {
		return p, e
	}
	p = products[0]
	return p, nil
}

func (s *Server) enrichBundleItems(ctx context.Context, items []bundleItem) error {
	if len(items) == 0 {
		return nil
	}
	args := make([]any, len(items))
	byID := map[uint32]*bundleItem{}
	for i := range items {
		args[i] = items[i].ItemID
		byID[items[i].ItemID] = &items[i]
	}
	q := fmt.Sprintf("SELECT entry,name,Quality,ItemLevel,RequiredLevel,InventoryType,displayid FROM `%s`.item_template WHERE entry IN (?%s)", s.c.WorldDB, strings.Repeat(",?", len(args)-1))
	rows, e := s.s.World.QueryContext(ctx, q, args...)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var x catalogItem
		if rows.Scan(&x.ItemID, &x.Name, &x.Quality, &x.ItemLevel, &x.RequiredLevel, &x.InventoryType, &x.DisplayID) == nil {
			if item := byID[x.ItemID]; item != nil {
				item.Name = x.Name
				item.Quality = x.Quality
				item.ItemLevel = x.ItemLevel
				item.RequiredLevel = x.RequiredLevel
				item.InventoryType = x.InventoryType
				item.DisplayID = x.DisplayID
			}
		}
	}
	return rows.Err()
}

func (s *Server) adminProductUpdate(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
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
	p.Tags, p.Visibility = strings.TrimSpace(p.Tags), strings.ToLower(strings.TrimSpace(p.Visibility))
	if p.Visibility == "" {
		p.Visibility = "all"
	}
	if e := validateManagedProduct(p); e != nil {
		problem(w, 422, e.Error())
		return
	}
	if e := s.validateProductItems(r.Context(), p); e != nil {
		problem(w, 422, e.Error())
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var exists int
	if e = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_products WHERE id=? AND realm_key=? FOR UPDATE", id, s.c.RealmKey).Scan(&exists); e != nil || exists == 0 {
		problem(w, 404, "Product not found")
		return
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE portal_products SET name=?,description=?,item_id=?,quantity=?,price=?,category=?,image_url=?,class_id=?,tier_label=?,service_level=?,gold_amount=?,service_action=?,active=?,starts_at=?,ends_at=?,per_account_limit=?,featured=?,sale_price=?,stock_limit=?,category_order=?,tags=?,visibility_segment=?,variant_required=?,bundle_template_id=? WHERE id=? AND realm_key=?`, p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ImageURL, p.ClassID, p.Tier, p.ServiceLevel, p.Gold, p.ServiceAction, p.Active, p.StartsAt, p.EndsAt, p.PerAccountLimit, p.Featured, p.SalePrice, p.StockLimit, p.CategoryOrder, p.Tags, p.Visibility, p.VariantRequired, p.BundleID, id, s.c.RealmKey)
	if e != nil {
		problem(w, 500, "Could not update product")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "DELETE FROM portal_product_items WHERE product_id=?", id); e != nil {
		problem(w, 500, "Could not update product items")
		return
	}
	for _, item := range p.Items {
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)", id, item.ItemID, item.Quantity); e != nil {
			problem(w, 500, "Could not update product items")
			return
		}
	}
	if e = saveProductVariants(r.Context(), tx, uint32(id), p.Variants); e != nil {
		problem(w, 500, "Could not update product variants")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'product.update',?,?)", a.ID, strconv.FormatUint(id, 10), p.Name); e != nil {
		problem(w, 500, "Could not audit product update")
		return
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "Could not update product")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminProductDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
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
	res, e := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_products SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
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

func (s *Server) applyCoupon(r *http.Request, tx *sql.Tx, accountID uint32, raw string, subtotal uint32, onSale bool, category string) (uint32, uint64, string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if code == "" {
		return 0, 0, "", nil
	}
	if !couponCodePattern.MatchString(code) {
		return 0, 0, "", fmt.Errorf("invalid coupon")
	}
	var c coupon
	err := tx.QueryRowContext(r.Context(), `SELECT id,code,discount_percent,discount_credits,max_uses,per_account_limit,allow_sale,min_subtotal,category FROM portal_coupons WHERE code=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) FOR UPDATE`, code).Scan(&c.ID, &c.Code, &c.DiscountPercent, &c.DiscountCredits, &c.MaxUses, &c.PerAccountLimit, &c.AllowSale, &c.MinSubtotal, &c.Category)
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
	if onSale && !c.AllowSale {
		return 0, 0, "", fmt.Errorf("coupon cannot be combined with a sale price")
	}
	if subtotal < c.MinSubtotal {
		return 0, 0, "", fmt.Errorf("coupon requires a subtotal of at least %d credits", c.MinSubtotal)
	}
	if c.Category != "" && !strings.EqualFold(strings.TrimSpace(c.Category), strings.TrimSpace(category)) {
		return 0, 0, "", fmt.Errorf("coupon is not valid for this product category")
	}
	discount := uint32(uint64(subtotal)*uint64(c.DiscountPercent)/100) + c.DiscountCredits
	if discount > subtotal {
		discount = subtotal
	}
	return discount, c.ID, c.Code, nil
}

func (s *Server) adminCoupons(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	if r.Method == http.MethodGet {
		page, perPage, offset := requestPage(r, 25, 100)
		state, search := strings.TrimSpace(r.URL.Query().Get("status")), strings.TrimSpace(r.URL.Query().Get("q"))
		if state != "" && state != "active" && state != "disabled" || len(search) > 100 {
			problem(w, http.StatusUnprocessableEntity, "Invalid coupon filters")
			return
		}
		if s.c.MockMode {
			s.mock.mu.Lock()
			out := append([]coupon{}, s.mock.coupons...)
			s.mock.mu.Unlock()
			filtered := out[:0]
			for _, item := range out {
				if state == "active" && !item.Active || state == "disabled" && item.Active || search != "" && !strings.Contains(strings.ToLower(item.Code+" "+item.Category), strings.ToLower(search)) {
					continue
				}
				filtered = append(filtered, item)
			}
			filtered, meta := slicePage(filtered, page, perPage)
			jsonOut(w, 200, map[string]any{"coupons": filtered, "pagination": meta})
			return
		}
		where, args := " WHERE 1=1", []any{}
		if state != "" {
			where += " AND c.active=?"
			args = append(args, state == "active")
		}
		if search != "" {
			where += " AND (c.code LIKE ? OR c.category LIKE ?)"
			pattern := likePattern(search)
			args = append(args, pattern, pattern)
		}
		var total int
		if e := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_coupons c"+where, args...).Scan(&total); e != nil {
			problem(w, 500, "Could not count coupons")
			return
		}
		meta := paginationMeta(page, perPage, total)
		offset = (meta.Page - 1) * perPage
		rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT c.id,c.code,c.discount_percent,c.discount_credits,c.max_uses,c.per_account_limit,c.starts_at,c.ends_at,c.active,c.allow_sale,c.min_subtotal,c.category,(SELECT COUNT(*) FROM portal_coupon_uses u WHERE u.coupon_id=c.id) FROM portal_coupons c"+where+" ORDER BY c.id DESC LIMIT ? OFFSET ?", append(args, perPage, offset)...)
		if e != nil {
			problem(w, 500, "Could not load coupons")
			return
		}
		defer rows.Close()
		out := []coupon{}
		for rows.Next() {
			var c coupon
			if rows.Scan(&c.ID, &c.Code, &c.DiscountPercent, &c.DiscountCredits, &c.MaxUses, &c.PerAccountLimit, &c.StartsAt, &c.EndsAt, &c.Active, &c.AllowSale, &c.MinSubtotal, &c.Category, &c.Uses) == nil {
				out = append(out, c)
			}
		}
		jsonOut(w, 200, map[string]any{"coupons": out, "pagination": meta})
		return
	}
	var c coupon
	if !decode(w, r, &c) {
		return
	}
	c.Code = strings.ToUpper(strings.TrimSpace(c.Code))
	c.Category = strings.TrimSpace(c.Category)
	if !couponCodePattern.MatchString(c.Code) || (c.DiscountPercent == 0 && c.DiscountCredits == 0) || c.DiscountPercent > 100 || c.DiscountCredits > 10_000_000 || c.MaxUses > 10_000_000 || c.PerAccountLimit > 100_000 || c.MinSubtotal > 10_000_000 || len(c.Category) > 40 || (c.StartsAt != nil && c.EndsAt != nil && !c.EndsAt.After(*c.StartsAt)) {
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
		tx, txErr := s.s.Auth.BeginTx(r.Context(), nil)
		if txErr != nil {
			problem(w, 503, "Database unavailable")
			return
		}
		defer tx.Rollback()
		res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_coupons(code,discount_percent,discount_credits,starts_at,ends_at,max_uses,per_account_limit,active,created_by,allow_sale,min_subtotal,category) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", c.Code, c.DiscountPercent, c.DiscountCredits, c.StartsAt, c.EndsAt, c.MaxUses, c.PerAccountLimit, true, a.ID, c.AllowSale, c.MinSubtotal, c.Category)
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
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO portal_coupon_events(coupon_id,actor_account_id,action,snapshot_json) VALUES(?,?,'created',JSON_OBJECT('code',?,'allowSale',?,'minSubtotal',?,'category',?))`, c.ID, a.ID, c.Code, c.AllowSale, c.MinSubtotal, c.Category); e != nil || tx.Commit() != nil {
			problem(w, 500, "Could not record coupon history")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'coupon.create',?)", a.ID, c.Code)
	}
	jsonOut(w, 201, c)
}

func (s *Server) adminCouponDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
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
		tx, txErr := s.s.Auth.BeginTx(r.Context(), nil)
		if txErr != nil {
			problem(w, 503, "Database unavailable")
			return
		}
		defer tx.Rollback()
		res, e := tx.ExecContext(r.Context(), "UPDATE portal_coupons SET active=0 WHERE id=?", id)
		if e != nil {
			problem(w, 500, "Could not disable coupon")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			problem(w, 404, "Coupon not found")
			return
		}
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO portal_coupon_events(coupon_id,actor_account_id,action) VALUES(?,?,'disabled')`, id, a.ID); e != nil || tx.Commit() != nil {
			problem(w, 500, "Could not record coupon history")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'coupon.disable',?)", a.ID, strconv.FormatUint(id, 10))
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminCouponHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid coupon")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"events": []map[string]any{}, "uses": []map[string]any{}})
		return
	}
	events := []map[string]any{}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,actor_account_id,action,COALESCE(snapshot_json,JSON_OBJECT()),created_at FROM portal_coupon_events WHERE coupon_id=? ORDER BY id DESC LIMIT 200`, id)
	if err != nil {
		problem(w, 500, "Could not load coupon history")
		return
	}
	for rows.Next() {
		var eventID uint64
		var actor uint32
		var action string
		var snapshot []byte
		var created time.Time
		if rows.Scan(&eventID, &actor, &action, &snapshot, &created) == nil {
			events = append(events, map[string]any{"id": eventID, "actorId": actor, "action": action, "snapshot": string(snapshot), "createdAt": created})
		}
	}
	rows.Close()
	uses := []map[string]any{}
	useRows, err := s.s.Auth.QueryContext(r.Context(), `SELECT u.account_id,u.order_id,u.created_at FROM portal_coupon_uses u WHERE u.coupon_id=? ORDER BY u.created_at DESC LIMIT 500`, id)
	if err != nil {
		problem(w, 500, "Could not load coupon usage")
		return
	}
	defer useRows.Close()
	for useRows.Next() {
		var accountID uint32
		var orderID uint64
		var created time.Time
		if useRows.Scan(&accountID, &orderID, &created) == nil {
			uses = append(uses, map[string]any{"accountId": accountID, "orderId": orderID, "createdAt": created})
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"events": events, "uses": uses})
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
	for i := range s.mock.products {
		if uint64(s.mock.products[i].ID) == id {
			p.ID = uint32(id)
			s.mock.products[i] = p
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
	for i := range s.mock.products {
		if uint64(s.mock.products[i].ID) == id {
			s.mock.products[i].Active = false
			jsonOut(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	problem(w, 404, "Product not found")
}

var mockCatalogItems = []catalogItem{
	{49284, "Reins of the Swift Spectral Tiger", 4, 40, 40, 0, 21974},
	{49623, "Shadowmourne", 5, 284, 80, 17, 34958},
	{51809, "Portable Hole", 3, 75, 0, 18, 32282},
	{41599, "Frostweave Bag", 2, 70, 0, 18, 26636},
	{51272, "Sanctified Lightsworn Headpiece", 4, 277, 80, 1, 31720},
	{51274, "Sanctified Lightsworn Shoulderplates", 4, 277, 80, 3, 31722},
	{51270, "Sanctified Lightsworn Battleplate", 4, 277, 80, 5, 31718},
	{51271, "Sanctified Lightsworn Legplates", 4, 277, 80, 7, 31719},
	{51269, "Sanctified Lightsworn Gauntlets", 4, 277, 80, 10, 31717},
	{50402, "Ashen Band of Endless Might", 4, 277, 80, 11, 30549},
	{50363, "Deathbringer's Will", 4, 277, 80, 12, 31033},
	{50730, "Glorenzelg, High-Blade of the Silver Hand", 4, 271, 80, 17, 31134},
}

func (s *Server) mockAdminItemSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	term := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if len(term) < 2 {
		problem(w, 422, "Search requires 2–64 characters")
		return
	}
	out := []catalogItem{}
	for _, item := range mockCatalogItems {
		if strings.Contains(strings.ToLower(item.Name), term) || strconv.FormatUint(uint64(item.ItemID), 10) == term {
			out = append(out, item)
		}
	}
	jsonOut(w, 200, map[string]any{"items": out})
}

func (s *Server) mockAdminProductDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.mockUser(r); !ok {
		problem(w, 401, "Sign in required")
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 32)
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	for _, original := range s.mock.products {
		if uint64(original.ID) != id {
			continue
		}
		p := original
		if len(p.Items) == 0 && p.ItemID > 0 {
			p.Items = []bundleItem{{ItemID: p.ItemID, Quantity: p.Quantity}}
		}
		for i := range p.Items {
			for _, known := range mockCatalogItems {
				if known.ItemID == p.Items[i].ItemID {
					p.Items[i].Name = known.Name
					p.Items[i].Quality = known.Quality
					p.Items[i].ItemLevel = known.ItemLevel
					p.Items[i].RequiredLevel = known.RequiredLevel
					p.Items[i].InventoryType = known.InventoryType
					p.Items[i].DisplayID = known.DisplayID
				}
			}
		}
		jsonOut(w, 200, map[string]any{"product": p})
		return
	}
	problem(w, 404, "Product not found")
}
