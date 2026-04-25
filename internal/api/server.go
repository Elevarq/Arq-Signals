package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/config"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/export"
	"github.com/elevarq/arq-signals/internal/safety"
)

// Deps holds handler dependencies.
type Deps struct {
	DB        *db.DB
	Collector *collector.Collector
	Exporter  *export.Builder
	Targets   []config.TargetConfig
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

func handleCollectNow(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.Collector.CollectNow()
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status": "collection triggered",
		})
	}
}

func handleExport(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := export.Options{
			Since: r.URL.Query().Get("since"),
			Until: r.URL.Query().Get("until"),
		}

		if tid := r.URL.Query().Get("target_id"); tid != "" {
			id, err := strconv.ParseInt(tid, 10, 64)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target_id"})
				return
			}
			opts.TargetID = id
		}

		// Buffer the ZIP fully before writing any response headers. If the
		// export fails midway we want to return a 500 with an error body, not
		// a 200 with a truncated/invalid ZIP that the client cannot
		// distinguish from success.
		var buf bytes.Buffer
		if err := deps.Exporter.WriteTo(&buf, opts); err != nil {
			slog.Error("export failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "export failed"})
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=arq-export-%s.zip",
			time.Now().UTC().Format("20060102-150405")))
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		if _, err := w.Write(buf.Bytes()); err != nil {
			slog.Error("export write failed", "err", err)
		}
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
