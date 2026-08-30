package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var shopSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type shopCollection struct {
	ID          uint64   `json:"id"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ImageURL    string   `json:"imageUrl"`
	Active      bool     `json:"active"`
	Featured    bool     `json:"featured"`
	SortOrder   int      `json:"sortOrder"`
	ProductIDs  []uint32 `json:"productIds"`
}

type bundleTemplate struct {
	ID          uint64       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Items       []bundleItem `json:"items"`
}

func (s *Server) loadBundleItems(ctx context.Context, bundleID uint64) ([]bundleItem, error) {
	if bundleID == 0 {
		return nil, nil
	}
	rows, err := s.s.Auth.QueryContext(ctx, `SELECT bi.item_id,bi.quantity FROM portal_bundle_template_items bi JOIN portal_bundle_templates b ON b.id=bi.bundle_id WHERE b.id=? AND b.realm_key=? ORDER BY bi.item_id`, bundleID, s.c.RealmKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []bundleItem{}
	for rows.Next() {
		var item bundleItem
		if rows.Scan(&item.ItemID, &item.Quantity) == nil {
			items = append(items, item)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return items, s.enrichBundleItems(ctx, items)
}

func (s *Server) listBundleTemplates(ctx context.Context) ([]bundleTemplate, error) {
	rows, err := s.s.Auth.QueryContext(ctx, `SELECT id,name,description FROM portal_bundle_templates WHERE realm_key=? ORDER BY name`, s.c.RealmKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []bundleTemplate{}
	for rows.Next() {
		var item bundleTemplate
		if rows.Scan(&item.ID, &item.Name, &item.Description) == nil {
			items, itemErr := s.loadBundleItems(ctx, item.ID)
			if itemErr != nil && itemErr != sql.ErrNoRows {
				return nil, itemErr
			}
			item.Items = items
			out = append(out, item)
		}
	}
	return out, rows.Err()
}

func validateBundleTemplate(bundle bundleTemplate) error {
	if strings.TrimSpace(bundle.Name) == "" || len(bundle.Name) > 100 || len(bundle.Description) > 500 || len(bundle.Items) == 0 || len(bundle.Items) > 48 {
		return fmt.Errorf("bundle name and 1–48 items are required")
	}
	seen := map[uint32]bool{}
	for _, item := range bundle.Items {
		if item.ItemID == 0 || item.Quantity == 0 || item.Quantity > 1000 || seen[item.ItemID] {
			return fmt.Errorf("bundle items must be positive and unique")
		}
		seen[item.ItemID] = true
	}
	return nil
}

func (s *Server) adminBundleTemplates(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, 200, map[string]any{"bundles": []bundleTemplate{{ID: 1, Name: "Raid consumables", Description: "Reusable raid preparation bundle", Items: []bundleItem{{ItemID: 37719, Quantity: 5}}}}})
		return
	}
	if r.Method == http.MethodGet {
		bundles, err := s.listBundleTemplates(r.Context())
		if err != nil {
			problem(w, 500, "Could not load bundle templates")
			return
		}
		jsonOut(w, 200, map[string]any{"bundles": bundles})
		return
	}
	var bundle bundleTemplate
	if !decode(w, r, &bundle) {
		return
	}
	if err := validateBundleTemplate(bundle); err != nil {
		problem(w, 422, err.Error())
		return
	}
	testProduct := product{Price: 1, Name: "validation", Category: "Bundle", Items: bundle.Items}
	if err := s.validateProductItems(r.Context(), testProduct); err != nil {
		problem(w, 422, err.Error())
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `INSERT INTO portal_bundle_templates(realm_key,name,description,created_by) VALUES(?,?,?,?)`, s.c.RealmKey, strings.TrimSpace(bundle.Name), strings.TrimSpace(bundle.Description), actor.ID)
	if err != nil {
		problem(w, 409, "Bundle template name already exists")
		return
	}
	id, _ := result.LastInsertId()
	for _, item := range bundle.Items {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_bundle_template_items(bundle_id,item_id,quantity) VALUES(?,?,?)`, id, item.ItemID, item.Quantity); err != nil {
			problem(w, 500, "Could not save bundle items")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not save bundle template")
		return
	}
	jsonOut(w, 201, map[string]any{"id": id})
}

