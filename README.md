# arq-signals

Lightweight, read-only PostgreSQL diagnostic collector. Runs on your
infrastructure, collects statistics from your databases, and packages
them as portable snapshots. No data leaves your machine. No AI. No
cloud. Just structured evidence from the views PostgreSQL already
exposes.

[![CI](https://github.com/elevarq/arq-signals/actions/workflows/ci.yml/badge.svg)](https://github.com/elevarq/arq-signals/actions/workflows/ci.yml)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/elevarq/arq-signals)](https://goreportcard.com/report/github.com/elevarq/arq-signals)

> **Read-only by design** — three independent enforcement layers prevent
> any write operations. Unsafe roles (superuser, replication) are blocked
> before collection starts.
> [Every SQL query is in the source.](internal/pgqueries/catalog.go)
>
> **No cloud, no phone-home** — all data stays on your machine. No
> telemetry, no analytics, no external network calls.
>
> **No AI inside** — arq-signals is a pure data collector. No language
> models, no scoring, no recommendations. What you collect is what you get.
>
> **Built for restricted environments** — runs airgapped, as a non-root
> container, with no internet dependency. Suitable for networks where
> third-party monitoring agents are not permitted.

---

## Try it in 2 minutes

```bash
git clone https://github.com/elevarq/arq-signals.git
cd arq-signals
docker compose -f examples/docker-compose.yml up -d
```

This starts arq-signals alongside PostgreSQL 16 with a pre-configured
monitoring role. Collection begins automatically.

```bash
# Trigger an immediate collection
curl -X POST http://localhost:8081/collect/now \
  -H "Authorization: Bearer test-token"

# Download your first snapshot
curl -o snapshot.zip http://localhost:8081/export \
  -H "Authorization: Bearer test-token"

# Inspect the contents
unzip -l snapshot.zip
```

Your snapshot contains raw PostgreSQL statistics in structured JSON —
nothing more. See [`examples/snapshot-example/`](examples/snapshot-example/)
for what the output looks like.

---

## Why arq-signals exists

Every PostgreSQL instance exposes diagnostic data through built-in
statistics views. But collecting this data consistently, safely, and in
a format you can actually use takes tooling that most teams end up
building themselves.

arq-signals handles the collection part so you don't have to. It
connects with a read-only role, runs approved SQL queries on a schedule,
and writes structured results to local storage. When you need the data
elsewhere, it packages everything as a portable ZIP snapshot.

The project is open source because we think data collection should be
transparent. You can read every SQL query arq-signals will run. You can
audit the binary. You own the output.

## What arq-signals does

- Connects to one or more PostgreSQL instances (14+)
- Executes a versioned catalog of read-only SQL collectors
- Stores results locally in SQLite as structured NDJSON
- Schedules collection with configurable cadences (5m to 7d per query)
- Packages snapshots as portable ZIP archives
- Exposes a local HTTP API for triggering collection and exports
- Provides a CLI (`arqctl`) for operations

## What arq-signals does NOT do

arq-signals is a collector, not an analyzer. It produces raw diagnostic
evidence and stops there.

- **No analysis** — does not interpret, score, or grade your database
- **No recommendations** — does not suggest configuration changes
- **No AI / LLM** — does not connect to any AI service or language model
- **No network calls** — does not phone home, report telemetry, or
  contact external services
- **No write operations** — enforces read-only access at three
  independent layers

If you want automated analysis and reporting on top of arq-signals
snapshots, see [Compatibility with Arq Analyzer](#compatibility-with-arq-analyzer)
below.

## Installation

### Docker Compose (recommended for trying)

```bash
git clone https://github.com/elevarq/arq-signals.git
cd arq-signals
docker compose -f examples/docker-compose.yml up -d
```

### Docker (bring your own PostgreSQL)

```bash
docker run -d --name arq-signals \
  -e ARQ_SIGNALS_TARGET_HOST=host.docker.internal \
  -e ARQ_SIGNALS_TARGET_USER=arq_monitor \
  -e ARQ_SIGNALS_TARGET_DBNAME=postgres \
  -e ARQ_SIGNALS_TARGET_PASSWORD_ENV=PG_PASSWORD \
  -e PG_PASSWORD=your_password \
  -e ARQ_ALLOW_INSECURE_PG_TLS=true \
  -e ARQ_ENV=dev \
  -v arq-data:/data \
  -p 8081:8081 \
  ghcr.io/elevarq/arq-signals:latest
```

### Build from source

```bash
git clone https://github.com/elevarq/arq-signals.git
cd arq-signals
make build    # produces bin/arq-signals and bin/arqctl
./bin/arq-signals --config signals.yaml
```

See [`examples/signals.yaml`](examples/signals.yaml) for a complete
annotated configuration file.

### PostgreSQL setup

arq-signals needs a monitoring role with read access to statistics views:

```sql
CREATE ROLE arq_monitor WITH LOGIN PASSWORD 'choose_a_strong_password';
GRANT pg_monitor TO arq_monitor;

-- Optional: enable query-level statistics
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

## Using arq-signals

### Trigger a collection

```bash
# Via CLI
arqctl collect now

# Via API
curl -X POST http://localhost:8081/collect/now \
  -H "Authorization: Bearer $ARQ_SIGNALS_API_TOKEN"
```

### Export snapshots

```bash
# Via CLI
arqctl export --output snapshot.zip

# Via API
curl -o snapshot.zip http://localhost:8081/export \
  -H "Authorization: Bearer $ARQ_SIGNALS_API_TOKEN"
```

### Check status

```bash
arqctl status
```

## Snapshot format

arq-signals produces snapshots in the `arq-snapshot.v1` format:

```
snapshot.zip
├── metadata.json          # collector version, timestamp, PG version
├── query_catalog.json     # which queries were executed
├── query_runs.ndjson      # execution metadata (timing, row counts, errors)
├── query_results.ndjson   # the actual data (one JSON object per row)
└── snapshots.ndjson       # legacy combined format
```

Example `metadata.json`:

```json
{
  "schema_version": "arq-snapshot.v1",
  "collector_version": "0.1.0",
  "collector_commit": "abc1234",
  "collected_at": "2026-03-14T10:30:00Z",
  "instance_id": "a1b2c3d4e5f6"
}
```

Example `query_results.ndjson` (one line per query):

```json
{"run_id":"01JD...","payload":[{"name":"max_connections","setting":"100","unit":"","source":"configuration file"},{"name":"shared_buffers","setting":"16384","unit":"8kB","source":"configuration file"}]}
```

The format is versioned. Breaking changes will bump `schema_version`.

A complete example snapshot is available at
[`examples/snapshot-example/`](examples/snapshot-example/) — you can
inspect exactly what arq-signals collects without running it.

## Collected signals

| Query ID | PostgreSQL source | Cadence | Notes |
|----------|-------------------|---------|-------|
| `pg_version_v1` | `version()` | 6h | Server version string |
| `pg_settings_v1` | `pg_settings` | 6h | All runtime parameters |
| `pg_stat_activity_v1` | `pg_stat_activity` | 5m | Active sessions |
| `pg_stat_database_v1` | `pg_stat_database` | 15m | Database-level counters |
| `pg_stat_user_tables_v1` | `pg_stat_user_tables` | 15m | Table scan/tuple stats |
| `pg_stat_user_indexes_v1` | `pg_stat_user_indexes` | 15m | Index usage stats |
| `pg_statio_user_tables_v1` | `pg_statio_user_tables` | 15m | Table I/O stats |
| `pg_statio_user_indexes_v1` | `pg_statio_user_indexes` | 15m | Index I/O stats |
| `pg_stat_statements_v1` | `pg_stat_statements` | 15m | Query statistics (requires extension) |
| `wraparound_db_level_v1` | `pg_database` | 15m | Transaction ID age by database |
| `wraparound_rel_level_v1` | `pg_class` | 15m | Transaction ID age by table |
| `wraparound_blockers_v1` | `pg_stat_activity` | 5m | Long-running transactions blocking wraparound |

Every query is visible in
[`internal/pgqueries/catalog.go`](internal/pgqueries/catalog.go).
Queries requiring unavailable extensions are silently skipped.

## API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/health` | No | Liveness probe, always 200 |
| `GET` | `/status` | Bearer | Collector status, targets, last collection |
| `POST` | `/collect/now` | Bearer | Trigger immediate collection |
| `GET` | `/export` | Bearer | Download snapshot ZIP |

Set `ARQ_SIGNALS_API_TOKEN` to configure the bearer token. If unset, a
random token is generated at startup and logged.

## Security and data handling

### Read-only enforcement (three layers)

1. **Static linting** — every SQL query is validated at startup. DDL
   (`CREATE`, `ALTER`, `DROP`), DML (`INSERT`, `UPDATE`, `DELETE`), and
   dangerous functions (`pg_terminate_backend`, `pg_sleep`) cause the
   process to abort immediately.
2. **Session-level** — all connections set
   `default_transaction_read_only=on`.
3. **Per-query** — each query runs inside `BEGIN ... READ ONLY`.

### Role safety validation (fail-closed)

Before collecting from any target, arq-signals validates the connected
role's safety posture. Collection is **blocked** if the role has:

- Superuser privileges (`rolsuper=true`)
- Replication privileges (`rolreplication=true`)
- Bypass RLS privileges (`rolbypassrls=true`)

This is enforced by default with no configuration needed. Use a
dedicated monitoring role with `pg_monitor` for safe collection.
See [docs/runtime-safety-model.md](docs/runtime-safety-model.md) for
details.

### Credentials

- Passwords are read from file or environment variable at connection
  time
- Passwords are never cached in memory beyond a single connection
  attempt
- Passwords are never written to SQLite
- Passwords never appear in snapshots or exports
- Password rotation is supported (re-read on each new connection)

### Network

- arq-signals makes **no outbound network connections** except to your
  PostgreSQL targets
- No telemetry, no analytics, no phone-home
- The HTTP API binds to loopback by default (`127.0.0.1:8081`)

### Container hardening

When deployed via Docker, arq-signals runs as a non-root user
(UID 10001) on a minimal Alpine base with no shell in the production
image.

## Configuration reference

arq-signals reads configuration from (in order):
1. `--config` flag
2. `/etc/arq/signals.yaml`
3. `./signals.yaml`

Environment variables override file-based config. See
[`examples/signals.yaml`](examples/signals.yaml) for a complete
annotated example.

| Environment variable | Description | Default |
|---------------------|-------------|---------|
| `ARQ_ENV` | Environment: dev, lab, prod | dev |
| `ARQ_ALLOW_INSECURE_PG_TLS` | Allow weak TLS in non-prod | false |
| `ARQ_SIGNALS_ALLOW_UNSAFE_ROLE` | Allow unsafe role attributes (lab/dev only) | false |
| `ARQ_SIGNALS_TARGET_HOST` | PostgreSQL host | -- |
| `ARQ_SIGNALS_TARGET_PORT` | PostgreSQL port | 5432 |
| `ARQ_SIGNALS_TARGET_DBNAME` | Database name | postgres |
| `ARQ_SIGNALS_TARGET_USER` | Username | -- |
| `ARQ_SIGNALS_TARGET_NAME` | Target name | default |
| `ARQ_SIGNALS_TARGET_PASSWORD_FILE` | Path to password file | -- |
| `ARQ_SIGNALS_TARGET_PASSWORD_ENV` | Env var containing the password | -- |
| `ARQ_SIGNALS_TARGET_PGPASS_FILE` | Path to pgpass file | -- |
| `ARQ_SIGNALS_TARGET_SSLMODE` | TLS mode | -- |
| `ARQ_SIGNALS_POLL_INTERVAL` | Collection interval | 5m |
| `ARQ_SIGNALS_RETENTION_DAYS` | Days to retain data | 30 |
| `ARQ_SIGNALS_LOG_LEVEL` | Log level: debug, info, warn, error | info |
| `ARQ_SIGNALS_LOG_JSON` | JSON log format | false |
| `ARQ_SIGNALS_MAX_CONCURRENT_TARGETS` | Max parallel targets | 4 |
| `ARQ_SIGNALS_TARGET_TIMEOUT` | Per-target timeout | 60s |
| `ARQ_SIGNALS_QUERY_TIMEOUT` | Per-query timeout | 10s |
| `ARQ_SIGNALS_LISTEN_ADDR` | API listen address | 127.0.0.1:8081 |
| `ARQ_SIGNALS_DB_PATH` | SQLite database path | /data/arq-signals.db |
| `ARQ_SIGNALS_WRITE_TIMEOUT` | API write timeout | 180s |
| `ARQ_SIGNALS_API_TOKEN` | Bearer token for API auth | auto-generated |

## Compatibility with Arq Analyzer

arq-signals snapshots are designed to be consumed by
[Arq Analyzer](https://elevarq.com/analyzer), a separate product that
performs automated analysis, scoring, and LLM-powered reporting. The
snapshot format (`arq-snapshot.v1`) is the stable contract between
collector and analyzer.

**arq-signals is fully usable on its own.** You do not need Arq Analyzer
to collect, export, or inspect your PostgreSQL diagnostics. Many teams
use arq-signals purely for data collection, feeding the snapshots into
their own tooling or analysis workflows.

## Project status

arq-signals is in **early release** (v0.1.0). The collection engine,
safety model, and snapshot format are stable and tested (94 automated
tests). The snapshot format is versioned and will maintain backward
compatibility.

**Roadmap:**

- Expand collector coverage (replication stats, locks, WAL)
- Kubernetes deployment examples
- Community-contributed collectors

## Contributing

We welcome contributions. See [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines and [GOVERNANCE.md](GOVERNANCE.md) for project governance.

**In scope:** new collectors, bug fixes, performance, documentation.
**Out of scope:** analysis, scoring, AI (those belong in a downstream
analyzer).

## Further reading

- [Security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Runtime safety model](docs/runtime-safety-model.md)
- [Adoption guide](docs/adoption-guide.md)
- [FAQ](docs/faq.md)

## License

BSD-3-Clause. See [LICENSE](LICENSE).

Free to use, modify, and distribute for any purpose, including
commercial use.
