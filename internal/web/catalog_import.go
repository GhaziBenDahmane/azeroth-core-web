package web

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type catalogImportRow struct {
	Row     int      `json:"row"`
	Product product  `json:"product"`
	Errors  []string `json:"errors"`
}

func parseCatalogBool(value string, fallback bool) (bool, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "1", "true", "yes", "active":
		return true, nil
	case "0", "false", "no", "inactive":
		return false, nil
	}
	return false, fmt.Errorf("must be true or false")
}

func parseCatalogUint(record map[string]string, key string, bits int) (uint64, error) {
	value := strings.TrimSpace(record[key])
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, bits)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return parsed, nil
}

func parseCatalogItems(value string) ([]bundleItem, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	items := []bundleItem{}
	for _, part := range strings.Split(value, ";") {
		fields := strings.Split(strings.TrimSpace(part), ":")
		id, err := strconv.ParseUint(strings.TrimSpace(fields[0]), 10, 32)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("items must use itemId:quantity separated by semicolons")
		}
		quantity := uint64(1)
		if len(fields) > 1 {
			quantity, err = strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 32)
			if err != nil || quantity == 0 {
				return nil, fmt.Errorf("items must use itemId:quantity separated by semicolons")
			}
		}
		items = append(items, bundleItem{ItemID: uint32(id), Quantity: uint32(quantity)})
	}
	return items, nil
}

func parseCatalogCSV(raw string) ([]catalogImportRow, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(raw, "\ufeff")))
	reader.TrimLeadingSpace = true
	reader.ReuseRecord = false
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV header is required")
	}
	indexes := map[string]int{}
	for index, name := range header {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return nil, fmt.Errorf("CSV contains an empty header")
		}
		if _, exists := indexes[key]; exists {
			return nil, fmt.Errorf("duplicate CSV header %q", key)
		}
		indexes[key] = index
	}
	for _, required := range []string{"name", "price", "category"} {
		if _, ok := indexes[required]; !ok {
			return nil, fmt.Errorf("missing required %s column", required)
		}
	}
	rows := []catalogImportRow{}
	for line := 2; ; line++ {
		values, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("row %d: %w", line, readErr)
		}
		record := map[string]string{}
		for key, index := range indexes {
			if index < len(values) {
				record[key] = values[index]
			}
		}
		item := catalogImportRow{Row: line}
		p := &item.Product
		p.Name = strings.TrimSpace(record["name"])
		p.Description = strings.TrimSpace(record["description"])
		p.Category = strings.TrimSpace(record["category"])
		p.Tier = strings.TrimSpace(record["tier"])
		p.Tags = strings.TrimSpace(record["tags"])
		p.Visibility = strings.ToLower(strings.TrimSpace(record["visibility"]))
		if p.Visibility == "" {
			p.Visibility = "all"
		}
		p.ServiceAction = strings.ToLower(strings.TrimSpace(record["service_action"]))
		for _, field := range []struct {
			key    string
			target any
			bits   int
		}{{"item_id", &p.ItemID, 32}, {"quantity", &p.Quantity, 32}, {"price", &p.Price, 32}, {"class_id", &p.ClassID, 8}, {"service_level", &p.ServiceLevel, 8}, {"gold", &p.Gold, 32}, {"sale_price", &p.SalePrice, 32}, {"stock_limit", &p.StockLimit, 32}, {"per_account_limit", &p.PerAccountLimit, 32}} {
			value, parseErr := parseCatalogUint(record, field.key, field.bits)
			if parseErr != nil {
				item.Errors = append(item.Errors, parseErr.Error())
				continue
			}
			switch out := field.target.(type) {
			case *uint32:
				*out = uint32(value)
			case *uint8:
				*out = uint8(value)
			}
		}
		if value := strings.TrimSpace(record["category_order"]); value != "" {
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil {
				item.Errors = append(item.Errors, "category_order must be an integer")
			} else {
				p.CategoryOrder = parsed
			}
		}
		p.Active, err = parseCatalogBool(record["active"], true)
		if err != nil {
			item.Errors = append(item.Errors, "active "+err.Error())
		}
		p.Featured, err = parseCatalogBool(record["featured"], false)
		if err != nil {
			item.Errors = append(item.Errors, "featured "+err.Error())
		}
		p.Items, err = parseCatalogItems(record["items"])
		if err != nil {
			item.Errors = append(item.Errors, err.Error())
		}
		if validationErr := validateManagedProduct(*p); validationErr != nil {
			item.Errors = append(item.Errors, validationErr.Error())
		}
		rows = append(rows, item)
		if len(rows) > 1000 {
			return nil, fmt.Errorf("CSV supports at most 1,000 products")
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV has no product rows")
	}
	return rows, nil
}

func (s *Server) adminCatalogImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	var input struct {
		CSV    string `json:"csv"`
		Commit bool   `json:"commit"`
	}
	if !decode(w, r, &input) {
		return
	}
	if len(input.CSV) > 1024*1024 {
		problem(w, 413, "CSV is limited to 1 MB")
		return
	}
	rows, err := parseCatalogCSV(input.CSV)
	if err != nil {
		problem(w, 422, err.Error())
		return
	}
	valid := true
	for index := range rows {
		if len(rows[index].Errors) == 0 && !s.c.MockMode {
			if itemErr := s.validateProductItems(r.Context(), rows[index].Product); itemErr != nil {
				rows[index].Errors = append(rows[index].Errors, itemErr.Error())
			}
		}
		valid = valid && len(rows[index].Errors) == 0
	}
	if !input.Commit || !valid {
		jsonOut(w, 200, map[string]any{"valid": valid, "rows": rows, "count": len(rows)})
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		for index := range rows {
			rows[index].Product.ID = uint32(len(s.mock.products) + 1)
			s.mock.products = append(s.mock.products, rows[index].Product)
		}
		s.mock.mu.Unlock()
		jsonOut(w, 201, map[string]any{"imported": len(rows)})
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	for _, row := range rows {
		p := row.Product
		result, execErr := tx.ExecContext(r.Context(), `INSERT INTO portal_products(name,description,item_id,quantity,price,category,class_id,tier_label,service_level,gold_amount,service_action,active,per_account_limit,realm_key,featured,sale_price,stock_limit,category_order,tags,visibility_segment) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ClassID, p.Tier, p.ServiceLevel, p.Gold, p.ServiceAction, p.Active, p.PerAccountLimit, s.c.RealmKey, p.Featured, p.SalePrice, p.StockLimit, p.CategoryOrder, p.Tags, p.Visibility)
		if execErr != nil {
			problem(w, 500, "Could not import row "+strconv.Itoa(row.Row))
			return
		}
		id, _ := result.LastInsertId()
		for _, bundleItem := range p.Items {
			if _, execErr = tx.ExecContext(r.Context(), `INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)`, id, bundleItem.ItemID, bundleItem.Quantity); execErr != nil {
				problem(w, 500, "Could not import bundle items")
				return
			}
		}
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'catalog.import',?,?)`, actor.ID, s.c.RealmKey, fmt.Sprintf("Imported %d validated products", len(rows))); err != nil || tx.Commit() != nil {
		problem(w, 500, "Could not commit catalog import")
		return
	}
	jsonOut(w, http.StatusCreated, map[string]any{"imported": len(rows)})
}
