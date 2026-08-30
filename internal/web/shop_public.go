package web

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) shopProductDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid product")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for _, candidate := range s.mock.products {
			if candidate.ID == uint32(id) && candidate.Active {
				jsonOut(w, http.StatusOK, map[string]any{"product": candidate})
				return
			}
		}
		problem(w, http.StatusNotFound, "Product not found")
		return
	}
	p, err := s.loadManagedProduct(r.Context(), uint32(id))
	now := time.Now()
	if err == sql.ErrNoRows || err == nil && (!p.Active || p.StartsAt != nil && p.StartsAt.After(now) || p.EndsAt != nil && !p.EndsAt.After(now)) {
		problem(w, http.StatusNotFound, "Product not found")
		return
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load product")
		return
	}
	enrichProductSummary(&p)
	jsonOut(w, http.StatusOK, map[string]any{"product": p})
}

func enrichProductSummary(p *product) {
	if len(p.Includes) == 0 {
		for _, item := range p.Items {
			name := item.Name
			if name == "" {
				name = "Item " + strconv.FormatUint(uint64(item.ItemID), 10)
			}
			p.Includes = append(p.Includes, strconv.FormatUint(uint64(item.Quantity), 10)+" × "+name)
		}
	}
	if p.ServiceLevel == 80 {
		p.Includes = append(p.Includes, "Level 80", "All class trainer spell ranks", "All class weapon proficiencies at 400", "Artisan Riding and Cold Weather Flying")
	}
	if p.Gold > 0 {
		p.Includes = append(p.Includes, commaNumber(p.Gold)+" gold")
	}
}

type productEligibility struct {
	Character character `json:"character"`
	Eligible  bool      `json:"eligible"`
	Reasons   []string  `json:"reasons"`
}

func (s *Server) shopProductEligibility(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		if _, ok := s.mockUser(r); !ok {
			problem(w, http.StatusUnauthorized, "Sign in required")
			return
		}
		id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
		if err != nil || id == 0 {
			problem(w, http.StatusBadRequest, "Invalid product")
			return
		}
		s.mockProductEligibility(w, uint32(id))
		return
	}
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid product")
		return
	}
	p, err := s.loadManagedProduct(r.Context(), uint32(id))
	if err != nil || !p.Active {
		problem(w, http.StatusNotFound, "Product not found")
		return
	}
	var balance, purchaseCount uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance)
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_orders WHERE account_id=? AND product_id=? AND realm_key=? AND status NOT IN ('failed','refunded')", a.ID, p.ID, s.c.RealmKey).Scan(&purchaseCount)
	characters, err := s.characterRows(r.Context(), a.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load characters")
		return
	}
	price := effectiveProductPrice(p)
	rows := make([]productEligibility, 0, len(characters))
	for _, c := range characters {
		reasons := eligibilityReasons(p, c, balance, purchaseCount, s.soap.Enabled())
		rows = append(rows, productEligibility{Character: c, Eligible: len(reasons) == 0, Reasons: reasons})
	}
	jsonOut(w, http.StatusOK, map[string]any{"productId": p.ID, "price": price, "balance": balance, "characters": rows})
}

func effectiveProductPrice(p product) uint32 {
	if p.SalePrice > 0 && p.SalePrice < p.Price {
		return p.SalePrice
	}
	return p.Price
}

func eligibilityReasons(p product, c character, balance, purchaseCount uint32, deliveryEnabled bool) []string {
	reasons := []string{}
	if c.Online {
		reasons = append(reasons, "Character must be offline")
	}
	if p.ClassID != 0 && c.Class != p.ClassID {
		reasons = append(reasons, "Class does not match this package")
	}
	if p.StockLimit > 0 && p.SoldCount >= p.StockLimit {
		reasons = append(reasons, "Product is sold out")
	}
	if p.PerAccountLimit > 0 && purchaseCount >= p.PerAccountLimit {
		reasons = append(reasons, "Account purchase limit reached")
	}
	if balance < effectiveProductPrice(p) {
		reasons = append(reasons, "Not enough credits")
	}
	if !deliveryEnabled {
		reasons = append(reasons, "Delivery is not configured")
	}
	return reasons
}

func (s *Server) mockProductEligibility(w http.ResponseWriter, id uint32) {
	s.mock.mu.Lock()
	defer s.mock.mu.Unlock()
	var p *product
	for i := range s.mock.products {
		if s.mock.products[i].ID == id && s.mock.products[i].Active {
			p = &s.mock.products[i]
			break
		}
	}
	if p == nil {
		problem(w, http.StatusNotFound, "Product not found")
		return
	}
	rows := make([]productEligibility, 0, len(mockCharacters))
	for _, c := range mockCharacters {
		reasons := eligibilityReasons(*p, c, s.mock.balance, s.mock.purchases[p.ID], true)
		rows = append(rows, productEligibility{Character: c, Eligible: len(reasons) == 0, Reasons: reasons})
	}
	jsonOut(w, http.StatusOK, map[string]any{"productId": p.ID, "price": effectiveProductPrice(*p), "balance": s.mock.balance, "characters": rows})
}
