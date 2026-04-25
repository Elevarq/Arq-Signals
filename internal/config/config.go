package config

import (
	"crypto/subtle"
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
	PollInterval                     time.Duration `yaml:"-"`
	PollIntervalS                    string        `yaml:"poll_interval"` // e.g. "5m"
	RetentionDays                    int           `yaml:"retention_days"`
	LogLevel                         string        `yaml:"log_level"`
	LogJSON                          bool          `yaml:"log_json"`
	MaxConcurrentTargets             int           `yaml:"max_concurrent_targets"`
	TargetTimeout                    time.Duration `yaml:"-"`
	TargetTimeoutS                   string        `yaml:"target_timeout"`
	QueryTimeout                     time.Duration `yaml:"-"`
	QueryTimeoutS                    string        `yaml:"query_timeout"`
	HighSensitivityCollectorsEnabled bool          `yaml:"high_sensitivity_collectors_enabled"`
	MetricsEnabled                   bool          `yaml:"metrics_enabled"`
	MetricsPath                      string        `yaml:"metrics_path"`
	// R083: Mode B opt-in. "standalone" (default) keeps Phase 2
	// behaviour byte-for-byte. "arq_managed" activates the
	// arq_control_plane_token check.
	Mode                     string `yaml:"mode"`
	ArqControlPlaneTokenFile string `yaml:"arq_control_plane_token_file"`
	ArqControlPlaneTokenEnv  string `yaml:"arq_control_plane_token_env"`
}

// R083 mode values.
const (
	ModeStandalone = "standalone"
	ModeArqManaged = "arq_managed"
)

// MinArqControlPlaneTokenLength is the floor for the R083 control-
// plane token. 32 chars matches the doc-stated minimum and is
// sufficient entropy for HMAC-equivalent strength.
const MinArqControlPlaneTokenLength = 32

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

// UnmarshalYAML decodes a TargetConfig with Enabled defaulting to true.
// Without this, an omitted `enabled:` key would deserialize to the zero
// value (false), silently disabling targets in configs that don't mention
// the field. Operators must use explicit `enabled: false` to disable a
// target.
func (t *TargetConfig) UnmarshalYAML(value *yaml.Node) error {
	type rawTarget TargetConfig
	raw := rawTarget{Enabled: true}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*t = TargetConfig(raw)
	return nil
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
			MetricsEnabled:       false,
			MetricsPath:          "/metrics",
			Mode:                 ModeStandalone,
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

// parseEnvInt returns the parsed integer for the given ARQ_SIGNALS_* env
// variable, or an error if the value is set but not a valid integer.
// Empty/unset returns ok=false with no error.
func parseEnvInt(name string) (int, bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false, fmt.Errorf("environment variable %s=%q is not a valid integer", name, v)
	}
	return n, true, nil
}

// parseEnvBool accepts "true"/"false"/"1"/"0" (case-insensitive). Any other
// non-empty value is a hard error so a typo like "yes" is not silently
// treated as false. Empty/unset returns ok=false with no error.
func parseEnvBool(name string) (bool, bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return false, false, nil
	}
	switch strings.ToLower(v) {
	case "true", "1":
		return true, true, nil
	case "false", "0":
		return false, true, nil
	}
	return false, false, fmt.Errorf("environment variable %s=%q is not a valid boolean (expected true/false/1/0)", name, v)
}

