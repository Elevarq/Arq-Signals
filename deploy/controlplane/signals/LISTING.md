# Signals — Control Plane Catalog listing copy

Source of truth for the catalog-facing text. Keep in sync with the live listing.

- **Product name:** Elevarq Signals
- **Category:** observability
- **Publisher / vendor:** Elevarq (Scantr LLC d/b/a Elevarq)
- **Price:** Free (open source, BSD-3-Clause)
- **One-line description:** Lightweight, read-only PostgreSQL diagnostic and
  telemetry collector — connects to your existing PostgreSQL, collects
  statistics, and stores portable snapshots. No data egress, no AI.

## Fuller description

Elevarq Signals is a read-only PostgreSQL diagnostic collector. It connects to a
PostgreSQL database you already run, gathers statistics and metadata with an
approved read-only SQL set (102 queries), and writes portable `signals-snapshot`
ZIP archives to a local volume. Everything stays inside the workload: there is no
outbound telemetry, no analytics upload, and no AI or LLM inside Signals — it is
the collection layer, not the Elevarq Analyzer.

Read-only is enforced in three independent layers: static SQL linting, a
session-level `default_transaction_read_only=on`, and a per-query
`BEGIN READ ONLY` transaction. Signals refuses to run under a superuser,
replication, or `BYPASSRLS` role — a plain `pg_monitor` grant is all it needs.

Deployed here as a single, stateful Control Plane workload, Signals runs as a
non-root user with all capabilities dropped, connects to PostgreSQL over TLS, and
keeps its snapshot history on a persistent volume. Trigger collections and
download snapshots over a token-protected HTTP API.

## Main capabilities

- Read-only collection from any PostgreSQL 12+ (and TimescaleDB), enforced at
  three layers.
- Portable `signals-snapshot.v1` ZIP exports, on a schedule or on demand.
- Token-protected HTTP API for status, on-demand collection, and export.
- Optional Prometheus operational metrics (never collected database data).
- Multi-target support; per-target sensitivity profiles.
- No data egress, no phone-home, no AI — safe for regulated/air-gapped estates.

## Requirements

- An existing PostgreSQL reachable from the GVC.
- A read-only role: `GRANT pg_monitor TO signals;` + `CONNECT` on the database.
- An API bearer token (`openssl rand -base64 32`).

## Security & privacy

- Non-root (UID/GID 10001), all caps dropped, no privilege escalation.
- TLS to PostgreSQL (`require`/`verify-ca`/`verify-full`); CA bundle supported.
- Credentials held in Control Plane secrets, revealed only to the Signals
  identity — never baked into the image.
- No outbound data; snapshots stay on the workload volume until you fetch them.

## PostgreSQL compatibility

PostgreSQL 12–17 and TimescaleDB. Managed services supported (Amazon RDS/Aurora,
Azure Database for PostgreSQL, Google Cloud SQL) — Signals needs only network
reachability and a read-only role.

## Links

- Source & documentation: https://github.com/Elevarq/signals
- Website: https://elevarq.com

## Positioning guardrails

Signals is **not** the full Elevarq Analyzer and performs **no AI analysis**. Do
not imply analysis/inference happens inside Signals. Present it as the read-only
PostgreSQL telemetry/metadata **collection layer** from Elevarq.
