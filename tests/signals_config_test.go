package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elevarq/arq-signals/internal/config"
)

// ---------------------------------------------------------------------------
// R027: Configuration via YAML + env vars (TC-SIG-040)
// ---------------------------------------------------------------------------

// TestConfigLoadFromYAML verifies that configuration is loaded from a
// YAML file with correct field parsing.
func TestConfigLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "signals.yaml")
	content := `
env: lab
signals:
  poll_interval: 10m
  retention_days: 7
  log_level: debug
  max_concurrent_targets: 2
  target_timeout: 30s
  query_timeout: 5s
targets:
  - name: test-db
    host: db.example.com
    port: 5433
    dbname: testdb
    user: monitor
    password_file: /tmp/pass
    sslmode: require
    enabled: true
database:
  path: /tmp/test.db
  wal: false
api:
  listen_addr: "0.0.0.0:9090"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Env != "lab" {
		t.Errorf("Env: got %q, want %q", cfg.Env, "lab")
	}
	if cfg.Signals.PollInterval != 10*time.Minute {
		t.Errorf("PollInterval: got %v, want 10m", cfg.Signals.PollInterval)
	}
	if cfg.Signals.RetentionDays != 7 {
		t.Errorf("RetentionDays: got %d, want 7", cfg.Signals.RetentionDays)
	}
	if cfg.Signals.MaxConcurrentTargets != 2 {
		t.Errorf("MaxConcurrentTargets: got %d, want 2", cfg.Signals.MaxConcurrentTargets)
	}
	if cfg.Signals.TargetTimeout != 30*time.Second {
		t.Errorf("TargetTimeout: got %v, want 30s", cfg.Signals.TargetTimeout)
	}
	if cfg.Signals.QueryTimeout != 5*time.Second {
		t.Errorf("QueryTimeout: got %v, want 5s", cfg.Signals.QueryTimeout)
	}
	if len(cfg.Targets) != 1 {
		t.Fatalf("Targets: got %d, want 1", len(cfg.Targets))
	}
	tgt := cfg.Targets[0]
	if tgt.Name != "test-db" || tgt.Host != "db.example.com" || tgt.Port != 5433 {
		t.Errorf("Target: got %+v", tgt)
	}
	if cfg.Database.Path != "/tmp/test.db" || cfg.Database.WAL {
		t.Errorf("Database: got path=%q wal=%v", cfg.Database.Path, cfg.Database.WAL)
	}
	if cfg.API.ListenAddr != "0.0.0.0:9090" {
		t.Errorf("ListenAddr: got %q, want %q", cfg.API.ListenAddr, "0.0.0.0:9090")
	}
}

// TestConfigEnvOverridesFile verifies that environment variables take
// precedence over YAML file values.
func TestConfigEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "signals.yaml")
	content := `
signals:
  poll_interval: 10m
  retention_days: 30
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ARQ_SIGNALS_POLL_INTERVAL", "2m")
	t.Setenv("ARQ_SIGNALS_RETENTION_DAYS", "3")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Signals.PollInterval != 2*time.Minute {
		t.Errorf("PollInterval: got %v, want 2m (env should override file)", cfg.Signals.PollInterval)
	}
	if cfg.Signals.RetentionDays != 3 {
		t.Errorf("RetentionDays: got %d, want 3 (env should override file)", cfg.Signals.RetentionDays)
	}
}

// ---------------------------------------------------------------------------
// R028: Config file search order (TC-SIG-040)
// ---------------------------------------------------------------------------

// TestConfigDefaultsWithNoFile verifies that when no config file is
// found, sensible defaults are returned.
func TestConfigDefaultsWithNoFile(t *testing.T) {
	// Load from a nonexistent directory so no signals.yaml is found.
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Env != "dev" {
		t.Errorf("Default Env: got %q, want %q", cfg.Env, "dev")
	}
	if cfg.Signals.PollInterval != 5*time.Minute {
		t.Errorf("Default PollInterval: got %v, want 5m", cfg.Signals.PollInterval)
	}
	if cfg.Signals.RetentionDays != 30 {
		t.Errorf("Default RetentionDays: got %d, want 30", cfg.Signals.RetentionDays)
	}
	if cfg.API.ListenAddr != "127.0.0.1:8081" {
		t.Errorf("Default ListenAddr: got %q, want %q", cfg.API.ListenAddr, "127.0.0.1:8081")
	}
}

// ---------------------------------------------------------------------------
// R029: Single-target container mode via env (TC-SIG-040)
// ---------------------------------------------------------------------------

