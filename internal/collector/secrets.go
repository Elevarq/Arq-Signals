package collector

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/elevarq/arq-signals/internal/config"
)

// BuildConnConfig creates a pgx.ConnConfig from structured target fields,
// resolving the password at call time from the configured secret source.
// Passwords are never cached — they are read fresh on every call to support rotation.
func BuildConnConfig(tgt config.TargetConfig) (*pgx.ConnConfig, error) {
	port := tgt.Port
	if port == 0 {
		port = 5432
	}

	host := tgt.Host
	if host == "" {
		return nil, fmt.Errorf("target %s: host is required", tgt.Name)
	}

	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s",
		host, port, tgt.DBName, tgt.User)

	if tgt.SSLMode != "" {
		connStr += fmt.Sprintf(" sslmode=%s", tgt.SSLMode)
	}
	if tgt.SSLRootCertFile != "" {
		connStr += fmt.Sprintf(" sslrootcert=%s", tgt.SSLRootCertFile)
	}

	cfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("target %s: parse config: %w", tgt.Name, err)
	}

	// Resolve password from configured secret source.
	password, err := ResolvePassword(tgt)
	if err != nil {
		return nil, fmt.Errorf("target %s: resolve password: %w", tgt.Name, redactError(err))
	}
	if password != "" {
		cfg.Password = password
	}

	cfg.RuntimeParams["application_name"] = "arq-signals"
	cfg.RuntimeParams["default_transaction_read_only"] = "on"

	return cfg, nil
}

// ResolvePassword reads the password from the configured secret source.
// Returns empty string if no secret source is configured (peer/trust auth).
func ResolvePassword(tgt config.TargetConfig) (string, error) {
	switch {
	case tgt.PasswordFile != "":
		return readPasswordFile(tgt.PasswordFile)
	case tgt.PasswordEnv != "":
		return readPasswordEnv(tgt.PasswordEnv)
	case tgt.PgpassFile != "":
		port := tgt.Port
		if port == 0 {
			port = 5432
		}
		return readPgpass(tgt.PgpassFile, tgt.Host, port, tgt.DBName, tgt.User)
	default:
		return "", nil
	}
}

func readPasswordFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read password_file: %w", err)
	}
	// Trim trailing newline (common in Docker secrets).
	return strings.TrimRight(string(data), "\n\r"), nil
}

func readPasswordEnv(envVar string) (string, error) {
	val, ok := os.LookupEnv(envVar)
	if !ok {
		return "", fmt.Errorf("password_env %q is not set", envVar)
	}
	return val, nil
}

// readPgpass reads a pgpass-format file and returns the matching password.
// Format: hostname:port:database:username:password
// Wildcard (*) matches any value in that field.
func readPgpass(path string, host string, port int, dbname string, user string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pgpass_file: %w", err)
	}
	defer f.Close()

	portStr := strconv.Itoa(port)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// Skip comments and empty lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := parsePgpassLine(line)
		if len(fields) != 5 {
			continue
		}

		if pgpassFieldMatch(fields[0], host) &&
			pgpassFieldMatch(fields[1], portStr) &&
			pgpassFieldMatch(fields[2], dbname) &&
			pgpassFieldMatch(fields[3], user) {
			return fields[4], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read pgpass_file: %w", err)
	}

	return "", fmt.Errorf("no matching entry in pgpass_file %s for %s:%d/%s@%s", path, host, port, dbname, user)
}

// parsePgpassLine splits a pgpass line into fields, handling escaped colons (\:)
// and escaped backslashes (\\).
func parsePgpassLine(line string) []string {
	var fields []string
	var current strings.Builder
	escaped := false

	for _, ch := range line {
		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == ':' {
			fields = append(fields, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(ch)
	}
	fields = append(fields, current.String())
	return fields
}

func pgpassFieldMatch(pattern, value string) bool {
	return pattern == "*" || pattern == value
}

// redactError wraps an error to ensure passwords don't leak into error messages.
// It replaces the original error message with a generic one if it might contain secrets.
func redactError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	// If the error might contain a password value, redact it.
	if strings.Contains(msg, "password") || strings.Contains(msg, "secret") {
		return fmt.Errorf("credential resolution failed (details redacted)")
	}
	return err
}

// RedactDSN takes a connection string that might contain embedded credentials
// and returns a safe version for logging.
func RedactDSN(dsn string) string {
	// Handle postgres:// URL format.
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// Find the userinfo section and redact password.
		if atIdx := strings.Index(dsn, "@"); atIdx != -1 {
			prefix := dsn[:strings.Index(dsn, "//")+2]
			userinfo := dsn[len(prefix):atIdx]
			rest := dsn[atIdx:]
			if colonIdx := strings.Index(userinfo, ":"); colonIdx != -1 {
				return prefix + userinfo[:colonIdx] + ":****" + rest
			}
		}
	}
	// Handle key=value format.
	if strings.Contains(dsn, "password=") {
		parts := strings.Fields(dsn)
		for i, part := range parts {
			if strings.HasPrefix(part, "password=") {
				parts[i] = "password=****"
			}
		}
		return strings.Join(parts, " ")
	}
	return dsn
}

// SafeTargetAddr returns a loggable host:port string for the target.
func SafeTargetAddr(tgt config.TargetConfig) string {
	port := tgt.Port
	if port == 0 {
		port = 5432
	}
	return net.JoinHostPort(tgt.Host, strconv.Itoa(port))
}
