package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/config"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/export"
	"github.com/elevarq/arq-signals/internal/metrics"
	"github.com/elevarq/arq-signals/internal/safety"
)

// Deps holds handler dependencies.
type Deps struct {
	DB        *db.DB
	Collector *collector.Collector
	Exporter  *export.Builder
	Targets   []config.TargetConfig
	// Metrics is the optional Prometheus registry. When nil the
	// /metrics endpoint is not registered. Pass non-nil only when
	// signals.metrics_enabled is true.
	Metrics *metrics.Registry
	// MetricsPath is the URL path the /metrics endpoint is mounted on.
	// Ignored when Metrics is nil.
	MetricsPath string
}

// Server is the Arq Signals HTTP API server.
type Server struct {
	httpServer *http.Server
	deps       *Deps
}

// NewServer creates a new API Server with signals-only endpoints.
func NewServer(addr string, readTimeout, writeTimeout time.Duration, apiToken string, deps *Deps) *Server {
	mux := http.NewServeMux()

	// Register signals-only routes.
	mux.HandleFunc("GET /health", handleHealth(deps))
	mux.HandleFunc("GET /status", handleStatus(deps))
	mux.HandleFunc("POST /collect/now", handleCollectNow(deps))
	mux.HandleFunc("GET /export", handleExport(deps))

	// Optional Prometheus /metrics endpoint (R079). Off unless an
	// explicit Metrics registry is supplied. Inherits the same bearer
	// token auth as the rest of the API; operators that want
	// unauthenticated scraping should bind locally and use
	// network-level controls.
	if deps.Metrics != nil && deps.MetricsPath != "" {
		mux.Handle("GET "+deps.MetricsPath, promhttp.HandlerFor(
			deps.Metrics.Gatherer(),
			promhttp.HandlerOpts{
				ErrorLog:      slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
				ErrorHandling: promhttp.ContinueOnError,
			},
		))
	}

	// Wrap with middleware: recovery -> logging -> token auth.
	tokenLimiter := newTokenRateLimiter()
	handler := recoveryMiddleware(loggingMiddleware(tokenAuthMiddleware(apiToken, tokenLimiter)(mux)))

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		deps: deps,
	}
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// Start begins listening and serving. It blocks until the server stops.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return err
	}
	slog.Info("API server listening", "addr", s.httpServer.Addr)
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("API server shutting down")
	return s.httpServer.Shutdown(ctx)
}

// --- Handlers ---

func handleHealth(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": safety.Version,
		})
	}
}

func handleStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapCount, _ := deps.DB.CountSnapshots()
		targets, _ := deps.DB.GetTargets()
		instanceID, _ := deps.DB.GetMeta("instance_id")

		var targetInfo []map[string]any
		for _, t := range targets {
			tInfo := map[string]any{
				"id":      t.ID,
				"name":    t.Name,
				"host":    t.Host,
				"port":    t.Port,
				"dbname":  t.DBName,
				"user":    t.Username,
				"sslmode": t.SSLMode,
				"enabled": t.Enabled,
			}
			// secret_type and secret_ref are intentionally omitted from
			// /status to avoid revealing credential source details.

			if lc := deps.DB.GetTargetLastCollected(t.ID); lc != "" {
				tInfo["last_collected"] = lc
			}

			targetInfo = append(targetInfo, tInfo)
		}

		queryCatalog, _ := deps.DB.GetQueryCatalog()

		resp := map[string]any{
			"instance_id":         instanceID,
			"version":             safety.Version,
			"target_count":        len(targets),
			"targets":             targetInfo,
			"snapshot_count":      snapCount,
			"query_catalog_count": len(queryCatalog),
			"last_collected":      deps.Collector.LastCollected(),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// handleCollectNow accepts an optional JSON body (R082 Phase 1) that
// narrows the cycle to a subset of configured targets. The historical
// behaviour — empty body, no body, or `Content-Length: 0` — keeps
// collecting every enabled target and is byte-for-byte unchanged.
//
// New behaviour when the body is present and parses to JSON:
//
//	{"targets": ["a", "b"]}
//
// Validation rules (R082):
//   - `targets` field absent → collect all enabled targets.
//   - `targets` empty array → 400 Bad Request (client bug; never
//     silently treated as "collect nothing" or "collect all").
//   - Any name not in `signals.yaml` → 400 with reason
//     `unknown_target`.
//   - Any name on a target with `enabled: false` → 400 with reason
//     `disabled_target`.
//   - Duplicate names are deduplicated silently.
//
// On success the 202 response carries `accepted_targets` so the
// caller can confirm which targets were scheduled. The 400 response
// carries `accepted_targets` (the names that *would* have been
// collected) plus `rejected_targets` with per-name reasons. The
// cycle is not triggered when any target was rejected.
func handleCollectNow(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		body = bytes.TrimSpace(body)

		var targetFilter []string
		var allEnabled []string
		for _, t := range deps.Targets {
			if t.Enabled {
				allEnabled = append(allEnabled, t.Name)
			}
		}

		if len(body) > 0 {
			var req struct {
				Targets *[]string `json:"targets,omitempty"`
			}
			if err := json.Unmarshal(body, &req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]any{
					"error": "invalid JSON body",
				})
				return
			}

			if req.Targets != nil {
				if len(*req.Targets) == 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error": "targets must be a non-empty array; omit the field to collect all enabled targets",
					})
					return
				}

				configured := make(map[string]config.TargetConfig, len(deps.Targets))
				for _, t := range deps.Targets {
					configured[t.Name] = t
				}

				type rejection struct {
					Name   string `json:"name"`
					Reason string `json:"reason"`
				}
				seen := make(map[string]bool, len(*req.Targets))
				accepted := make([]string, 0, len(*req.Targets))
				var rejected []rejection

				for _, name := range *req.Targets {
					if seen[name] {
						continue
					}
					seen[name] = true

					cfg, ok := configured[name]
					if !ok {
						rejected = append(rejected, rejection{Name: name, Reason: "unknown_target"})
						continue
					}
					if !cfg.Enabled {
						rejected = append(rejected, rejection{Name: name, Reason: "disabled_target"})
						continue
					}
					accepted = append(accepted, name)
				}

				if len(rejected) > 0 {
					writeJSON(w, http.StatusBadRequest, map[string]any{
						"error":            "one or more targets cannot be collected",
						"accepted_targets": accepted,
						"rejected_targets": rejected,
					})
					return
				}

				targetFilter = accepted
			}
		}

		responseTargets := targetFilter
		if responseTargets == nil {
			responseTargets = allEnabled
		}

		deps.Collector.CollectNow(targetFilter)
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":           "collection triggered",
			"accepted_targets": responseTargets,
		})
	}
}

