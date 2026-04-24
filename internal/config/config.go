package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for Arq Signals.
type Config struct {
	Env                string         `yaml:"env"` // "dev" (default), "lab", "prod"
	AllowInsecurePgTLS bool           `yaml:"-"`   // env-only via ARQ_ALLOW_INSECURE_PG_TLS
	AllowUnsafeRole    bool           `yaml:"-"`   // env-only via ARQ_SIGNALS_ALLOW_UNSAFE_ROLE
	Signals            SignalsConfig  `yaml:"signals"`
	Targets            []TargetConfig `yaml:"targets"`
	API                APIConfig      `yaml:"api"`
	Database           DatabaseConfig `yaml:"database"`
}

type SignalsConfig struct {
	PollInterval         time.Duration `yaml:"-"`
	PollIntervalS        string        `yaml:"poll_interval"` // e.g. "5m"
	RetentionDays        int           `yaml:"retention_days"`
	LogLevel             string        `yaml:"log_level"`
	LogJSON              bool          `yaml:"log_json"`
	MaxConcurrentTargets int           `yaml:"max_concurrent_targets"`
	TargetTimeout        time.Duration `yaml:"-"`
	TargetTimeoutS       string        `yaml:"target_timeout"`
	QueryTimeout         time.Duration `yaml:"-"`
	QueryTimeoutS        string        `yaml:"query_timeout"`
}

type TargetConfig struct {
	Name            string `yaml:"name"`
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	DBName          string `yaml:"dbname"`
	User            string `yaml:"user"`
	SSLMode         string `yaml:"sslmode"`
	SSLRootCertFile string `yaml:"sslrootcert_file"`
	PasswordFile    string `yaml:"password_file"`
	PasswordEnv     string `yaml:"password_env"`
	PgpassFile      string `yaml:"pgpass_file"`
	Enabled         bool   `yaml:"enabled"`
}

// SecretType returns the credential source type for display/storage.
func (t TargetConfig) SecretType() string {
	switch {
	case t.PasswordFile != "":
		return "FILE"
	case t.PasswordEnv != "":
		return "ENV"
	case t.PgpassFile != "":
		return "PGPASS"
	default:
		return "NONE"
	}
}

// SecretRef returns the non-secret reference for the credential source.
func (t TargetConfig) SecretRef() string {
	switch {
	case t.PasswordFile != "":
		return t.PasswordFile
	case t.PasswordEnv != "":
		return t.PasswordEnv
	case t.PgpassFile != "":
		return t.PgpassFile
	default:
		return ""
	}
}

// ConnIdentity returns a stable string identifying the target's connection
// (host:port/dbname@user) for hashing purposes, without any secrets.
func (t TargetConfig) ConnIdentity() string {
	port := t.Port
	if port == 0 {
		port = 5432
	}
	return fmt.Sprintf("%s:%d/%s@%s", t.Host, port, t.DBName, t.User)
}

