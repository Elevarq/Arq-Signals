# Arq Signals

Arq Signals is a read-only PostgreSQL diagnostic collector. It runs on
your infrastructure, collects statistics from your databases, and
packages them as portable snapshots. No data leaves your machine. No AI.
No cloud. Just structured evidence from the views PostgreSQL already
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
> **No AI inside** — Arq Signals is a pure data collector. No language
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

This starts Arq Signals alongside PostgreSQL 16 with a pre-configured
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

## Why Arq Signals exists

Every PostgreSQL instance exposes diagnostic data through built-in
statistics views. But collecting this data consistently, safely, and in
a format you can actually use takes tooling that most teams end up
building themselves.

Arq Signals handles the collection part so you don't have to. It
connects with a read-only role, runs approved SQL queries on a schedule,
and writes structured results to local storage. When you need the data
elsewhere, it packages everything as a portable ZIP snapshot.

The project is open source because we think data collection should be
transparent. You can read every SQL query Arq Signals will run. You can
audit the binary. You own the output.

## What Arq Signals does

- Connects to one or more PostgreSQL instances (14+)
- Runs 29 read-only diagnostic collectors covering:
  - Server configuration and identity
  - Session activity and connection pressure
  - Table, index, and I/O statistics
  - Query intelligence (via pg_stat_statements)
  - Transaction wraparound risk
  - Vacuum and autovacuum health
  - Replication status and slot risk (graceful when absent)
  - Checkpoint and background writer pressure
  - Storage growth and temp I/O pressure
- Stores results locally in SQLite as structured NDJSON
- Schedules collection with configurable cadences (5m to 7d per query)
- Packages snapshots as portable ZIP archives
- Exposes a local HTTP API for triggering collection and exports
- Provides a CLI (`arqctl`) for operations

## Specification & Test-Driven Development (STDD)

Arq Signals is developed using STDD — a methodology where the
specification and tests define the system, and code is a replaceable
artifact that must satisfy both.

The repository contains:

- **Formal specification** — 56 numbered requirements covering
  collection, safety, configuration, API, persistence, and diagnostics
  ([specification.md](features/arq-signals/specification.md))
- **Acceptance tests** — 43 test cases derived directly from the
  specification
  ([acceptance-tests.md](features/arq-signals/acceptance-tests.md))
- **Traceability matrix** — every requirement mapped to executable
  tests with evidence classification (behavioral, structural, or
  integration)
  ([traceability.md](features/arq-signals/traceability.md))
- **Language-neutral contracts** — API and configuration schemas
  defined as appendices, independent of the Go implementation
  ([Appendix A](features/arq-signals/appendix-a-api-contract.md),
  [Appendix B](features/arq-signals/appendix-b-configuration-schema.md))

This approach matters for a tool that connects to production databases.
Every safety guarantee — read-only enforcement, role validation,
credential handling — is formally specified, tested, and traceable.
You can verify the claims without reading the implementation.

## Why DBAs trust Arq Signals

- All PostgreSQL queries execute inside `READ ONLY` transactions,
  enforced at three independent layers
- Role safety validation blocks superuser, replication, and bypassrls
  roles before any query runs
- Defensive session timeouts (`statement_timeout`, `lock_timeout`,
  `idle_in_transaction_session_timeout`) prevent runaway queries
- The collector never performs write operations on PostgreSQL — this is
  enforced by static SQL linting, session configuration, and
  transaction access mode
- Credentials are never stored in snapshots, export metadata, API
  responses, or log output
- If an unsafe role override is used, it is explicitly recorded in
  export metadata with the specific bypassed checks
- The entire safety model is formally specified and covered by 135
  automated tests

For the full safety model, see
[docs/runtime-safety-model.md](docs/runtime-safety-model.md).

## Examples

| Example | Description |
|---------|-------------|
| [Local safe role](examples/local-safe-role/) | Recommended production setup with `arq_signals` monitoring role |
| [Local superuser override](examples/local-superuser-override/) | Dev/test setup with postgres superuser (unsafe override) |
| [Docker](examples/docker/) | Container build, run, and export workflow |
| [Docker Compose](examples/docker-compose.yml) | Quick start with PostgreSQL 16 |
| [Helm](examples/helm/) | Kubernetes deployment with the starter Helm chart |
| [Snapshot inspection](examples/snapshot-inspection/) | How to inspect and understand export output |
| [Snapshot example](examples/snapshot-example/) | Static reference snapshot for offline review |

## Supported PostgreSQL versions

Arq Signals supports PostgreSQL 14 and later. Smoke-tested against
PostgreSQL 14, 15, 16, 17, and 18. Version-specific collectors
(e.g. `checkpointer_stats_v1` on PG 17+) are automatically included
or excluded based on the detected server version. Collectors requiring
optional extensions (e.g. `pg_stat_statements`) are silently skipped
when the extension is not installed.

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

### Recommended PostgreSQL role

Arq Signals is designed to run using a dedicated monitoring role, not
the PostgreSQL superuser. For production use, create a role such as
`arq_signals` and grant the `pg_monitor` predefined role:

