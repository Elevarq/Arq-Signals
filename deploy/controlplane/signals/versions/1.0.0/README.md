# Elevarq Signals

A lightweight, **read-only PostgreSQL diagnostic and telemetry collector**. Signals
connects to an existing PostgreSQL database that *you* supply, collects statistics
using an approved read-only SQL set, and stores portable snapshots on a local
volume. No data ever leaves the workload — there is no AI, no analytics upload,
and no phone-home.

> Signals is the **collection layer** only. It is not the Elevarq Analyzer and
> performs no AI analysis. Snapshots are collected locally and downloaded via the
> Signals HTTP API for analysis elsewhere.

## What this template deploys

| Resource | Purpose |
|----------|---------|
| `workload` (stateful, single replica) | The Signals collector daemon |
| `volumeset` | Persistent `/data` store for the SQLite snapshot database |
| `secret` (dictionary) | PostgreSQL password + API bearer token |
| `secret` (opaque, optional) | PEM CA bundle for `verify-ca`/`verify-full` TLS |
| `identity` + `policy` | Least-privilege `reveal` access to the secrets above |

The image is pinned by immutable digest
(`ghcr.io/elevarq/signals@sha256:76aeda85…`, the multi-arch `1.3.0` index —
`linux/amd64` + `linux/arm64`, cosign-signed with an attached SPDX SBOM).

## Prerequisites

- An existing, reachable PostgreSQL instance (Signals bundles no database).
- A **read-only** PostgreSQL role. The minimum:

  ```sql
  CREATE ROLE signals LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  GRANT pg_monitor TO signals;              -- read-all-stats / read-all-settings
  GRANT CONNECT ON DATABASE <dbname> TO signals;
  ```

  Do **not** grant superuser/replication — Signals fails closed if the role has
  them. `pg_monitor` is sufficient for the default collectors.

## Required inputs

| Value | Description |
|-------|-------------|
| `postgres.host` | PostgreSQL hostname/IP reachable from the GVC |
| `postgres.user` | The read-only role above |
| `postgres.password` | Password for that role (stored in a Control Plane secret) |
| `api.token` | API bearer token, `openssl rand -base64 32` (≥ 32 chars) |

## Common optional inputs

| Value | Default | Description |
|-------|---------|-------------|
| `postgres.port` | `5432` | |
| `postgres.database` | `postgres` | Database to connect to |
| `postgres.sslMode` | `require` | `require` \| `verify-ca` \| `verify-full` (`disable`/`prefer` are rejected in prod) |
| `postgres.caCert` | — | PEM CA bundle, **required** for `verify-ca`/`verify-full` |
| `collection.pollInterval` | `5m` | Time between collection cycles |
| `collection.retentionDays` | `30` | Snapshot retention window |
| `logLevel` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `metrics.enabled` | `false` | Prometheus `/metrics` (behind the API token) |
| `resources.cpu` / `resources.memory` | `100m` / `128Mi` | |
| `storage.capacity` | `10` | Volume size in GiB (Control Plane minimum is 10) |

## Verifying it works

- **Running:** `GET http://<workload>.<gvc>.cpln.local:8081/health` → `200 {"status":"ok",...}` (no auth).
- **Collection / connectivity / failures:** `GET /status` with header
  `Authorization: Bearer <api.token>` → target list, last collection time,
  snapshot count, and per-target last-cycle outcome (a failed PostgreSQL
  connection shows here).
- **Trigger a collection now:** `POST /collect/now` (bearer auth).
- **Download a snapshot:** `GET /export` (bearer auth) → a `signals-snapshot.v1` ZIP.
- **Metrics:** `GET /metrics` (bearer auth) when `metrics.enabled=true`.

## Security model

- Read-only enforced in three layers: static SQL linting, session
  `default_transaction_read_only=on`, and per-query `BEGIN READ ONLY`.
- Runs as non-root (UID/GID 10001), all capabilities dropped, no privilege
  escalation, TLS to PostgreSQL, and no outbound telemetry.
- Credentials are held in Control Plane secrets and revealed only to the
  Signals identity via the bundled policy — never baked into the image or
  committed in plaintext.

Source & docs: https://github.com/Elevarq/signals