func handleExport(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		opts := export.Options{
			Since: r.URL.Query().Get("since"),
			Until: r.URL.Query().Get("until"),
		}

		if tid := r.URL.Query().Get("target_id"); tid != "" {
			id, err := strconv.ParseInt(tid, 10, 64)
			if err != nil {
				safety.AuditLog("export_requested",
					"source_ip", remoteIP(r),
					"target_id", tid,
					"since", opts.Since,
					"until", opts.Until,
				)
				safety.AuditLog("export_completed",
					"status", "failed",
					"duration_ms", time.Since(start).Milliseconds(),
					"size_bytes", 0,
					"error_category", "invalid_target_id",
				)
				deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
				deps.Metrics.RecordExportFailure("invalid_target_id")
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target_id"})
				return
			}
			opts.TargetID = id
		}

		safety.AuditLog("export_requested",
			"source_ip", remoteIP(r),
			"target_id", opts.TargetID,
			"since", opts.Since,
			"until", opts.Until,
		)

		// Buffer the ZIP fully before writing any response headers. If the
		// export fails midway we want to return a 500 with an error body, not
		// a 200 with a truncated/invalid ZIP that the client cannot
		// distinguish from success.
		var buf bytes.Buffer
		if err := deps.Exporter.WriteTo(&buf, opts); err != nil {
			slog.Error("export failed", "err", err)
			safety.AuditLog("export_completed",
				"status", "failed",
				"duration_ms", time.Since(start).Milliseconds(),
				"size_bytes", 0,
				"error_category", "builder_error",
			)
			deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
			deps.Metrics.RecordExportFailure("builder_error")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export failed"})
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=arq-export-%s.zip",
			time.Now().UTC().Format("20060102-150405")))
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		if _, err := w.Write(buf.Bytes()); err != nil {
			slog.Error("export write failed", "err", err)
			safety.AuditLog("export_completed",
				"status", "failed",
				"duration_ms", time.Since(start).Milliseconds(),
				"size_bytes", buf.Len(),
				"error_category", "write_error",
			)
			deps.Metrics.RecordExport("failed", time.Since(start).Seconds())
			deps.Metrics.RecordExportFailure("write_error")
			return
		}
		safety.AuditLog("export_completed",
			"status", "success",
			"duration_ms", time.Since(start).Milliseconds(),
			"size_bytes", buf.Len(),
		)
		deps.Metrics.RecordExport("success", time.Since(start).Seconds())
	}
}

// --- Middleware ---

// tokenRateLimiter tracks invalid bearer token attempts per IP.
type tokenRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*tokenAttempt
}

type tokenAttempt struct {
	failures    int
	lastAttempt time.Time
}

func newTokenRateLimiter() *tokenRateLimiter {
	return &tokenRateLimiter{attempts: make(map[string]*tokenAttempt)}
}

const (
	tokenMaxFailures   = 5
	tokenLockoutWindow = 5 * time.Minute
)

func (l *tokenRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[ip]
	if !ok {
		return true
	}
	if time.Since(a.lastAttempt) > tokenLockoutWindow {
		delete(l.attempts, ip)
		return true
	}
	return a.failures < tokenMaxFailures
}

func (l *tokenRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	a, ok := l.attempts[ip]
	if !ok {
		a = &tokenAttempt{}
		l.attempts[ip] = a
	}
	a.failures++
	a.lastAttempt = now

	// Opportunistic prune: drop entries older than the lockout window.
	// Bounds map growth in long-lived processes where attackers come
	// from many short-lived IPs that never retry.
	cutoff := now.Add(-tokenLockoutWindow)
	for otherIP, other := range l.attempts {
		if other.lastAttempt.Before(cutoff) {
			delete(l.attempts, otherIP)
		}
	}
}

// recordSuccess clears any prior failures for the IP. Called when a
// request authenticates successfully so a near-miss IP does not stay
// near the lockout threshold indefinitely.
func (l *tokenRateLimiter) recordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// recoveryMiddleware catches panics and returns 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "panic", rec, "path", r.URL.Path)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs each request.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID.
		b := make([]byte, 8)
		rand.Read(b)
		reqID := hex.EncodeToString(b)
		w.Header().Set("X-Request-ID", reqID)

		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		slog.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration", time.Since(start).String(),
			"request_id", reqID,
		)
	})
}

// tokenAuthMiddleware checks for a valid Bearer token on all requests except /health.
// It also rate-limits invalid token attempts per remote IP.
func tokenAuthMiddleware(token string, limiter *tokenRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health endpoint (loopback health checks).
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			ip := remoteIP(r)

			if !limiter.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "too many invalid token attempts"})
				return
			}

			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				limiter.recordFailure(ip)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid Authorization header"})
				return
			}

			provided := auth[len(prefix):]
			if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
				limiter.recordFailure(ip)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid API token"})
				return
			}

			// Successful authentication clears any prior failure count for this IP.
			limiter.recordSuccess(ip)
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
