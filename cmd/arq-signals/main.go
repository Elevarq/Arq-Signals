package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/elevarq/arq-signals/internal/api"
	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/config"
	"github.com/elevarq/arq-signals/internal/db"
	"github.com/elevarq/arq-signals/internal/export"
	"github.com/elevarq/arq-signals/internal/pgqueries"
	"github.com/elevarq/arq-signals/internal/safety"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "arq-signals: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	// Load configuration.
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Setup logging before validation so warnings reach the configured sink.
	safety.SetupLogging(cfg.Signals.LogLevel, cfg.Signals.LogJSON)

	// Strict configuration validation (R076). Hard errors abort startup;
	// warnings are logged and the daemon continues.
	warnings, err := config.ValidateStrict(cfg)
	for _, w := range warnings {
		slog.Warn("config warning", "msg", w)
	}
	if err != nil {
		safety.AuditLog("config_validated",
			"status", "error",
			"warnings", len(warnings),
			"hard_errors", 1,
		)
		return err
	}
	safety.AuditLog("config_validated",
		"status", "ok",
		"warnings", len(warnings),
		"hard_errors", 0,
	)

	// Enforce Postgres TLS policy.
	if err := config.ValidateProdTLS(cfg); err != nil {
		safety.AuditLog("config_validated",
			"status", "error",
			"phase", "tls_policy",
		)
		return fmt.Errorf("TLS policy: %w", err)
	}

	// Audit posture: high-sensitivity gate state and target enable/disable
	// counts. Per R078 these record *what* ran, not *which credentials* —
	// only counts and booleans, no host/user/password leakage.
	enabled, disabled := 0, 0
	for _, t := range cfg.Targets {
		if t.Enabled {
			enabled++
		} else {
			disabled++
		}
	}
	safety.AuditLog("high_sensitivity_collectors",
		"enabled", cfg.Signals.HighSensitivityCollectorsEnabled,
	)
	safety.AuditLog("targets_loaded",
		"enabled", enabled,
		"disabled", disabled,
	)

	slog.Info("arq-signals starting",
		"version", safety.Version,
		"commit", safety.Commit,
		"build_date", safety.BuildDate,
	)

	// Open database.
	store, err := db.Open(cfg.Database.Path, cfg.Database.WAL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer store.Close()

	// Run migrations.
	if err := store.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// Ensure instance ID.
	instanceID, err := store.EnsureInstanceID()
	if err != nil {
		return fmt.Errorf("instance id: %w", err)
	}
	slog.Info("instance", "id", instanceID)

	// Create context with signal handling.
	ctx, cancel := safety.SignalContext(context.Background())
	defer cancel()

	// Sync query catalog.
	syncQueryCatalog(store)

	// Initialize collector (no license gate, no stats engine).
	coll := collector.New(store, cfg.Targets, cfg.Signals.PollInterval, cfg.Signals.RetentionDays,
		collector.WithMaxConcurrentTargets(cfg.Signals.MaxConcurrentTargets),
		collector.WithTargetTimeout(cfg.Signals.TargetTimeout),
		collector.WithQueryTimeout(cfg.Signals.QueryTimeout),
		collector.WithAllowUnsafeRole(cfg.AllowUnsafeRole),
		collector.WithHighSensitivityCollectors(cfg.Signals.HighSensitivityCollectorsEnabled),
	)

	if cfg.AllowUnsafeRole {
		slog.Warn("UNSAFE MODE ENABLED: collection will proceed with unsafe role attributes — this is NOT recommended for production")
	}

	// Initialize exporter (no license gating).
	exporter := export.NewBuilder(store, instanceID)
	exporter.SetHighSensitivityCollectorsEnabled(cfg.Signals.HighSensitivityCollectorsEnabled)
	if cfg.AllowUnsafeRole {
		// Pass a function that returns the actual bypassed checks at export time,
		// so metadata reflects the specific role attributes that were bypassed.
		exporter.SetUnsafeMode(func() []string {
			checks := coll.GetBypassedChecks()
			if len(checks) == 0 {
				return []string{"ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true (no role checks bypassed yet)"}
			}
			return checks
		})
	}

	// Start collector in background.
	go coll.Run(ctx)

	// Generate API token if not set.
	if cfg.API.APIToken == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate API token: %w", err)
		}
		cfg.API.APIToken = hex.EncodeToString(b)
		fp := fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.API.APIToken)))[:12]
		slog.Info("signals API token generated (auto)", "fingerprint", fp)
	}

	// Start HTTP API server.
	deps := &api.Deps{
		DB:        store,
		Collector: coll,
		Exporter:  exporter,
		Targets:   cfg.Targets,
	}
	srv := api.NewServer(cfg.API.ListenAddr, cfg.API.ReadTimeout, cfg.API.WriteTimeout, cfg.API.APIToken, deps)

	// Run server in background.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	store.InsertEvent("signals_started", fmt.Sprintf("version=%s instance=%s", safety.Version, instanceID))

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		slog.Info("shutting down...")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	// Graceful shutdown.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("API server shutdown error", "err", err)
	}

	store.InsertEvent("signals_stopped", "graceful shutdown")
	slog.Info("arq-signals stopped")
	return nil
}

func syncQueryCatalog(store *db.DB) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, q := range pgqueries.All() {
		if err := store.UpsertQueryCatalog(db.QueryCatalogRow{
			QueryID:        q.ID,
			Category:       q.Category,
			ResultKind:     string(q.ResultKind),
			RetentionClass: string(q.RetentionClass),
			RegisteredAt:   now,
		}); err != nil {
			slog.Warn("failed to upsert query catalog", "query", q.ID, "err", err)
		}
	}
	slog.Info("query catalog synced", "count", len(pgqueries.All()))
}
