# Local Safe Role Example

This is the recommended way to run Arq Signals in production: using a
dedicated monitoring role with `pg_monitor` privileges instead of the
PostgreSQL superuser.

## 1. Create the monitoring role

```sql
CREATE ROLE arq_signals LOGIN;
GRANT pg_monitor TO arq_signals;
GRANT CONNECT ON DATABASE your_database TO arq_signals;
```

## 2. Configure

Copy and edit the example config:

```bash
cp signals.yaml.example signals.yaml
# Edit host, port, dbname to match your environment
```

## 3. Run

```bash
# Build
cd /path/to/arq-signals
make build

# Start the collector
ARQ_ALLOW_INSECURE_PG_TLS=true \
ARQ_SIGNALS_API_TOKEN=my-token \
./bin/arq-signals --config examples/local-safe-role/signals.yaml
```

## 4. Collect and export

```bash
# Trigger an immediate collection
curl -X POST http://localhost:8081/collect/now \
  -H "Authorization: Bearer my-token"

# Export a snapshot
curl -o snapshot.zip http://localhost:8081/export \
  -H "Authorization: Bearer my-token"
```

## 5. What to expect

The export `metadata.json` will show:

```json
{
  "schema_version": "arq-snapshot.v1",
  "unsafe_mode": false
}
```

`unsafe_mode: false` confirms the collector ran with a safe,
non-superuser role. No `unsafe_reasons` field will be present.

## Why this matters

Running with a dedicated monitoring role means:
- The safety model passes without override
- No superuser, replication, or bypassrls attributes are detected
- Export metadata is clean
- This is the intended production configuration