func applyEnvOverrides(cfg *Config) error {
	if v := os.Getenv("ARQ_ENV"); v != "" {
		cfg.Env = strings.ToLower(v)
	}
	if b, ok, err := parseEnvBool("ARQ_ALLOW_INSECURE_PG_TLS"); err != nil {
		return err
	} else if ok {
		cfg.AllowInsecurePgTLS = b
	}
	if b, ok, err := parseEnvBool("ARQ_SIGNALS_ALLOW_UNSAFE_ROLE"); err != nil {
		return err
	} else if ok {
		cfg.AllowUnsafeRole = b
	}
	if v := os.Getenv("ARQ_SIGNALS_POLL_INTERVAL"); v != "" {
		cfg.Signals.PollIntervalS = v
	}
	if n, ok, err := parseEnvInt("ARQ_SIGNALS_RETENTION_DAYS"); err != nil {
		return err
	} else if ok {
		cfg.Signals.RetentionDays = n
	}
	if v := os.Getenv("ARQ_SIGNALS_LOG_LEVEL"); v != "" {
		cfg.Signals.LogLevel = strings.ToLower(v)
	}
	if b, ok, err := parseEnvBool("ARQ_SIGNALS_LOG_JSON"); err != nil {
		return err
	} else if ok {
		cfg.Signals.LogJSON = b
	}
	if n, ok, err := parseEnvInt("ARQ_SIGNALS_MAX_CONCURRENT_TARGETS"); err != nil {
		return err
	} else if ok {
		cfg.Signals.MaxConcurrentTargets = n
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
	if b, ok, err := parseEnvBool("ARQ_SIGNALS_HIGH_SENSITIVITY_COLLECTORS_ENABLED"); err != nil {
		return err
	} else if ok {
		cfg.Signals.HighSensitivityCollectorsEnabled = b
	}
	if b, ok, err := parseEnvBool("ARQ_SIGNALS_METRICS_ENABLED"); err != nil {
		return err
	} else if ok {
		cfg.Signals.MetricsEnabled = b
	}
	if v := os.Getenv("ARQ_SIGNALS_METRICS_PATH"); v != "" {
		cfg.Signals.MetricsPath = v
	}
	// R083 — Mode B knobs.
	if v := os.Getenv("ARQ_SIGNALS_MODE"); v != "" {
		cfg.Signals.Mode = strings.ToLower(v)
	}
	if v := os.Getenv("ARQ_SIGNALS_ARQ_CONTROL_PLANE_TOKEN_FILE"); v != "" {
		cfg.Signals.ArqControlPlaneTokenFile = v
	}
	if v := os.Getenv("ARQ_SIGNALS_ARQ_CONTROL_PLANE_TOKEN_ENV"); v != "" {
		cfg.Signals.ArqControlPlaneTokenEnv = v
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
		if n, ok, err := parseEnvInt("ARQ_SIGNALS_TARGET_PORT"); err != nil {
			return err
		} else if ok {
			port = n
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

// ValidateStrict implements R076. It returns the list of non-fatal warnings
// (caller logs and continues) and a hard error that the caller must abort
// on. The hard / warn taxonomy is defined in
// `features/arq-signals/appendix-b-configuration-schema.md`.
func ValidateStrict(cfg Config) (warnings []string, err error) {
	// Hard errors first; we still gather as many as we can find before
	// returning so the operator sees the full picture in one run.
	var hard []string

	if cfg.Database.Path == "" {
		hard = append(hard, "database.path is empty")
	}
	if cfg.API.ListenAddr == "" {
		hard = append(hard, "api.listen_addr is empty")
	}
	if cfg.Signals.PollInterval <= 0 {
		hard = append(hard, "signals.poll_interval must be > 0")
	}
	if cfg.Signals.TargetTimeout <= 0 {
		hard = append(hard, "signals.target_timeout must be > 0")
	}
	if cfg.Signals.QueryTimeout <= 0 {
		hard = append(hard, "signals.query_timeout must be > 0")
	}
	if cfg.Signals.RetentionDays < 0 {
		hard = append(hard, "signals.retention_days must be >= 0")
	}

	if cfg.Signals.MetricsEnabled {
		path := cfg.Signals.MetricsPath
		switch {
		case !strings.HasPrefix(path, "/"):
			hard = append(hard, fmt.Sprintf("signals.metrics_path %q must start with /", path))
		case path == "/health":
			hard = append(hard, "signals.metrics_path must not be /health (reserved for liveness probes)")
		case path == "/status" || path == "/collect/now" || path == "/export":
			hard = append(hard, fmt.Sprintf("signals.metrics_path %q collides with an existing API path", path))
		}
	}

	// R083: mode + control-plane token configuration. Cross-token
	// equality and length checks happen later in ValidateModeBTokens
	// because they need the resolved api.token, which is generated
	// after Load returns.
	switch cfg.Signals.Mode {
	case "", ModeStandalone, ModeArqManaged:
		// allowed (empty == standalone via default)
	default:
		hard = append(hard, fmt.Sprintf("signals.mode %q must be %q or %q", cfg.Signals.Mode, ModeStandalone, ModeArqManaged))
	}
	if cfg.Signals.ArqControlPlaneTokenFile != "" && cfg.Signals.ArqControlPlaneTokenEnv != "" {
		hard = append(hard, "signals.arq_control_plane_token_file and signals.arq_control_plane_token_env are mutually exclusive — pick one")
	}
	if cfg.Signals.Mode == ModeArqManaged &&
		cfg.Signals.ArqControlPlaneTokenFile == "" &&
		cfg.Signals.ArqControlPlaneTokenEnv == "" {
		hard = append(hard, `signals.mode is "arq_managed" but no control-plane token is configured (set arq_control_plane_token_file or arq_control_plane_token_env)`)
	}

	seen := make(map[string]int, len(cfg.Targets))
	for i, t := range cfg.Targets {
		if t.Name == "" {
			hard = append(hard, fmt.Sprintf("target[%d]: name is required", i))
		} else if prev, ok := seen[t.Name]; ok {
			hard = append(hard, fmt.Sprintf("target[%d] (%s): duplicate name (also at target[%d])", i, t.Name, prev))
		} else {
			seen[t.Name] = i
		}
		if t.Host == "" {
			hard = append(hard, fmt.Sprintf("target[%d] (%s): host is required", i, t.Name))
		}
		if t.User == "" {
			hard = append(hard, fmt.Sprintf("target[%d] (%s): user is required", i, t.Name))
		}
		if t.DBName == "" {
			hard = append(hard, fmt.Sprintf("target[%d] (%s): dbname is required", i, t.Name))
		}
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
			hard = append(hard, fmt.Sprintf("target[%d] (%s): specify at most one of password_file, password_env, pgpass_file", i, t.Name))
		}
	}

	// Warnings.
	if cfg.Signals.PollInterval > 0 && cfg.Signals.PollInterval < 30*time.Second {
		warnings = append(warnings, fmt.Sprintf("signals.poll_interval is very short (%s); minimum recommended is 30s", cfg.Signals.PollInterval))
	}
	if cfg.Signals.RetentionDays == 0 {
		warnings = append(warnings, "signals.retention_days is 0; snapshots will be deleted on the next cleanup cycle")
	}
	if len(cfg.Targets) == 0 {
		warnings = append(warnings, "no targets configured; the collector will start but have nothing to collect")
	}
	for i, t := range cfg.Targets {
		if cfg.Env != "prod" && t.SSLMode == "prefer" {
			warnings = append(warnings, fmt.Sprintf("target[%d] (%s): sslmode=prefer does not verify server identity; consider verify-ca or verify-full", i, t.Name))
		}
	}

	if len(hard) > 0 {
		return warnings, fmt.Errorf("configuration is invalid:\n  - %s", strings.Join(hard, "\n  - "))
	}
	return warnings, nil
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

// ResolveArqControlPlaneToken reads the configured Arq control-plane
// token (R083). It is called per authentication attempt by the auth
// middleware so rotating the file's contents takes effect on the
// next request — no daemon restart required. Returns empty string
// when mode != arq_managed or when no source is configured (caller
// treats that as "control-plane auth disabled" without allocation).
//
// File source is preferred over env-var source. If both are set the
// file wins; ValidateStrict already rejects the both-set case at
// startup so this only matters under a lossy env reload.
func ResolveArqControlPlaneToken(s SignalsConfig) (string, error) {
	if s.Mode != ModeArqManaged {
		return "", nil
	}
	switch {
	case s.ArqControlPlaneTokenFile != "":
		data, err := os.ReadFile(s.ArqControlPlaneTokenFile)
		if err != nil {
			return "", fmt.Errorf("read arq_control_plane_token_file: %w", err)
		}
		// Strip a single trailing newline pair — same convention as
		// the api.token file and pgpass handling.
		return strings.TrimRight(string(data), "\n\r"), nil
	case s.ArqControlPlaneTokenEnv != "":
		v, ok := os.LookupEnv(s.ArqControlPlaneTokenEnv)
		if !ok {
			return "", fmt.Errorf("env var %q referenced by arq_control_plane_token_env is not set", s.ArqControlPlaneTokenEnv)
		}
		return v, nil
	}
	return "", nil
}

// ValidateModeBTokens runs the cross-token checks that depend on the
// resolved values of both tokens (R083): control-plane token length
// floor and distinctness from the API token. Called from main.go
// once both tokens are populated; not called when mode != arq_managed.
//
// arqToken is the resolved control-plane token (i.e. the result of
// ResolveArqControlPlaneToken at startup). apiToken is the
// effective api.token after auto-generation.
func ValidateModeBTokens(cfg Config, apiToken, arqToken string) error {
	if cfg.Signals.Mode != ModeArqManaged {
		return nil
	}
	if arqToken == "" {
		return fmt.Errorf("signals.arq_control_plane_token resolved to empty string — check the configured file or env var")
	}
	if len(arqToken) < MinArqControlPlaneTokenLength {
		return fmt.Errorf("signals.arq_control_plane_token must be at least %d characters", MinArqControlPlaneTokenLength)
	}
	if subtle.ConstantTimeCompare([]byte(arqToken), []byte(apiToken)) == 1 {
		return fmt.Errorf("signals.arq_control_plane_token must differ from api.token")
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
