package web

import (
	"net/http"
	"strconv"
)

func (s *Server) wishlist(w http.ResponseWriter, r *http.Request) {
	account, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		ids := []uint32{}
		for id, saved := range s.mock.wishlist {
			if saved {
				ids = append(ids, id)
			}
		}
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]any{"productIds": ids})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT product_id FROM portal_wishlist WHERE account_id=? AND realm_key=? ORDER BY created_at DESC`, account.ID, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load wishlist")
		return
	}
	defer rows.Close()
	ids := []uint32{}
	for rows.Next() {
		var id uint32
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"productIds": ids})
}

func (s *Server) wishlistItem(w http.ResponseWriter, r *http.Request) {
	account, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid product")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		if s.mock.wishlist == nil {
			s.mock.wishlist = map[uint32]bool{}
		}
		if r.Method == http.MethodDelete {
			delete(s.mock.wishlist, uint32(id))
		} else {
			s.mock.wishlist[uint32(id)] = true
		}
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusOK, map[string]bool{"saved": r.Method != http.MethodDelete})
		return
	}
	if r.Method == http.MethodDelete {
		_, err = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_wishlist WHERE account_id=? AND realm_key=? AND product_id=?`, account.ID, s.c.RealmKey, id)
	} else {
		var exists bool
		if err = s.s.Auth.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM portal_products WHERE id=? AND realm_key=? AND active=1)`, id, s.c.RealmKey).Scan(&exists); err == nil && !exists {
			problem(w, http.StatusNotFound, "Product not found")
			return
		}
		if err == nil {
			_, err = s.s.Auth.ExecContext(r.Context(), `INSERT IGNORE INTO portal_wishlist(account_id,realm_key,product_id) VALUES(?,?,?)`, account.ID, s.c.RealmKey, id)
		}
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update wishlist")
		return
	}
	jsonOut(w, http.StatusOK, map[string]bool{"saved": r.Method != http.MethodDelete})
}
