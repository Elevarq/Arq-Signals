# Signals — Control Plane Catalog listing copy

Source of truth for the catalog-facing text. Keep in sync with the live listing.

- **Product name:** Elevarq Signals
- **Category:** observability
- **Publisher / vendor:** Elevarq (Scantr LLC d/b/a Elevarq)
- **Price:** Free (open source, BSD-3-Clause)
- **One-line description:** Lightweight, read-only PostgreSQL diagnostic and
  telemetry collector — connects to your existing PostgreSQL, collects
  statistics, and stores portable snapshots locally. No AI.

## Fuller description

Elevarq Signals is a read-only PostgreSQL diagnostic collector. It connects to a
PostgreSQL database you already run, gathers statistics and metadata with an
approved read-only SQL set (102 queries), and stores snapshot history in an
embedded SQLite database on a local volume. Portable `signals-snapshot` ZIP
archives are served on demand via the API; optional scheduled drop-to-disk is off
by default. Signals initiates no outbound telemetry and no automatic upload, and
there is no AI or LLM inside Signals — it is the collection layer, not the Elevarq
Analyzer. Snapshots stay on the workload until you retrieve them.

Read-only is enforced in three independent layers: static SQL linting, a
session-level `default_transaction_read_only=on`, and a per-query
`BEGIN READ ONLY` transaction. Signals refuses to run under a superuser,
replication, or `BYPASSRLS` role — a plain `pg_monitor` grant is all it needs.

Deployed here as a single, stateful Control Plane workload, Signals runs as a
non-root user under Control Plane's restricted container capabilities, connects to
PostgreSQL over TLS, and keeps its snapshot history on a persistent volume. Trigger
collections and download snapshots over a token-protected HTTP API.

## Main capabilities

- Read-only collection from PostgreSQL 14–18 (and TimescaleDB), enforced at
  three layers.
- Portable `signals-snapshot.v1` ZIP exports on demand via the API (optional
  scheduled drop-to-disk).
- Token-protected HTTP API for status, on-demand collection, and export.
- Optional Prometheus operational metrics (never collected database data).
- Single target per deployment. Collectors that can include query text or stored
  SQL bodies are off by default (opt-in).
- No phone-home or automatic export, no AI — snapshots stay local until retrieved.

## Requirements

- An existing PostgreSQL reachable from the GVC.
- A read-only role: `GRANT pg_monitor TO signals;` + `CONNECT` on the database.
- A CA bundle for TLS (`verify-full`) — managed PostgreSQL providers publish one.
- An API bearer token (`openssl rand -base64 32`).

## Security & privacy

- Non-root (UID/GID 10001) under Control Plane's restricted container
  capabilities, no privilege escalation.
- Verified TLS to PostgreSQL (`verify-full`/`verify-ca` with a CA bundle).
- Credentials held in Control Plane secrets, revealed only to the Signals
  identity — never baked into the image.
- Signals initiates no automatic export; snapshots stay on the workload volume
  until you retrieve them.

## PostgreSQL compatibility

PostgreSQL 14–18 and TimescaleDB. Managed services supported (Amazon RDS/Aurora,
Azure Database for PostgreSQL, Google Cloud SQL) — Signals needs only network
reachability and a read-only role.

## Links

- Source & documentation: https://github.com/Elevarq/signals
- Website: https://elevarq.com

## Positioning guardrails

Signals is **not** the full Elevarq Analyzer and performs **no AI analysis**. Do
not imply analysis/inference happens inside Signals. Present it as the read-only
PostgreSQL telemetry/metadata **collection layer** from Elevarq.
