package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests handled by path and status code.",
	}, []string{"path", "status"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds by path and status code.",
		Buckets: prometheus.DefBuckets,
	}, []string{"path", "status"})

	cacheHitRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cache_hit_ratio",
		Help: "In-memory cache hit ratio by cache name since process start.",
	}, []string{"name"})

	cacheMetricState sync.Map
)

func init() {
	for _, name := range []string{
		"admin_ai_settings",
		"admin_dialog_summary",
		"admin_mode_model_stats",
		"admin_orchestration",
		"admin_summary_prompts",
		"admin_tariff_groups",
		"modes",
		"public_demo_modes",
		"simple_global",
	} {
		cacheHitRatio.WithLabelValues(name).Set(0)
	}
}

type cacheMetricCounters struct {
	mu       sync.Mutex
	hits     uint64
	requests uint64
}

func recordCacheLookup(name string, hit bool) {
	if strings.TrimSpace(name) == "" {
		name = "unknown"
	}
	value, _ := cacheMetricState.LoadOrStore(name, &cacheMetricCounters{})
	c := value.(*cacheMetricCounters)
	c.mu.Lock()
	c.requests++
	if hit {
		c.hits++
	}
	ratio := float64(c.hits) / float64(c.requests)
	c.mu.Unlock()
	cacheHitRatio.WithLabelValues(name).Set(ratio)
}

func RegisterDBPoolMetrics(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "db_pool_acquire_wait_seconds",
		Help: "Cumulative time spent waiting for PostgreSQL pool connection acquisition in seconds.",
	}, func() float64 {
		return pool.Stat().AcquireDuration().Seconds()
	}))
	prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "pgxpool_active_connections",
		Help: "Currently acquired PostgreSQL connections in the pgx pool.",
	}, func() float64 {
		return float64(pool.Stat().AcquiredConns())
	}))
}

func withPrometheusMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		path := r.Pattern
		if path == "" {
			path = normalizeMetricsPath(r.URL.Path)
		}
		status := strconv.Itoa(rw.status)
		httpRequestsTotal.WithLabelValues(path, status).Inc()
		httpRequestDuration.WithLabelValues(path, status).Observe(time.Since(start).Seconds())
	})
}

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func normalizeMetricsPath(path string) string {
	switch {
	case path == "":
		return "/"
	case strings.HasPrefix(path, "/api/auth/oauth/"):
		return "/api/auth/oauth/"
	case strings.HasPrefix(path, "/api/admin/users/"):
		return "/api/admin/users/"
	case strings.HasPrefix(path, "/api/admin/modes/"):
		return "/api/admin/modes/"
	case strings.HasPrefix(path, "/api/admin/tariffs/"):
		return "/api/admin/tariffs/"
	case strings.HasPrefix(path, "/api/admin/tariff-groups/"):
		return "/api/admin/tariff-groups/"
	case strings.HasPrefix(path, "/api/admin/promocodes/"):
		return "/api/admin/promocodes/"
	case strings.HasPrefix(path, "/api/admin/summary-prompts/"):
		return "/api/admin/summary-prompts/"
	case strings.HasPrefix(path, "/api/admin/dialogs/"):
		return "/api/admin/dialogs/"
	case strings.HasPrefix(path, "/api/tester/users/"):
		return "/api/tester/users/"
	case strings.HasPrefix(path, "/webhooks/"):
		return "/webhooks/*"
	default:
		return path
	}
}

func (h Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if !metricsBasicAuthOK(r, h.MetricsBasicAuth) {
		w.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	promhttp.Handler().ServeHTTP(w, r)
}

func metricsBasicAuthOK(r *http.Request, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false
	}
	wantUser, wantPass, ok := strings.Cut(expected, ":")
	if !ok || wantUser == "" || wantPass == "" {
		return false
	}
	gotUser, gotPass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(gotUser), []byte(wantUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(gotPass), []byte(wantPass)) == 1
	return userOK && passOK
}