// TestConfigSingleTargetFromEnv verifies that setting ARQ_SIGNALS_TARGET_HOST
// creates a target from environment variables.
func TestConfigSingleTargetFromEnv(t *testing.T) {
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("ARQ_SIGNALS_TARGET_HOST", "pg.internal")
	t.Setenv("ARQ_SIGNALS_TARGET_PORT", "5433")
	t.Setenv("ARQ_SIGNALS_TARGET_DBNAME", "myapp")
	t.Setenv("ARQ_SIGNALS_TARGET_USER", "monitor")
	t.Setenv("ARQ_SIGNALS_TARGET_NAME", "container-db")
	t.Setenv("ARQ_SIGNALS_TARGET_PASSWORD_ENV", "PG_PASS")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Targets) != 1 {
		t.Fatalf("Targets: got %d, want 1", len(cfg.Targets))
	}
	tgt := cfg.Targets[0]
	if tgt.Host != "pg.internal" {
		t.Errorf("Host: got %q, want %q", tgt.Host, "pg.internal")
	}
	if tgt.Port != 5433 {
		t.Errorf("Port: got %d, want 5433", tgt.Port)
	}
	if tgt.DBName != "myapp" {
		t.Errorf("DBName: got %q, want %q", tgt.DBName, "myapp")
	}
	if tgt.User != "monitor" {
		t.Errorf("User: got %q, want %q", tgt.User, "monitor")
	}
	if tgt.Name != "container-db" {
		t.Errorf("Name: got %q, want %q", tgt.Name, "container-db")
	}
	if tgt.PasswordEnv != "PG_PASS" {
		t.Errorf("PasswordEnv: got %q, want %q", tgt.PasswordEnv, "PG_PASS")
	}
	if !tgt.Enabled {
		t.Error("Enabled: expected true for env-configured target")
	}
}

// TestConfigSingleTargetDefaultName verifies that the target name defaults
// to "default" when ARQ_SIGNALS_TARGET_NAME is not set.
func TestConfigSingleTargetDefaultName(t *testing.T) {
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("ARQ_SIGNALS_TARGET_HOST", "localhost")

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Targets) != 1 {
		t.Fatalf("Targets: got %d, want 1", len(cfg.Targets))
	}
	if cfg.Targets[0].Name != "default" {
		t.Errorf("Name: got %q, want %q", cfg.Targets[0].Name, "default")
	}
	if cfg.Targets[0].DBName != "postgres" {
		t.Errorf("DBName: got %q, want %q (default)", cfg.Targets[0].DBName, "postgres")
	}
}

// ---------------------------------------------------------------------------
// R030: Config validation at startup (TC-SIG-040)
// ---------------------------------------------------------------------------

// TestConfigValidateCatchesIssues verifies that Validate returns warnings
// for common misconfigurations.
func TestConfigValidateCatchesIssues(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Signals.PollInterval = 5 * time.Second // too short
	cfg.Signals.RetentionDays = 0              // disables cleanup
	cfg.Database.Path = ""                     // empty
	cfg.API.ListenAddr = ""                    // empty
	cfg.Targets = nil                          // no targets

	issues := config.Validate(cfg)
	if len(issues) < 4 {
		t.Errorf("expected at least 4 validation warnings, got %d: %v", len(issues), issues)
	}
}

// TestConfigValidateRejectsMultipleSecretSources verifies that specifying
// more than one credential source per target is flagged.
func TestConfigValidateRejectsMultipleSecretSources(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Targets = []config.TargetConfig{
		{
			Name:         "test",
			Host:         "localhost",
			DBName:       "db",
			User:         "user",
			PasswordFile: "/tmp/pass",
			PasswordEnv:  "PG_PASS",
		},
	}

	issues := config.Validate(cfg)
	found := false
	for _, issue := range issues {
		if contains(issue, "at most one of") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation warning about multiple secret sources, got: %v", issues)
	}
}