```sql
CREATE ROLE arq_signals LOGIN;
GRANT pg_monitor TO arq_signals;
GRANT CONNECT ON DATABASE your_database TO arq_signals;

-- Optional: enable query-level statistics
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

The default `postgres` role is a superuser and will be rejected by the
safety model unless the operator explicitly enables unsafe override
mode (`ARQ_SIGNALS_ALLOW_UNSAFE_ROLE=true`). This behavior is
intentional — it prevents accidental execution with elevated
privileges in production.

## Using Arq Signals

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

Arq Signals produces snapshots in the `arq-snapshot.v1` format:

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
inspect exactly what Arq Signals collects without running it.

## Collected signals

Arq Signals includes 29 read-only collectors organized into four
packs:

- **Baseline** (9) — core PostgreSQL statistics: server config,
  sessions, databases, tables, indexes, I/O, query stats
- **Wraparound risk** (3) — transaction ID age at database and
  relation level, freeze blockers
- **Diagnostic Pack 1** (9) — server identity, extensions, bgwriter,
  long transactions, blocking locks, login roles, connection
  pressure, planner staleness, pgss reset tracking
- **Server Survival Pack** (8) — replication slot risk, replica lag,
  checkpointer stats, vacuum health, idle-in-transaction offenders,
  database sizes, largest relations, temp I/O pressure

Collectors requiring unavailable extensions or unsupported PostgreSQL
versions are silently skipped. Replication collectors return empty
results on standalone instances.

See [docs/collectors.md](docs/collectors.md) for the full inventory
with query IDs, PostgreSQL sources, and cadences. Every query is
visible in [`internal/pgqueries/`](internal/pgqueries/).

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

Before collecting from any target, Arq Signals validates the connected
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

- Arq Signals makes **no outbound network connections** except to your
  PostgreSQL targets
- No telemetry, no analytics, no phone-home
- The HTTP API binds to loopback by default (`127.0.0.1:8081`)

### Container hardening

When deployed via Docker, Arq Signals runs as a non-root user
(UID 10001) on a minimal Alpine 3.21 base. The image contains
BusyBox (used by the `wget`-based healthcheck and `tini` init) and
no Bash, sh or other full shell beyond BusyBox's `ash` applet.
For deployments that require a shell-free runtime, build against
a distroless base — the binary is statically linked and CGO-free
so it runs without glibc.

## Configuration reference

Arq Signals reads configuration from (in order):
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

## Architecture and scope

Arq Signals is the open-source collection layer of the Arq platform.
It is a complete, standalone tool — not a crippled free tier.

```
┌─────────────────┐
│   Arq Signals   │  Collects diagnostic signals from PostgreSQL.
│  (open source)  │  Produces portable snapshots. This repository.
└────────┬────────┘
         │ snapshot (ZIP / NDJSON)
         ▼
┌─────────────────┐
│       Arq       │  Analyzes signals. Scores health. Generates
│    (private)    │  findings and recommendations.
└────────┬────────┘
         │ findings
         ▼
┌─────────────────┐
│ Arq Workbench   │  Presents results to engineers.
│    (private)    │  Interactive UI for DBA workflows.
└─────────────────┘
```

The snapshot format (`arq-snapshot.v1`) is the stable contract between
layers. Each layer is independently deployable and separately
maintained.

**Arq Signals is fully usable on its own.** You do not need Arq or
Arq Workbench to collect, export, or inspect your PostgreSQL
diagnostics. Many teams use Arq Signals purely for data collection,
feeding the snapshots into their own scripts, dashboards, or analysis
workflows.

### What stays out of Arq Signals — by design

The boundary between Signals and the rest of the platform is
intentional, not accidental:

| Capability | Where it lives | Why not in Signals |
|-----------|---------------|-------------------|
| Database analysis | Arq | Interpretation is a separate concern from evidence collection |
| Health scoring | Arq | Scoring requires domain judgment that evolves independently |
| AI / LLM | Arq | Language models are not needed for safe data collection |
| Recommendations | Arq | Remediation advice requires analysis context |
| Cloud services | None | No component phones home or uploads data |
| Telemetry | None | No usage tracking exists anywhere in the platform |

This separation keeps the collector small, auditable, and safe to run
in restricted environments where third-party analysis tools may not be
permitted.

## Project status

Arq Signals v0.2.0 — the collection engine, safety model, and snapshot
format are stable and tested (135 automated tests, 56 STDD
requirements). Smoke-tested against PostgreSQL 14, 15, 16, 17, and 18.

**Roadmap:**

- Kubernetes deployment examples
- Community-contributed collectors
- Additional storage and replication diagnostics

## Development methodology

This project follows
[STDD — Specification & Test-Driven Development](https://github.com/fheikens/stdd).
Specifications and tests define correct behavior. Implementation is
written to satisfy those rules. The development policy is defined in
[CLAUDE.md](CLAUDE.md).

## Contributing

We welcome contributions. See [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines and [GOVERNANCE.md](GOVERNANCE.md) for project governance.

**In scope:** new collectors, bug fixes, performance, documentation.
**Out of scope:** analysis, scoring, AI (those belong in a downstream
analyzer).

## Project resources

- [Collector inventory](docs/collectors.md) — all 29 collectors with sources and cadences
- [Runtime safety model](docs/runtime-safety-model.md) — read-only enforcement details
- [Adoption guide](docs/adoption-guide.md) — production deployment guidance
- [FAQ](docs/faq.md) — common questions
- [Changelog](CHANGELOG.md) — release history
- [Security policy](SECURITY.md) — vulnerability reporting
- [Citation](CITATION.cff) — how to cite this project

## License

BSD-3-Clause. See [LICENSE](LICENSE).

Free to use, modify, and distribute for any purpose, including
commercial use.