type APIConfig struct {
	ListenAddr    string        `yaml:"listen_addr"`
	ReadTimeout   time.Duration `yaml:"-"`
	ReadTimeoutS  string        `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"-"`
	WriteTimeoutS string        `yaml:"write_timeout"`
	APIToken      string        `yaml:"-"` // env-only via ARQ_SIGNALS_API_TOKEN
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
	WAL  bool   `yaml:"wal"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Env: "dev",
		Signals: SignalsConfig{
			PollInterval:         5 * time.Minute,
			PollIntervalS:        "5m",
			RetentionDays:        30,
			LogLevel:             "info",
			LogJSON:              false,
			MaxConcurrentTargets: 4,
			TargetTimeout:        60 * time.Second,
			TargetTimeoutS:       "60s",
			QueryTimeout:         10 * time.Second,
			QueryTimeoutS:        "10s",
		},
		API: APIConfig{
			ListenAddr:    "127.0.0.1:8081",
			ReadTimeout:   30 * time.Second,
			ReadTimeoutS:  "30s",
			WriteTimeout:  180 * time.Second,
			WriteTimeoutS: "180s",
		},
		Database: DatabaseConfig{
			Path: "/data/arq-signals.db",
			WAL:  true,
		},
	}
}

// Load reads configuration from the given file path, then applies env overrides.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		// Try default locations.
		for _, p := range []string{"/etc/arq/signals.yaml", "./signals.yaml"} {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if err := applyEnvOverrides(&cfg); err != nil {
		return cfg, err
	}

	if err := parseDurations(&cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) error {
	if v := os.Getenv("ARQ_ENV"); v != "" {
		cfg.Env = strings.ToLower(v)
	}
	if v := os.Getenv("ARQ_ALLOW_INSECURE_PG_TLS"); v != "" {
		cfg.AllowInsecurePgTLS = v == "true" || v == "1"
	}
	if v := os.Getenv("ARQ_SIGNALS_ALLOW_UNSAFE_ROLE"); v != "" {
		cfg.AllowUnsafeRole = v == "true" || v == "1"
	}
	if v := os.Getenv("ARQ_SIGNALS_POLL_INTERVAL"); v != "" {
		cfg.Signals.PollIntervalS = v
	}
	if v := os.Getenv("ARQ_SIGNALS_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Signals.RetentionDays = n
		}
	}
	if v := os.Getenv("ARQ_SIGNALS_LOG_LEVEL"); v != "" {
		cfg.Signals.LogLevel = strings.ToLower(v)
	}
	if v := os.Getenv("ARQ_SIGNALS_LOG_JSON"); v != "" {
		cfg.Signals.LogJSON = v == "true" || v == "1"
	}
	if v := os.Getenv("ARQ_SIGNALS_MAX_CONCURRENT_TARGETS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Signals.MaxConcurrentTargets = n
		}
	}
	if v := os.Getenv("ARQ_SIGNALS_TARGET_TIMEOUT"); v != "" {
		cfg.Signals.TargetTimeoutS = v
	}
	if v := os.Getenv("ARQ_SIGNALS_QUERY_TIMEOUT"); v != "" {
		cfg.Signals.QueryTimeoutS = v
	}
	if v := os.Getenv("ARQ_SIGNALS_LISTEN_ADDR"); v != "" {
		cfg.API.ListenAddr = v
	}
	if v := os.Getenv("ARQ_SIGNALS_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("ARQ_SIGNALS_WRITE_TIMEOUT"); v != "" {
		cfg.API.WriteTimeoutS = v
	}
	if v := os.Getenv("ARQ_SIGNALS_API_TOKEN"); v != "" {
		cfg.API.APIToken = v
	}
	// File takes precedence over the raw env var when both are set —
	// matches the _FILE convention used by the official postgres image.
	// A missing or unreadable file is a hard error so a deployment
	// mistake does not silently fall through to the weaker env-based
	// value.
	if v := os.Getenv("ARQ_SIGNALS_API_TOKEN_FILE"); v != "" {
		data, err := os.ReadFile(v)
		if err != nil {
			return fmt.Errorf("read ARQ_SIGNALS_API_TOKEN_FILE %s: %w", v, err)
		}
		cfg.API.APIToken = strings.TrimRight(string(data), "\n\r")
	}

	// Allow a single target via env (common for containers).
	if host := os.Getenv("ARQ_SIGNALS_TARGET_HOST"); host != "" {
		name := os.Getenv("ARQ_SIGNALS_TARGET_NAME")
		if name == "" {
			name = "default"
		}
		port := 5432
		if v := os.Getenv("ARQ_SIGNALS_TARGET_PORT"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				port = n
			}
		}
		dbname := os.Getenv("ARQ_SIGNALS_TARGET_DBNAME")
		if dbname == "" {
			dbname = "postgres"
		}
		tgt := TargetConfig{
			Name:            name,
			Host:            host,
			Port:            port,
			DBName:          dbname,
			User:            os.Getenv("ARQ_SIGNALS_TARGET_USER"),
			SSLMode:         os.Getenv("ARQ_SIGNALS_TARGET_SSLMODE"),
			SSLRootCertFile: os.Getenv("ARQ_SIGNALS_TARGET_SSLROOTCERT_FILE"),
			PasswordFile:    os.Getenv("ARQ_SIGNALS_TARGET_PASSWORD_FILE"),
			PasswordEnv:     os.Getenv("ARQ_SIGNALS_TARGET_PASSWORD_ENV"),
			PgpassFile:      os.Getenv("ARQ_SIGNALS_TARGET_PGPASS_FILE"),
			Enabled:         true,
		}
		cfg.Targets = append(cfg.Targets, tgt)
	}
	return nil
}

// Validate checks the Config for common issues, returning human-readable
// warnings. An empty slice means the config is healthy.
func Validate(cfg Config) []string {
	var issues []string

	if cfg.Signals.PollInterval < 10*time.Second {
		issues = append(issues, fmt.Sprintf("signals.poll_interval is very short (%s); minimum recommended is 30s", cfg.Signals.PollInterval))
	}
	if cfg.Signals.RetentionDays < 1 {
		issues = append(issues, "signals.retention_days is < 1; snapshots will be deleted immediately")
	}
	if cfg.Database.Path == "" {
		issues = append(issues, "database.path is empty")
	}
	if cfg.API.ListenAddr == "" {
		issues = append(issues, "api.listen_addr is empty")
	}
	if len(cfg.Targets) == 0 {
		issues = append(issues, "no targets configured; the collector will have nothing to collect")
	}
	for i, t := range cfg.Targets {
		if t.Name == "" {
			issues = append(issues, fmt.Sprintf("target[%d]: name is empty", i))
		}
		if t.Host == "" {
			issues = append(issues, fmt.Sprintf("target[%d] (%s): host is empty", i, t.Name))
		}
		if t.User == "" {
			issues = append(issues, fmt.Sprintf("target[%d] (%s): user is empty", i, t.Name))
		}
		if t.DBName == "" {
			issues = append(issues, fmt.Sprintf("target[%d] (%s): dbname is empty", i, t.Name))
		}
		// Reject multiple secret sources.
		secretCount := 0
		if t.PasswordFile != "" {
			secretCount++
		}
		if t.PasswordEnv != "" {
			secretCount++
		}
		if t.PgpassFile != "" {
			secretCount++
		}
		if secretCount > 1 {
			issues = append(issues, fmt.Sprintf("target[%d] (%s): specify at most one of password_file, password_env, pgpass_file", i, t.Name))
		}
		// Warn on weak sslmode.
		if t.SSLMode == "disable" || t.SSLMode == "allow" || t.SSLMode == "prefer" {
			issues = append(issues, fmt.Sprintf("target[%d] (%s): sslmode=%s is not recommended for production; consider require, verify-ca, or verify-full", i, t.Name, t.SSLMode))
		}
	}
	return issues
}

// weakSSLModes are sslmode values that do not provide adequate TLS guarantees.
var weakSSLModes = map[string]bool{
	"disable": true,
	"allow":   true,
	"prefer":  true,
	"require": true, // require does not verify server identity
}

// ValidateProdTLS enforces strict Postgres TLS requirements in production.
// In prod, all targets must use verify-ca or verify-full with a CA cert.
// In non-prod, ARQ_ALLOW_INSECURE_PG_TLS=true suppresses the error.
// Returns nil if all checks pass.
func ValidateProdTLS(cfg Config) error {
	isProd := cfg.Env == "prod"

	for i, t := range cfg.Targets {
		if !t.Enabled {
			continue
		}

		mode := t.SSLMode
		if mode == "" {
			mode = "prefer" // libpq default
		}

		if !weakSSLModes[mode] {
			// verify-ca or verify-full — check that sslrootcert is set.
			if t.SSLRootCertFile == "" {
				return fmt.Errorf("target[%d] (%s): sslmode=%s requires sslrootcert_file to be set", i, t.Name, mode)
			}
			continue
		}

		// Weak mode detected.
		if isProd {
			if cfg.AllowInsecurePgTLS {
				return fmt.Errorf("target[%d] (%s): ARQ_ALLOW_INSECURE_PG_TLS is not permitted in prod; use verify-ca or verify-full", i, t.Name)
			}
			return fmt.Errorf("target[%d] (%s): sslmode=%s is not allowed in prod; set sslmode=verify-full and provide sslrootcert_file", i, t.Name, mode)
		}

		// Non-prod: allow with override, warn otherwise.
		if !cfg.AllowInsecurePgTLS {
			return fmt.Errorf("target[%d] (%s): sslmode=%s is insecure; set ARQ_ALLOW_INSECURE_PG_TLS=true to allow in %s environment", i, t.Name, mode, cfg.Env)
		}
	}

	return nil
}

func parseDurations(cfg *Config) error {
	if cfg.Signals.PollIntervalS != "" {
		d, err := time.ParseDuration(cfg.Signals.PollIntervalS)
		if err != nil {
			return fmt.Errorf("parse signals.poll_interval %q: %w", cfg.Signals.PollIntervalS, err)
		}
		cfg.Signals.PollInterval = d
	}
	if cfg.API.ReadTimeoutS != "" {
		d, err := time.ParseDuration(cfg.API.ReadTimeoutS)
		if err != nil {
			return fmt.Errorf("parse api.read_timeout %q: %w", cfg.API.ReadTimeoutS, err)
		}
		cfg.API.ReadTimeout = d
	}
	if cfg.API.WriteTimeoutS != "" {
		d, err := time.ParseDuration(cfg.API.WriteTimeoutS)
		if err != nil {
			return fmt.Errorf("parse api.write_timeout %q: %w", cfg.API.WriteTimeoutS, err)
		}
		cfg.API.WriteTimeout = d
	}
	if cfg.Signals.TargetTimeoutS != "" {
		d, err := time.ParseDuration(cfg.Signals.TargetTimeoutS)
		if err != nil {
			return fmt.Errorf("parse signals.target_timeout %q: %w", cfg.Signals.TargetTimeoutS, err)
		}
		cfg.Signals.TargetTimeout = d
	}
	if cfg.Signals.QueryTimeoutS != "" {
		d, err := time.ParseDuration(cfg.Signals.QueryTimeoutS)
		if err != nil {
			return fmt.Errorf("parse signals.query_timeout %q: %w", cfg.Signals.QueryTimeoutS, err)
		}
		cfg.Signals.QueryTimeout = d
	}
	return nil
}