// TestConfigValidateInvalidDuration verifies that an unparseable duration
// string causes Load to return an error.
func TestConfigValidateInvalidDuration(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "signals.yaml")
	content := `
signals:
  poll_interval: not-a-duration
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Error("expected error for unparseable duration, got nil")
	}
}

// ---------------------------------------------------------------------------
// R076: Strict configuration validation
// ---------------------------------------------------------------------------

// TestValidateStrictAcceptsValidConfig verifies the happy path: a complete,
// healthy config returns no error and no warnings.
// Traces: ARQ-SIGNALS-R076
func TestValidateStrictAcceptsValidConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Targets = []config.TargetConfig{
		{
			Name:    "primary",
			Host:    "db.example.com",
			Port:    5432,
			DBName:  "app",
			User:    "monitor",
			SSLMode: "verify-full",
			Enabled: true,
		},
	}
	warnings, err := config.ValidateStrict(cfg)
	if err != nil {
		t.Fatalf("ValidateStrict returned hard error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("ValidateStrict returned warnings on healthy config: %v", warnings)
	}
}

// TestValidateStrictAggregatesHardErrors verifies that ValidateStrict reports
// every hard error in a single message rather than failing on the first.
// Traces: ARQ-SIGNALS-R076
func TestValidateStrictAggregatesHardErrors(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Database.Path = ""
	cfg.API.ListenAddr = ""
	cfg.Targets = []config.TargetConfig{
		{Name: "a", Host: "h", DBName: "d", User: ""}, // missing user
		{Name: "a", Host: "", DBName: "d", User: "u"}, // duplicate name + missing host
	}
	_, err := config.ValidateStrict(cfg)
	if err == nil {
		t.Fatal("expected hard error")
	}
	msg := err.Error()
	for _, want := range []string{
		"database.path is empty",
		"api.listen_addr is empty",
		"user is required",
		"host is required",
		"duplicate name",
	} {
		if !contains(msg, want) {
			t.Errorf("expected error to contain %q, got:\n%s", want, msg)
		}
	}
}

// TestValidateStrictRejectsMultipleSecretSources verifies that more than one
// of password_file/password_env/pgpass_file is a hard error.
// Traces: ARQ-SIGNALS-R076
func TestValidateStrictRejectsMultipleSecretSources(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Targets = []config.TargetConfig{
		{
			Name:         "p",
			Host:         "h",
			DBName:       "d",
			User:         "u",
			SSLMode:      "verify-full",
			PasswordFile: "/run/secrets/pw",
			PasswordEnv:  "PG_PASS",
			Enabled:      true,
		},
	}
	_, err := config.ValidateStrict(cfg)
	if err == nil || !contains(err.Error(), "specify at most one") {
		t.Fatalf("expected secret-source conflict, got %v", err)
	}
}

// TestLoadRejectsMalformedIntEnv verifies that a non-integer in
// ARQ_SIGNALS_RETENTION_DAYS aborts Load instead of being silently dropped.
// Traces: ARQ-SIGNALS-R076
func TestLoadRejectsMalformedIntEnv(t *testing.T) {
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("ARQ_SIGNALS_RETENTION_DAYS", "thirty")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for malformed integer env var")
	}
	if !contains(err.Error(), "ARQ_SIGNALS_RETENTION_DAYS") {
		t.Errorf("error should name the offending env var, got: %v", err)
	}
}

// TestLoadRejectsMalformedBoolEnv verifies that a non-boolean in
// ARQ_ALLOW_INSECURE_PG_TLS aborts Load. Previously "yes" silently became
// false.
// Traces: ARQ-SIGNALS-R076
func TestLoadRejectsMalformedBoolEnv(t *testing.T) {
	origDir, _ := os.Getwd()
	dir := t.TempDir()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	t.Setenv("ARQ_ALLOW_INSECURE_PG_TLS", "yes")
	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for malformed boolean env var")
	}
	if !contains(err.Error(), "ARQ_ALLOW_INSECURE_PG_TLS") {
		t.Errorf("error should name the offending env var, got: %v", err)
	}
}

// TestTargetEnabledDefaultsToTrue verifies R076: a target without an explicit
// `enabled:` field is treated as enabled. The previous zero-value behaviour
// silently disabled targets in any minimal config.
// Traces: ARQ-SIGNALS-R076
func TestTargetEnabledDefaultsToTrue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "signals.yaml")
	content := `
targets:
  - name: implicit-on
    host: db.example.com
    dbname: app
    user: monitor
  - name: explicit-on
    host: db.example.com
    dbname: app
    user: monitor
    enabled: true
  - name: explicit-off
    host: db.example.com
    dbname: app
    user: monitor
    enabled: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(cfg.Targets))
	}

	cases := []struct {
		name string
		want bool
	}{
		{"implicit-on", true},
		{"explicit-on", true},
		{"explicit-off", false},
	}
	for i, c := range cases {
		if cfg.Targets[i].Name != c.name {
			t.Fatalf("target[%d] name = %q, want %q", i, cfg.Targets[i].Name, c.name)
		}
		if cfg.Targets[i].Enabled != c.want {
			t.Errorf("target %q Enabled = %v, want %v", c.name, cfg.Targets[i].Enabled, c.want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