func (s *Server) adminBundleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, 400, "Invalid bundle template")
		return
	}
	if s.c.MockMode {
		jsonOut(w, 200, map[string]bool{"ok": true})
		return
	}
	var uses uint32
	if err = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_products WHERE realm_key=? AND bundle_template_id=?`, s.c.RealmKey, id).Scan(&uses); err != nil {
		problem(w, 500, "Could not inspect bundle template")
		return
	}
	if uses > 0 {
		problem(w, 409, "Bundle template is still attached to products")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM portal_bundle_template_items WHERE bundle_id=?`, id)
	result, err := tx.ExecContext(r.Context(), `DELETE FROM portal_bundle_templates WHERE id=? AND realm_key=?`, id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not delete bundle template")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, 404, "Bundle template not found")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not delete bundle template")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminBundleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid bundle template")
		return
	}
	var bundle bundleTemplate
	if !decode(w, r, &bundle) {
		return
	}
	if err = validateBundleTemplate(bundle); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	testProduct := product{Price: 1, Name: "validation", Category: "Bundle", Items: bundle.Items}
	if err = s.validateProductItems(r.Context(), testProduct); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `UPDATE portal_bundle_templates SET name=?,description=?,updated_at=NOW() WHERE id=? AND realm_key=?`, strings.TrimSpace(bundle.Name), strings.TrimSpace(bundle.Description), id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusConflict, "Bundle template name already exists")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Bundle template not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_bundle_template_items WHERE bundle_id=?`, id); err != nil {
		problem(w, http.StatusInternalServerError, "Could not replace bundle items")
		return
	}
	for _, item := range bundle.Items {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_bundle_template_items(bundle_id,item_id,quantity) VALUES(?,?,?)`, id, item.ItemID, item.Quantity); err != nil {
			problem(w, http.StatusInternalServerError, "Could not save bundle items")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not update bundle template")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key) VALUES(?,'bundle.update',?,'Reusable bundle updated',?)`, actor.ID, strconv.FormatUint(id, 10), s.c.RealmKey)
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

type stockMovement struct {
	ID        uint64    `json:"id"`
	ProductID uint32    `json:"productId"`
	Delta     int32     `json:"delta"`
	Type      string    `json:"type"`
	Reference string    `json:"reference"`
	Reason    string    `json:"reason"`
	ActorID   uint32    `json:"actorId"`
	CreatedAt time.Time `json:"createdAt"`
}

func consolidateBundleItems(items []bundleItem) []bundleItem {
	out, positions := []bundleItem{}, map[uint32]int{}
	for _, item := range items {
		if index, found := positions[item.ItemID]; found {
			out[index].Quantity += item.Quantity
			continue
		}
		positions[item.ItemID] = len(out)
		out = append(out, item)
	}
	return out
}

func (s *Server) adminProductValidation(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, 400, "Invalid product")
		return
	}
	if s.c.MockMode {
		p, found := product{}, false
		s.mock.mu.Lock()
		for _, candidate := range s.mock.products {
			if candidate.ID == uint32(id) {
				p, found = candidate, true
				break
			}
		}
		s.mock.mu.Unlock()
		if !found {
			problem(w, 404, "Product not found")
			return
		}
		jsonOut(w, 200, map[string]any{"valid": true, "product": p.Name, "items": len(p.Items) + btoi(p.ItemID > 0), "variants": len(p.Variants), "deliveryConfigured": true, "mode": "mock", "warnings": []string{"Mock mode does not contact a worldserver."}})
		return
	}
	p, err := s.loadManagedProduct(r.Context(), uint32(id))
	if err == sql.ErrNoRows {
		problem(w, 404, "Product not found")
		return
	}
	if err != nil {
		problem(w, 500, "Could not load product")
		return
	}
	validationErr := s.validateProductItems(r.Context(), p)
	warnings := []string{}
	if !s.soap.Enabled() {
		warnings = append(warnings, "SOAP delivery is not configured.")
	}
	for _, variant := range p.Variants {
		if !variant.Active {
			warnings = append(warnings, "Variant "+variant.Name+" is archived.")
		}
	}
	if validationErr != nil {
		jsonOut(w, 200, map[string]any{"valid": false, "product": p.Name, "error": validationErr.Error(), "deliveryConfigured": s.soap.Enabled(), "warnings": warnings})
		return
	}
	jsonOut(w, 200, map[string]any{"valid": true, "product": p.Name, "items": len(p.Items), "variants": len(p.Variants), "deliveryConfigured": s.soap.Enabled(), "mode": "preflight", "warnings": warnings})
}

