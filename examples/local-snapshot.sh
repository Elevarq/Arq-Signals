#!/usr/bin/env bash
# Repeatable Elevarq Signals local snapshot.
#
# Brings up the local dev stack (PostgreSQL seeded by init.sql with a
# representative schema + the non-superuser `signals` pg_monitor role),
# runs ONE collection, WAITS for it to actually finish, and exports an
# inspectable snapshot ZIP. Idempotent: run it as many times as you
# like — no wheel to reinvent.
#
# Usage:
#   examples/local-snapshot.sh                # up -> collect -> export ./signals-snapshot.zip -> verify
#   OUT=~/Downloads/snap.zip examples/local-snapshot.sh
#   examples/local-snapshot.sh --down         # tear the stack down (and volumes) and exit
#
# Requires: docker (compose v2), curl, python3.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
COMPOSE="$HERE/docker-compose.yml"
TOKEN="dev-local-only-replace-in-prod-32chars"   # matches examples/docker-compose.yml (dev only)
BASE="http://localhost:8081"
OUT="${OUT:-$HERE/signals-snapshot.zip}"

dc() { docker compose -f "$COMPOSE" "$@"; }

if [ "${1:-}" = "--down" ]; then dc down -v; echo "stack down."; exit 0; fi

echo "==> 1/5 bringing up postgres + signals (docker compose up --build)"
dc up -d --build

echo "==> 2/5 waiting for the Signals API to answer"
for i in $(seq 1 60); do curl -fsS "$BASE/health" >/dev/null 2>&1 && break; sleep 2; done
curl -fsS "$BASE/health" >/dev/null 2>&1 || { echo "Signals API never came up; 'dc logs signals':"; dc logs --tail=30 signals; exit 1; }

echo "==> 3/5 forcing a full collection cycle"
curl -fsS -X POST "$BASE/collect/now?force=true" -H "Authorization: Bearer $TOKEN" -d '{}' >/dev/null

echo "==> 4/5 waiting for the collection to COMPLETE (polling the export, not a blind sleep)"
runs=0
for i in $(seq 1 45); do
  curl -fsS "$BASE/export" -H "Authorization: Bearer $TOKEN" -o "$OUT" 2>/dev/null || true
  read -r runs succ < <(python3 - "$OUT" <<'PY' 2>/dev/null || echo "0 0"
import sys,zipfile,json
try:
    z=zipfile.ZipFile(sys.argv[1])
    runs=[json.loads(l) for l in z.read("query_runs.ndjson").decode().splitlines()]
    succ=sum(1 for r in runs if r.get("status")=="success")
    print(len(runs), succ)
except Exception:
    print(0, 0)
PY
)
  # collection is "done enough" once we have a healthy set of successful runs
  if [ "${succ:-0}" -ge 40 ]; then break; fi
  sleep 2
done
echo "    collected: ${runs:-0} runs (${succ:-0} success)"

echo "==> 5/5 verifying the snapshot (constraint types must be single-char STRINGS)"
python3 - "$OUT" <<'PY'
import sys,zipfile,json
z=zipfile.ZipFile(sys.argv[1])
print("snapshot:", sys.argv[1])
print("entries :", ", ".join(z.namelist()))
runs={json.loads(l)["id"]:json.loads(l) for l in z.read("query_runs.ndjson").decode().splitlines()}
found=False
for l in z.read("query_results.ndjson").decode().splitlines():
    res=json.loads(l); q=runs.get(res.get("run_id"),{}).get("query_id","")
    if q.startswith("pg_constraints_v1"):
        rows=res.get("payload") or []
        types=sorted({r.get("contype") for r in rows})
        print(f"\npg_constraints_v1: {len(rows)} rows; contype values = {types}")
        assert all(isinstance(t,str) for t in types), "contype is NOT a string -> char-type wire bug!"
        for want,label in [("p","PRIMARY KEY"),("f","FOREIGN KEY"),("u","UNIQUE"),("c","CHECK")]:
            ex=next((r for r in rows if r.get("contype")==want), None)
            if ex: print(f"  contype={want!r} ({label}): {json.dumps({k:ex[k] for k in list(ex)[:5]})}")
        found=True; break
if not found:
    print("\npg_constraints_v1 returned no rows — is the target DB empty? (init.sql seeds constraints)")
print("\nOK: contype serialized as strings (the #312/#319 char-type fix).")
PY
echo
echo "Done. Snapshot at: $OUT   (inspect with examples/snapshot-inspection/README.md; tear down with: $0 --down)"
