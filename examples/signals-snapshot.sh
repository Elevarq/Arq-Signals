#!/usr/bin/env bash
# Point Elevarq Signals at an EXISTING PostgreSQL and get a snapshot ZIP.
#
#   examples/signals-snapshot.sh <host> <port> <dbname> [user] [out.zip]
#
# Example (a local Postgres.app instance):
#   examples/signals-snapshot.sh 127.0.0.1 54318 frs signals ~/Downloads/frs.zip
#
# The role must be NON-superuser (Signals refuses superuser) — ideally
# pg_monitor. Auth: trust (no password) works out of the box; for a
# password set PG_PASSWORD in the environment. Requires: a `signals`
# binary on PATH or $SIGNALS_BIN, plus curl + python3.
set -euo pipefail

HOST="${1:?usage: signals-snapshot.sh <host> <port> <dbname> [user] [out.zip]}"
PORT="${2:?port required}"
DB="${3:?dbname required}"
USER_ROLE="${4:-signals}"
OUT="${5:-./signals-snapshot.zip}"

SIG="${SIGNALS_BIN:-$(command -v signals || echo /tmp/signals-demo/signals-darwin-arm64)}"
[ -x "$SIG" ] || { echo "no signals binary (set SIGNALS_BIN or put 'signals' on PATH)"; exit 1; }

TMP="$(mktemp -d)"; trap 'kill "${SIGPID:-0}" 2>/dev/null || true; rm -rf "$TMP"' EXIT
APIPORT=$(( 18000 + RANDOM % 2000 ))
TOKEN="local-snapshot-token-000000000000000000"

cat > "$TMP/signals.yaml" <<YAML
env: dev
signals: {poll_interval: 1h, retention_days: 7, log_level: warn}
targets:
  - {name: target, host: ${HOST}, port: ${PORT}, dbname: ${DB}, user: ${USER_ROLE}, enabled: true, password_env: PG_PASSWORD, sslmode: disable}
database: {path: ${TMP}/signals.db, wal: true}
api: {listen_addr: "127.0.0.1:${APIPORT}"}
YAML

echo "==> starting Signals against ${USER_ROLE}@${HOST}:${PORT}/${DB} (api :${APIPORT})"
PG_PASSWORD="${PG_PASSWORD:-trust-auth-ignored}" SIGNALS_API_TOKEN="$TOKEN" SIGNALS_ALLOW_INSECURE_PG_TLS=true \
  "$SIG" -config "$TMP/signals.yaml" > "$TMP/signals.log" 2>&1 &
SIGPID=$!
for i in $(seq 1 30); do curl -fsS "http://127.0.0.1:${APIPORT}/health" >/dev/null 2>&1 && break; sleep 1; done
curl -fsS "http://127.0.0.1:${APIPORT}/health" >/dev/null 2>&1 || { echo "Signals did not start:"; tail -5 "$TMP/signals.log"; exit 1; }

echo "==> forcing a collection and waiting for it to complete"
curl -fsS -X POST "http://127.0.0.1:${APIPORT}/collect/now?force=true" -H "Authorization: Bearer $TOKEN" -d '{}' >/dev/null
succ=0
for i in $(seq 1 45); do
  curl -fsS "http://127.0.0.1:${APIPORT}/export" -H "Authorization: Bearer $TOKEN" -o "$OUT" 2>/dev/null || true
  succ=$(python3 - "$OUT" <<'PY' 2>/dev/null || echo 0
import sys,zipfile,json
try:
    z=zipfile.ZipFile(sys.argv[1])
    print(sum(1 for l in z.read("query_runs.ndjson").decode().splitlines() if json.loads(l).get("status")=="success"))
except Exception: print(0)
PY
)
  [ "${succ:-0}" -ge 40 ] && break
  sleep 2
done

# fail loudly if the collection did not actually produce data (don't hand back a false-clean empty ZIP)
if [ "${succ:-0}" -eq 0 ]; then
  echo "COLLECTION PRODUCED NO SUCCESSFUL COLLECTORS — check the role/credentials/target. Signals log:"; tail -8 "$TMP/signals.log"; exit 2
fi

echo "==> wrote $OUT (${succ} successful collectors)"
python3 - "$OUT" <<'PY'
import sys,zipfile,json
z=zipfile.ZipFile(sys.argv[1])
runs={json.loads(l)["id"]:json.loads(l) for l in z.read("query_runs.ndjson").decode().splitlines()}
for l in z.read("query_results.ndjson").decode().splitlines():
    res=json.loads(l); q=runs.get(res.get("run_id"),{}).get("query_id","")
    if q.startswith("pg_constraints_v1"):
        rows=res.get("payload") or []
        print(f"pg_constraints_v1: {len(rows)} rows; contype values = {sorted({r.get('contype') for r in rows})}")
        fk=next((r for r in rows if r.get('contype')=='f'), None)
        if fk: print("  example FK:", fk.get("condef"))
        break
PY
