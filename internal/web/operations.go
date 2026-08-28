package web

import (
	"fmt"
	"net/http"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	jsonOut(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"ready": true, "mode": "demo"})
		return
	}
	checks := map[string]bool{"auth": s.s.Auth.PingContext(r.Context()) == nil, "characters": s.s.Characters.PingContext(r.Context()) == nil, "world": s.s.World.PingContext(r.Context()) == nil, "delivery": s.soap.Enabled()}
	ready := checks["auth"] && checks["characters"] && checks["world"]
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	jsonOut(w, status, map[string]any{"ready": ready, "checks": checks})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, "# TYPE azeroth_portal_http_requests_total counter\nazeroth_portal_http_requests_total %d\n# TYPE azeroth_portal_http_errors_total counter\nazeroth_portal_http_errors_total %d\n# TYPE azeroth_portal_orders_total counter\nazeroth_portal_orders_total %d\n", s.metrics.requests.Load(), s.metrics.errors.Load(), s.metrics.orders.Load())
}
