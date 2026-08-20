# Elevarq Signals

A lightweight, **read-only PostgreSQL diagnostic and telemetry collector**. Signals
connects to an existing PostgreSQL database that *you* supply, collects statistics
using an approved read-only SQL set, and stores portable snapshots on a local
volume. Signals initiates no telemetry and no automatic upload; snapshots stay on
the workload until you retrieve them. There is no AI in Signals.

> Signals is the **collection layer** only. It is not the Elevarq Analyzer and
> performs no AI analysis. Snapshots are collected locally and downloaded via the
> Signals HTTP API for analysis elsewhere.

## What this template deploys

| Resource | Purpose |
|----------|---------|
| `workload` (stateful, single replica) | The Signals collector daemon |
| `volumeset` | Persistent `/data` store for the SQLite snapshot database |
| `secret` (dictionary) | PostgreSQL password + API bearer token |
| `secret` (opaque, required) | PEM CA bundle for `verify-ca`/`verify-full` TLS |
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
| `postgres.caCert` | PEM CA bundle to verify the server cert (TLS is `verify-full` in prod). Managed PostgreSQL: use the provider bundle, e.g. [Amazon RDS](https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem) |
| `api.token` | API bearer token, `openssl rand -base64 32` (≥ 32 chars) |

## Common optional inputs

| Value | Default | Description |
|-------|---------|-------------|
| `postgres.port` | `5432` | |
| `postgres.database` | `postgres` | Database to connect to |
| `postgres.sslMode` | `verify-full` | `verify-full` \| `verify-ca` (weaker modes are rejected by Signals in prod) |
| `collection.pollInterval` | `5m` | Time between collection cycles |
| `collection.retentionDays` | `30` | Snapshot retention window |
| `collection.highSensitivityCollectorsEnabled` | `true` | Set `false` to exclude collectors that can capture live query text (`pg_stat_activity`) and object definitions (views/triggers/function bodies) from snapshots |
| `logLevel` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `metrics.enabled` | `false` | Prometheus `/metrics` (behind the API token) |
| `export.onCollect` | `true` | Drop one ZIP per database into `export.dest` after each cycle (see Snapshot exports) |
| `export.dest` | `/data/exports` | Directory for dropped ZIPs — must be on the volume (`/data...`) |
| `resources.cpu` / `resources.memory` | `100m` / `128Mi` | |
| `storage.capacity` | `10` | Volume size in GiB (≥ 10; ≥ 200 for `high-throughput-ssd`) |
| `storage.performanceClass` | `general-purpose-ssd` | `general-purpose-ssd` \| `high-throughput-ssd` |
| `firewall.external.outboundAllowCIDR` | `[0.0.0.0/0]` | Egress to PostgreSQL. Hostname firewall rules can't reach 5432, so this is a CIDR; narrow it to your DB if it has a stable IP |

## Verifying it works

- **Running:** `GET http://<workload>.<gvc>.cpln.local:8081/health` → `200 {"status":"ok",...}` (no auth).
- **Collection / connectivity / failures:** `GET /status` with header
  `Authorization: Bearer <api.token>` → target list, last collection time,
  snapshot count, and per-target last-cycle outcome (a failed PostgreSQL
  connection shows here).
- **Trigger a collection now:** `POST /collect/now` (bearer auth).
- **Download a snapshot:** `GET /export` (bearer auth) → a `signals-snapshot.v1` ZIP.
- **Metrics:** `GET /metrics` (bearer auth) when `metrics.enabled=true`.

## Snapshot exports

Snapshots are always available two ways:

- **Pull (always on):** `GET /export` (bearer) streams a `signals-snapshot.v1` ZIP.
- **Drop to disk (`export.onCollect`, on by default):** after every collection
  cycle Signals writes one ZIP per database into `export.dest` (default
  `/data/exports` on the persistent volume), named
  `<instance>-t<targetID>-<UTC>.zip`. The directory is created automatically.

Retrieving dropped files: list/copy them with
`cpln workload exec <name> --gvc <gvc> -- ls -la /data/exports`, or run a
sidecar/second workload that mounts the same volume and syncs the directory to
object storage.

> **Files accumulate.** Dropped ZIPs are timestamped and never overwritten, and
> Signals does not prune them (only the SQLite store honours
> `collection.retentionDays`). Sync them out and delete, or size
> `storage.capacity` for your cadence and retention. Set `export.onCollect=false`
> to disable dropping and rely on `GET /export` only.

## Security model

- Read-only enforced in three layers: static SQL linting, session
  `default_transaction_read_only=on`, and per-query `BEGIN READ ONLY`.
- Runs as non-root (UID/GID 10001) under Control Plane's restricted container
  capabilities, no privilege escalation, TLS to PostgreSQL. Signals initiates no
  outbound telemetry (the workload's egress firewall still governs connectivity).
- Credentials are held in Control Plane secrets and revealed only to the
  Signals identity via the bundled policy — never baked into the image or
  committed in plaintext.

Source & docs: https://github.com/Elevarq/signals
