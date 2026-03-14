# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |

## Reporting a vulnerability

If you discover a security vulnerability in arq-signals, please report it
responsibly:

1. **Do not** open a public GitHub issue
2. Email security@elevarq.com with:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
3. We will acknowledge receipt within 48 hours
4. We will provide a fix timeline within 5 business days

## Security model

### PostgreSQL connections

arq-signals enforces read-only access through three independent layers:

1. **Static linting**: All SQL queries are validated at startup. DDL, DML, and
   dangerous functions cause the process to abort immediately.
2. **Session-level**: Connections use `default_transaction_read_only=on`.
3. **Per-query**: Each query runs inside `BEGIN ... READ ONLY`.

### Credentials

- Passwords are read from file or environment variable at connection time
- Passwords are never cached in memory beyond a single connection attempt
- Passwords are never written to SQLite
- Passwords are never included in snapshot exports
- Password rotation is supported (re-read on each connection)

### Network

- The HTTP API binds to a configurable address (default `127.0.0.1:8081`)
- arq-signals makes no outbound network connections except to PostgreSQL targets
- No data is sent to external services, AI providers, or analytics platforms

### Data handling

- Snapshots contain only PostgreSQL statistics view data
- No credentials, DSNs, or secrets appear in exports
- SQLite database is stored locally with no remote replication

### Container hardening (when deployed via Docker)

- Non-root runtime (UID 10001)
- Minimal Alpine base image
- No shell or compilers in production image
