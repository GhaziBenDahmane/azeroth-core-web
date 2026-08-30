package web

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
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
	checks, _ := s.databaseChecks(r.Context())
	checks["delivery"] = s.soap.Enabled()
	ready := checks["auth"] && checks["characters"] && checks["world"]
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	jsonOut(w, status, map[string]any{"ready": ready, "checks": checks})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	queueDepth, oldestSeconds := int64(0), float64(0)
	if !s.c.MockMode && s.s != nil {
		var oldest sql.NullTime
		if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*),MIN(created_at) FROM portal_orders WHERE realm_key=? AND status IN ('pending','delivering','review')", s.c.RealmKey).Scan(&queueDepth, &oldest); err == nil && oldest.Valid {
			oldestSeconds = time.Since(oldest.Time).Seconds()
		}
	}
	_, _ = fmt.Fprintf(w, `# TYPE azeroth_portal_http_requests_total counter
azeroth_portal_http_requests_total %d
# TYPE azeroth_portal_http_errors_total counter
azeroth_portal_http_errors_total %d
# TYPE azeroth_portal_orders_total counter
azeroth_portal_orders_total %d
# TYPE azeroth_portal_login_failures_total counter
azeroth_portal_login_failures_total %d
# TYPE azeroth_portal_rate_limit_hits_total counter
azeroth_portal_rate_limit_hits_total %d
# TYPE azeroth_portal_webhook_failures_total counter
azeroth_portal_webhook_failures_total %d
# TYPE azeroth_portal_email_failures_total counter
azeroth_portal_email_failures_total %d
# TYPE azeroth_portal_soap_requests_total counter
azeroth_portal_soap_requests_total %d
# TYPE azeroth_portal_soap_faults_total counter
azeroth_portal_soap_faults_total %d
# TYPE azeroth_portal_soap_latency_seconds gauge
azeroth_portal_soap_latency_seconds %.6f
# TYPE azeroth_portal_delivery_outcomes_total counter
azeroth_portal_delivery_outcomes_total{outcome="delivered"} %d
azeroth_portal_delivery_outcomes_total{outcome="review"} %d
# TYPE azeroth_portal_delivery_queue_depth gauge
azeroth_portal_delivery_queue_depth %d
# TYPE azeroth_portal_delivery_queue_oldest_seconds gauge
azeroth_portal_delivery_queue_oldest_seconds %.3f
# TYPE azeroth_portal_database_reachable gauge
azeroth_portal_database_reachable{database="auth"} %s
azeroth_portal_database_reachable{database="characters"} %s
azeroth_portal_database_reachable{database="world"} %s
# TYPE azeroth_portal_database_latency_seconds gauge
azeroth_portal_database_latency_seconds{database="auth"} %.6f
azeroth_portal_database_latency_seconds{database="characters"} %.6f
azeroth_portal_database_latency_seconds{database="world"} %.6f
`, s.metrics.requests.Load(), s.metrics.errors.Load(), s.metrics.orders.Load(), s.metrics.loginFailures.Load(), s.metrics.rateLimitHits.Load(), s.metrics.webhookFailures.Load(), s.metrics.emailFailures.Load(), s.metrics.soapRequests.Load(), s.metrics.soapFaults.Load(), float64(s.metrics.soapLatencyMicros.Load())/1e6, s.metrics.deliverySuccess.Load(), s.metrics.deliveryReview.Load(), queueDepth, oldestSeconds, boolMetric(s.metrics.authDBReachable.Load()), boolMetric(s.metrics.charactersDBReachable.Load()), boolMetric(s.metrics.worldDBReachable.Load()), float64(s.metrics.authDBLatencyMicros.Load())/1e6, float64(s.metrics.charactersDBLatencyMicros.Load())/1e6, float64(s.metrics.worldDBLatencyMicros.Load())/1e6)
}

func boolMetric(value bool) string {
	return strconv.Itoa(map[bool]int{false: 0, true: 1}[value])
}

func (s *Server) databaseChecks(ctx context.Context) (map[string]bool, map[string]time.Duration) {
	checks := make(map[string]bool, 3)
	latencies := make(map[string]time.Duration, 3)
	for _, target := range []struct {
		name      string
		db        *sql.DB
		reachable *atomic.Bool
		latency   *atomic.Int64
	}{
		{"auth", s.s.Auth, &s.metrics.authDBReachable, &s.metrics.authDBLatencyMicros},
		{"characters", s.s.Characters, &s.metrics.charactersDBReachable, &s.metrics.charactersDBLatencyMicros},
		{"world", s.s.World, &s.metrics.worldDBReachable, &s.metrics.worldDBLatencyMicros},
	} {
		start := time.Now()
		err := target.db.PingContext(ctx)
		latency := time.Since(start)
		checks[target.name], latencies[target.name] = err == nil, latency
		target.reachable.Store(err == nil)
		target.latency.Store(latency.Microseconds())
		if err == nil {
			s.metrics.dbLastSuccessUnix.Store(time.Now().Unix())
		}
	}
	return checks, latencies
}