func btoi(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Server) shopCollections(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		s.mock.mu.Lock()
		collections := append([]shopCollection{}, s.mock.collections...)
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]any{"collections": collections})
		return
	}
	collections, err := s.listShopCollections(r.Context(), false)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load shop collections")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"collections": collections})
}

func (s *Server) filterProductsForAudience(r *http.Request, products []product) []product {
	if s.c.MockMode {
		return products
	}
	account, authErr := s.auth(r)
	purchases := uint32(0)
	if authErr == nil {
		_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_orders WHERE account_id=? AND realm_key=? AND status NOT IN ('failed','refunded')`, account.ID, s.c.RealmKey).Scan(&purchases)
	}
	out := make([]product, 0, len(products))
	for _, item := range products {
		visible := item.Visibility == "" || item.Visibility == "all" ||
			(item.Visibility == "new" && authErr == nil && purchases == 0) ||
			(item.Visibility == "returning" && authErr == nil && purchases > 0) ||
			(item.Visibility == "veteran" && authErr == nil && purchases >= 5)
		if visible {
			out = append(out, item)
		}
	}
	return out
}

func (s *Server) loadProductMerchandising(ctx context.Context, products []product) error {
	if len(products) == 0 || s.c.MockMode {
		return nil
	}
	byID, args := map[uint32]*product{}, make([]any, len(products))
	for index := range products {
		byID[products[index].ID], args[index] = &products[index], products[index].ID
	}
	query := fmt.Sprintf(`SELECT id,product_id,name,sku,price_adjustment,active,sort_order FROM portal_product_variants WHERE product_id IN (%s) ORDER BY product_id,sort_order,id`, placeholders(len(args)))
	rows, err := s.s.Auth.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	type variantLocation struct{ product, variant int }
	variantByID := map[uint64]variantLocation{}
	productIndex := map[uint32]int{}
	for index := range products {
		productIndex[products[index].ID] = index
	}
	for rows.Next() {
		var variant productVariant
		var productID uint32
		if rows.Scan(&variant.ID, &productID, &variant.Name, &variant.SKU, &variant.PriceAdjustment, &variant.Active, &variant.SortOrder) == nil && byID[productID] != nil {
			index := productIndex[productID]
			products[index].Variants = append(products[index].Variants, variant)
			variantByID[variant.ID] = variantLocation{index, len(products[index].Variants) - 1}
		}
	}
	rows.Close()
	if len(variantByID) > 0 {
		variantArgs := make([]any, 0, len(variantByID))
		for id := range variantByID {
			variantArgs = append(variantArgs, id)
		}
		itemQuery := fmt.Sprintf(`SELECT variant_id,item_id,quantity FROM portal_product_variant_items WHERE variant_id IN (%s) ORDER BY variant_id,item_id`, placeholders(len(variantArgs)))
		itemRows, itemErr := s.s.Auth.QueryContext(ctx, itemQuery, variantArgs...)
		if itemErr != nil {
			return itemErr
		}
		for itemRows.Next() {
			var variantID uint64
			var item bundleItem
			if itemRows.Scan(&variantID, &item.ItemID, &item.Quantity) == nil {
				if location, found := variantByID[variantID]; found {
					products[location.product].Variants[location.variant].Items = append(products[location.product].Variants[location.variant].Items, item)
				}
			}
		}
		itemRows.Close()
		for _, location := range variantByID {
			if err := s.enrichBundleItems(ctx, products[location.product].Variants[location.variant].Items); err != nil {
				return err
			}
		}
	}
	collectionQuery := fmt.Sprintf(`SELECT cp.product_id,c.slug FROM portal_collection_products cp JOIN portal_shop_collections c ON c.id=cp.collection_id WHERE cp.product_id IN (%s) AND c.realm_key=? AND c.active=1 ORDER BY c.sort_order,c.name`, placeholders(len(args)))
	collectionArgs := append(append([]any{}, args...), s.c.RealmKey)
	collectionRows, err := s.s.Auth.QueryContext(ctx, collectionQuery, collectionArgs...)
	if err != nil {
		return err
	}
	defer collectionRows.Close()
	for collectionRows.Next() {
		var productID uint32
		var slug string
		if collectionRows.Scan(&productID, &slug) == nil && byID[productID] != nil {
			byID[productID].Collections = append(byID[productID].Collections, slug)
		}
	}
	return collectionRows.Err()
}

func validateCollection(collection shopCollection) error {
	collection.Slug = strings.ToLower(strings.TrimSpace(collection.Slug))
	if strings.TrimSpace(collection.Name) == "" || len(collection.Name) > 100 || !shopSlugPattern.MatchString(collection.Slug) || len(collection.Description) > 500 || len(collection.ProductIDs) > 500 {
		return fmt.Errorf("collection name, slug, description, or product list is invalid")
	}
	if collection.ImageURL != "" {
		u, err := url.ParseRequestURI(collection.ImageURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return fmt.Errorf("collection image must use an absolute HTTPS URL")
		}
	}
	seen := map[uint32]bool{}
	for _, id := range collection.ProductIDs {
		if id == 0 || seen[id] {
			return fmt.Errorf("collection product IDs must be positive and unique")
		}
		seen[id] = true
	}
	return nil
}

func (s *Server) listShopCollections(ctx context.Context, includeInactive bool) ([]shopCollection, error) {
	where := "realm_key=? AND active=1"
	if includeInactive {
		where = "realm_key=?"
	}
	rows, err := s.s.Auth.QueryContext(ctx, `SELECT id,slug,name,description,image_url,active,featured,sort_order FROM portal_shop_collections WHERE `+where+` ORDER BY sort_order,name`, s.c.RealmKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, byID := []shopCollection{}, map[uint64]int{}
	for rows.Next() {
		var item shopCollection
		if rows.Scan(&item.ID, &item.Slug, &item.Name, &item.Description, &item.ImageURL, &item.Active, &item.Featured, &item.SortOrder) == nil {
			out = append(out, item)
			byID[item.ID] = len(out) - 1
		}
	}
	if len(byID) == 0 {
		return out, rows.Err()
	}
	ids, args := make([]string, 0, len(byID)), make([]any, 0, len(byID))
	for id := range byID {
		ids, args = append(ids, "?"), append(args, id)
	}
	productRows, err := s.s.Auth.QueryContext(ctx, `SELECT collection_id,product_id FROM portal_collection_products WHERE collection_id IN (`+strings.Join(ids, ",")+`) ORDER BY sort_order,product_id`, args...)
	if err != nil {
		return nil, err
	}
	defer productRows.Close()
	for productRows.Next() {
		var collectionID uint64
		var productID uint32
		if productRows.Scan(&collectionID, &productID) == nil {
			if index, found := byID[collectionID]; found {
				out[index].ProductIDs = append(out[index].ProductIDs, productID)
			}
		}
	}
	return out, productRows.Err()
}

func (s *Server) adminShopCollections(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if r.Method == http.MethodPost {
			var collection shopCollection
			if !decode(w, r, &collection) {
				return
			}
			collection.Slug = strings.ToLower(strings.TrimSpace(collection.Slug))
			if err := validateCollection(collection); err != nil {
				problem(w, 422, err.Error())
				return
			}
			collection.ID = uint64(len(s.mock.collections) + 1)
			s.mock.collections = append(s.mock.collections, collection)
			jsonOut(w, http.StatusCreated, map[string]any{"id": collection.ID})
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"collections": append([]shopCollection{}, s.mock.collections...)})
		return
	}
	if r.Method == http.MethodGet {
		collections, err := s.listShopCollections(r.Context(), true)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load collections")
			return
		}
		jsonOut(w, http.StatusOK, map[string]any{"collections": collections})
		return
	}
	var collection shopCollection
	if !decode(w, r, &collection) {
		return
	}
	collection.Slug = strings.ToLower(strings.TrimSpace(collection.Slug))
	if err := validateCollection(collection); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `INSERT INTO portal_shop_collections(realm_key,slug,name,description,image_url,active,featured,sort_order) VALUES(?,?,?,?,?,?,?,?)`, s.c.RealmKey, collection.Slug, collection.Name, collection.Description, collection.ImageURL, collection.Active, collection.Featured, collection.SortOrder)
	if err != nil {
		problem(w, http.StatusConflict, "Collection slug already exists")
		return
	}
	id, _ := result.LastInsertId()
	for order, productID := range collection.ProductIDs {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_collection_products(collection_id,product_id,sort_order) SELECT ?,id,? FROM portal_products WHERE id=? AND realm_key=?`, id, order, productID, s.c.RealmKey); err != nil {
			problem(w, http.StatusInternalServerError, "Could not attach collection products")
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'collection.create',?,?)`, actor.ID, strconv.FormatInt(id, 10), collection.Name); err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not create collection")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) adminShopCollection(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid collection")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for index := range s.mock.collections {
			if s.mock.collections[index].ID == id {
				if r.Method == http.MethodDelete {
					s.mock.collections[index].Active = false
				} else {
					var collection shopCollection
					if !decode(w, r, &collection) {
						return
					}
					collection.Slug = strings.ToLower(strings.TrimSpace(collection.Slug))
					if err = validateCollection(collection); err != nil {
						problem(w, 422, err.Error())
						return
					}
					collection.ID = id
					s.mock.collections[index] = collection
				}
				jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, http.StatusNotFound, "Collection not found")
		return
	}
	if r.Method == http.MethodDelete {
		result, execErr := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_shop_collections SET active=0 WHERE id=? AND realm_key=?`, id, s.c.RealmKey)
		if execErr != nil {
			problem(w, http.StatusInternalServerError, "Could not archive collection")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			problem(w, http.StatusNotFound, "Collection not found")
			return
		}
		_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'collection.archive',?)`, actor.ID, strconv.FormatUint(id, 10))
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var collection shopCollection
	if !decode(w, r, &collection) {
		return
	}
	collection.Slug = strings.ToLower(strings.TrimSpace(collection.Slug))
	if err = validateCollection(collection); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), `UPDATE portal_shop_collections SET slug=?,name=?,description=?,image_url=?,active=?,featured=?,sort_order=? WHERE id=? AND realm_key=?`, collection.Slug, collection.Name, collection.Description, collection.ImageURL, collection.Active, collection.Featured, collection.SortOrder, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusConflict, "Could not update collection")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Collection not found")
		return
	}
	_, _ = tx.ExecContext(r.Context(), `DELETE FROM portal_collection_products WHERE collection_id=?`, id)
	for order, productID := range collection.ProductIDs {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_collection_products(collection_id,product_id,sort_order) SELECT ?,id,? FROM portal_products WHERE id=? AND realm_key=?`, id, order, productID, s.c.RealmKey); err != nil {
			problem(w, http.StatusInternalServerError, "Could not update collection products")
			return
		}
	}
	_, _ = tx.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'collection.update',?,?)`, actor.ID, strconv.FormatUint(id, 10), collection.Name)
	if err = tx.Commit(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not update collection")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminStock(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if r.Method == http.MethodPost {
			var input struct {
				ProductID uint32 `json:"productId"`
				Delta     int32  `json:"delta"`
				Reason    string `json:"reason"`
			}
			if !decode(w, r, &input) {
				return
			}
			for index := range s.mock.products {
				product := &s.mock.products[index]
				if product.ID == input.ProductID {
					if product.StockLimit == 0 || int64(product.StockLimit)+int64(input.Delta) < int64(product.SoldCount) {
						problem(w, 409, "Adjustment would create invalid or unlimited stock")
						return
					}
					product.StockLimit = uint32(int64(product.StockLimit) + int64(input.Delta))
					movement := stockMovement{ID: uint64(len(s.mock.stockMovements) + 1), ProductID: input.ProductID, Delta: input.Delta, Type: "adjustment", Reason: input.Reason, ActorID: actor.ID, CreatedAt: time.Now()}
					s.mock.stockMovements = append([]stockMovement{movement}, s.mock.stockMovements...)
					jsonOut(w, 200, map[string]any{"stockLimit": product.StockLimit, "soldCount": product.SoldCount})
					return
				}
			}
			problem(w, 404, "Product not found")
			return
		}
		items := append([]stockMovement{}, s.mock.stockMovements...)
		productID, _ := strconv.ParseUint(r.URL.Query().Get("productId"), 10, 32)
		movementType := strings.TrimSpace(r.URL.Query().Get("type"))
		filtered := items[:0]
		for _, item := range items {
			if productID > 0 && item.ProductID != uint32(productID) || movementType != "" && item.Type != movementType {
				continue
			}
			filtered = append(filtered, item)
		}
		page, perPage, _ := requestPage(r, 25, 100)
		filtered, meta := slicePage(filtered, page, perPage)
		jsonOut(w, http.StatusOK, map[string]any{"movements": filtered, "pagination": meta})
		return
	}
	if r.Method == http.MethodGet {
		page, perPage, offset := requestPage(r, 25, 100)
		productID, _ := strconv.ParseUint(r.URL.Query().Get("productId"), 10, 32)
		movementType := strings.TrimSpace(r.URL.Query().Get("type"))
		if movementType != "" && !map[string]bool{"purchase": true, "refund": true, "adjustment": true, "reservation": true, "release": true}[movementType] {
			problem(w, http.StatusUnprocessableEntity, "Invalid stock movement type")
			return
		}
		where, args := "realm_key=?", []any{s.c.RealmKey}
		if productID > 0 {
			where += " AND product_id=?"
			args = append(args, productID)
		}
		if movementType != "" {
			where += " AND movement_type=?"
			args = append(args, movementType)
		}
		var total int
		if err := s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_stock_movements WHERE `+where, args...).Scan(&total); err != nil {
			problem(w, http.StatusInternalServerError, "Could not count stock history")
			return
		}
		meta := paginationMeta(page, perPage, total)
		offset = (meta.Page - 1) * perPage
		rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,product_id,quantity_delta,movement_type,reference_id,reason,actor_account_id,created_at FROM portal_stock_movements WHERE `+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, perPage, offset)...)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load stock history")
			return
		}
		defer rows.Close()
		items := []stockMovement{}
		for rows.Next() {
			var item stockMovement
			if rows.Scan(&item.ID, &item.ProductID, &item.Delta, &item.Type, &item.Reference, &item.Reason, &item.ActorID, &item.CreatedAt) == nil {
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"movements": items, "pagination": meta})
		return
	}
	var input struct {
		ProductID uint32 `json:"productId"`
		Delta     int32  `json:"delta"`
		Reason    string `json:"reason"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.ProductID == 0 || input.Delta == 0 || input.Delta < -1_000_000 || input.Delta > 1_000_000 || len(input.Reason) < 3 || len(input.Reason) > 500 {
		problem(w, http.StatusUnprocessableEntity, "Product, non-zero adjustment, and reason are required")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var limit, sold uint32
	if err = tx.QueryRowContext(r.Context(), `SELECT stock_limit,sold_count FROM portal_products WHERE id=? AND realm_key=? FOR UPDATE`, input.ProductID, s.c.RealmKey).Scan(&limit, &sold); err != nil {
		problem(w, http.StatusNotFound, "Product not found")
		return
	}
	if limit == 0 || int64(limit)+int64(input.Delta) < int64(sold) || int64(limit)+int64(input.Delta) > 1_000_000 {
		problem(w, http.StatusConflict, "Adjustment would create invalid or unlimited stock")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal_products SET stock_limit=stock_limit+? WHERE id=?`, input.Delta, input.ProductID); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_stock_movements(realm_key,product_id,quantity_delta,movement_type,reason,actor_account_id) VALUES(?,?,?,'adjustment',?,?)`, s.c.RealmKey, input.ProductID, input.Delta, input.Reason, actor.ID)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not adjust stock")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"stockLimit": int64(limit) + int64(input.Delta), "soldCount": sold})
}

func saveProductVariants(ctx context.Context, tx *sql.Tx, productID uint32, variants []productVariant) error {
	if _, err := tx.ExecContext(ctx, `DELETE vi FROM portal_product_variant_items vi JOIN portal_product_variants v ON v.id=vi.variant_id WHERE v.product_id=?`, productID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM portal_product_variants WHERE product_id=?`, productID); err != nil {
		return err
	}
	for _, variant := range variants {
		result, err := tx.ExecContext(ctx, `INSERT INTO portal_product_variants(product_id,name,sku,price_adjustment,active,sort_order) VALUES(?,?,?,?,?,?)`, productID, strings.TrimSpace(variant.Name), strings.TrimSpace(variant.SKU), variant.PriceAdjustment, variant.Active, variant.SortOrder)
		if err != nil {
			return err
		}
		variantID, _ := result.LastInsertId()
		for _, item := range variant.Items {
			if _, err = tx.ExecContext(ctx, `INSERT INTO portal_product_variant_items(variant_id,item_id,quantity) VALUES(?,?,?)`, variantID, item.ItemID, item.Quantity); err != nil {
				return err
			}
		}
	}
	return nil
}
