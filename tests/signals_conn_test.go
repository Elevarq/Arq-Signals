package tests

import (
	"testing"

	"github.com/elevarq/arq-signals/internal/collector"
	"github.com/elevarq/arq-signals/internal/config"
)

// TestBuildConnConfigValid verifies that BuildConnConfig creates a valid pgx.ConnConfig
// from a TargetConfig with host, port, dbname, and user.
// Traces: ARQ-SIGNALS-R001 / TC-SIG-001
func TestBuildConnConfigValid(t *testing.T) {
	tgt := config.TargetConfig{
		Name:   "test-target",
		Host:   "db.example.com",
		Port:   5433,
		DBName: "mydb",
		User:   "monitor",
		// No password source => peer/trust auth, ResolvePassword returns "".
	}

	cfg, err := collector.BuildConnConfig(tgt)
	if err != nil {
		t.Fatalf("BuildConnConfig returned error: %v", err)
	}

	if cfg.Host != "db.example.com" {
		t.Errorf("Host = %q, want %q", cfg.Host, "db.example.com")
	}
	if cfg.Port != 5433 {
		t.Errorf("Port = %d, want %d", cfg.Port, 5433)
	}
	if cfg.Database != "mydb" {
		t.Errorf("Database = %q, want %q", cfg.Database, "mydb")
	}
	if cfg.User != "monitor" {
		t.Errorf("User = %q, want %q", cfg.User, "monitor")
	}
}

// TestBuildConnConfigApplicationName verifies that BuildConnConfig sets application_name
// to "arq-signals" in the runtime parameters.
// Traces: ARQ-SIGNALS-R001 / TC-SIG-001
func TestBuildConnConfigApplicationName(t *testing.T) {
	tgt := config.TargetConfig{
		Name:   "test-target",
		Host:   "localhost",
		Port:   5432,
		DBName: "postgres",
		User:   "arq",
	}

	cfg, err := collector.BuildConnConfig(tgt)
	if err != nil {
		t.Fatalf("BuildConnConfig returned error: %v", err)
	}

	appName, ok := cfg.RuntimeParams["application_name"]
	if !ok {
		t.Fatal("application_name not set in RuntimeParams")
	}
	if appName != "arq-signals" {
		t.Errorf("application_name = %q, want %q", appName, "arq-signals")
	}
}

// TestBuildConnConfigDefaultPort verifies that BuildConnConfig defaults to port 5432
// when the target config has port == 0.
// Traces: ARQ-SIGNALS-R001 / TC-SIG-001
func TestBuildConnConfigDefaultPort(t *testing.T) {
	tgt := config.TargetConfig{
		Name:   "test-target",
		Host:   "localhost",
		Port:   0, // should default to 5432
		DBName: "postgres",
		User:   "arq",
	}

	cfg, err := collector.BuildConnConfig(tgt)
	if err != nil {
		t.Fatalf("BuildConnConfig returned error: %v", err)
	}

	if cfg.Port != 5432 {
		t.Errorf("Port = %d, want default 5432", cfg.Port)
	}
}

// TestBuildConnConfigEmptyHostError verifies that BuildConnConfig returns an error
// when the host is empty.
// Traces: ARQ-SIGNALS-R001 / TC-SIG-001
func TestBuildConnConfigEmptyHostError(t *testing.T) {
	tgt := config.TargetConfig{
		Name:   "bad-target",
		Host:   "",
		Port:   5432,
		DBName: "postgres",
		User:   "arq",
	}

	_, err := collector.BuildConnConfig(tgt)
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

// TestBuildConnConfigReadOnlyParam verifies that BuildConnConfig sets
// default_transaction_read_only=on in runtime parameters.
// Traces: ARQ-SIGNALS-R013 / TC-SIG-019
func TestBuildConnConfigReadOnlyParam(t *testing.T) {
	tgt := config.TargetConfig{
		Name:   "ro-target",
		Host:   "localhost",
		Port:   5432,
		DBName: "postgres",
		User:   "arq",
	}

	cfg, err := collector.BuildConnConfig(tgt)
	if err != nil {
		t.Fatalf("BuildConnConfig returned error: %v", err)
	}

	val, ok := cfg.RuntimeParams["default_transaction_read_only"]
	if !ok {
		t.Fatal("default_transaction_read_only not set in RuntimeParams")
	}
	if val != "on" {
		t.Errorf("default_transaction_read_only = %q, want %q", val, "on")
	}
}
