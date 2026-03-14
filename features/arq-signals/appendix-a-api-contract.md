# Appendix A: API Contract

This appendix defines the HTTP API contract for Arq Signals. Any
conforming implementation must expose these endpoints with the specified
behavior.

## Authentication

All endpoints except `GET /health` require a bearer token in the
`Authorization` header:

```
Authorization: Bearer <token>
```

The token is configured via the `ARQ_SIGNALS_API_TOKEN` environment
variable. If not set, the system generates a random token at startup
and logs a fingerprint (not the token itself).

Invalid or missing tokens shall return HTTP 401 with a JSON body:
```json
{"error": "missing or invalid Authorization header"}
```

The system shall rate-limit invalid token attempts: after 5 failures
from the same IP within 5 minutes, subsequent requests from that IP
shall receive HTTP 429.

Token comparison must be constant-time to prevent timing attacks.

## Endpoints

### GET /health

**Authentication:** None required
**Response:** HTTP 200
```json
{
  "status": "ok",
  "version": "<semver>"
}
```

### GET /status

**Authentication:** Bearer token required
**Response:** HTTP 200
```json
{
  "instance_id": "<string>",
  "version": "<semver>",
  "target_count": <integer>,
  "targets": [
    {
      "id": <integer>,
      "name": "<string>",
      "host": "<string>",
      "port": <integer>,
      "dbname": "<string>",
      "user": "<string>",
      "sslmode": "<string>",
      "enabled": <boolean>,
      "last_collected": "<RFC3339 timestamp or absent>"
    }
  ],
  "snapshot_count": <integer>,
  "query_catalog_count": <integer>,
  "last_collected": "<RFC3339 timestamp or empty string>"
}
```

**Excluded fields:** The status response must NOT include `secret_type`,
`secret_ref`, passwords, credential paths, or any information that
reveals how or where credentials are stored.

### POST /collect/now

**Authentication:** Bearer token required
**Response:** HTTP 202
```json
{
  "status": "collection triggered"
}
```

This triggers an immediate collection cycle (non-blocking). The actual
collection runs asynchronously.

### GET /export

**Authentication:** Bearer token required
**Query parameters:**
- `target_id` (optional, integer) — filter to a single target
- `since` (optional, RFC3339) — include data collected after this time
- `until` (optional, RFC3339) — include data collected before this time

**Response:** HTTP 200 with `Content-Type: application/zip`

The response body is a ZIP archive containing:
- `metadata.json`
- `snapshots.ndjson`
- `query_catalog.json`
- `query_runs.ndjson`
- `query_results.ndjson`

## Export metadata schema

The `metadata.json` file in the export ZIP shall contain:

```json
{
  "schema_version": "arq-snapshot.v1",
  "instance_id": "<string>",
  "collector_version": "<semver>",
  "collector_commit": "<git short hash>",
  "collected_at": "<RFC3339 timestamp>",
  "unsafe_mode": <boolean>
}
```

When `unsafe_mode` is `true`, the metadata shall also include:

```json
{
  "unsafe_reasons": [
    "<description of bypassed check 1>",
    "<description of bypassed check 2>"
  ]
}
```

The `unsafe_reasons` values shall describe the specific role attributes
that were bypassed (e.g. "role has superuser attribute (rolsuper=true)"),
not generic flags.

## General conventions

- All responses use `Content-Type: application/json` except `/export`
  which uses `application/zip`.
- All timestamps are RFC3339 in UTC.
- Each response includes an `X-Request-ID` header for tracing.
- The server shall include recovery middleware that returns HTTP 500
  with a JSON error body if an unhandled error occurs.
