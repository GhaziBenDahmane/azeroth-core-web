package web

import (
	"fmt"
	"net/http"
)

type adminAnalyticsSnapshot struct {
	Accounts       uint64 `json:"accounts"`
	NewAccounts24h uint64 `json:"newAccounts24h"`
	Characters     uint64 `json:"characters"`
	OnlinePlayers  uint64 `json:"onlinePlayers"`
	OpenTickets    uint64 `json:"openTickets"`
	OrdersToday    uint64 `json:"ordersToday"`
	OrdersPending  uint64 `json:"ordersPending"`
	Credits30d     uint64 `json:"credits30d"`
}

func (s *Server) adminAnalytics(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "overview"); !ok {
		problem(w, http.StatusForbidden, "Administrator access required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, adminAnalyticsSnapshot{Accounts: 2841, NewAccounts24h: 37, Characters: 18472, OnlinePlayers: 846, OpenTickets: 3, OrdersToday: 28, OrdersPending: 2, Credits30d: 18450})
		return
	}
	var out adminAnalyticsSnapshot
	accountQuery := fmt.Sprintf("SELECT COUNT(*),COALESCE(SUM(joindate>=NOW()-INTERVAL 24 HOUR),0) FROM `%s`.account", s.c.AuthDB)
	characterQuery := fmt.Sprintf("SELECT COUNT(*),COALESCE(SUM(online),0) FROM `%s`.characters WHERE deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Auth.QueryRowContext(r.Context(), accountQuery).Scan(&out.Accounts, &out.NewAccounts24h); err != nil {
		problem(w, http.StatusServiceUnavailable, "Could not load dashboard analytics")
		return
	}
	if err := s.s.Characters.QueryRowContext(r.Context(), characterQuery).Scan(&out.Characters, &out.OnlinePlayers); err != nil {
		problem(w, http.StatusServiceUnavailable, "Could not load dashboard analytics")
		return
	}
	queries := []struct {
		query string
		dest  []any
	}{
		{"SELECT COUNT(*) FROM portal_support_tickets WHERE realm_key=? AND status='open'", []any{&out.OpenTickets}},
		{"SELECT COUNT(*) FROM portal_orders WHERE realm_key=? AND created_at>=CURRENT_DATE", []any{&out.OrdersToday}},
		{"SELECT COUNT(*) FROM portal_orders WHERE realm_key=? AND status IN ('pending','delivering','review')", []any{&out.OrdersPending}},
		{"SELECT COALESCE(SUM(total),0) FROM portal_orders WHERE realm_key=? AND created_at>=NOW()-INTERVAL 30 DAY AND status NOT IN ('failed','refunded')", []any{&out.Credits30d}},
	}
	for _, query := range queries {
		err := s.s.Auth.QueryRowContext(r.Context(), query.query, s.c.RealmKey).Scan(query.dest...)
		if err != nil {
			problem(w, http.StatusServiceUnavailable, "Could not load dashboard analytics")
			return
		}
	}
	jsonOut(w, http.StatusOK, out)
}
