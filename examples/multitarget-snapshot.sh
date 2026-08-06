#!/usr/bin/env bash
# Multi-target Elevarq Signals local validation (#347).
#
# Proves one Signals install collects from SEVERAL databases using a
# config RENDERED FROM THE HELM CHART — so it exercises the actual
# #347 chart artifact, not a hand-written config.
#
# What it does, in order:
#   1. `helm template` the chart with one `target` (pg-primary) + one
#      `targets:` list entry (pg-analytics), extract the rendered
#      signals.yaml, and write it where the compose stack mounts it.
#   2. Bring up two PostgreSQL instances + Signals (docker compose).
#   3. Force one collection cycle and wait for it to finish.
#   4. Assert GET /status reports BOTH targets, each with a
#      last_collected timestamp — i.e. the multi-target config the chart
#      produced actually drove collection against both databases.
#
# Usage:
#   examples/multitarget-snapshot.sh            # render -> up -> collect -> assert
#   examples/multitarget-snapshot.sh --down     # tear the stack down (and volumes)
#
# Requires: helm, docker (compose v2), curl, python3.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
CHART="$HERE/../deploy/helm/signals"
COMPOSE="$HERE/docker-compose.multitarget.yml"
RENDERED="$HERE/.rendered-multitarget-signals.yaml"
TOKEN="dev-local-only-replace-in-prod-32chars"
BASE="http://localhost:8081"

dc() { docker compose -f "$COMPOSE" "$@"; }

if [ "${1:-}" = "--down" ]; then dc down -v; rm -f "$RENDERED"; echo "stack down."; exit 0; fi

command -v helm >/dev/null || { echo "helm not on PATH"; exit 1; }

echo "==> 1/5 rendering the multi-target config from the Helm chart (#347 artifact)"
helm template signals "$CHART" \
  --set target.host=pg-primary --set target.user=signals --set target.dbname=postgres \
  --set target.passwordSecretName=pg-primary-pw \
  --set 'targets[0].name=analytics' --set 'targets[0].host=pg-analytics' \
  --set 'targets[0].user=signals' --set 'targets[0].dbname=postgres' \
  --set 'targets[0].passwordEnv=PG_PASSWORD_ANALYTICS' \
  --show-only templates/configmap.yaml \
  | python3 -c '
import sys
lines = sys.stdin.read().splitlines()
out, capturing, indent = [], False, None
for ln in lines:
    if ln.strip() == "signals.yaml: |":
        capturing = True
        indent = len(ln) - len(ln.lstrip()) + 2  # content is one level deeper
        continue
    if capturing:
        if ln.strip() == "" :
            out.append("")
            continue
        if len(ln) - len(ln.lstrip()) < indent:
            break
        out.append(ln[indent:])
print("\n".join(out))
' > "$RENDERED"

echo "    rendered targets:"; grep -E '^\s+- name:|^\s+host:' "$RENDERED" | sed 's/^/      /'
if ! grep -q "name: default" "$RENDERED" || ! grep -q "name: analytics" "$RENDERED"; then
  echo "ERROR: rendered config is missing one of the targets:"; cat "$RENDERED"; exit 1
fi

echo "==> 2/5 bringing up pg-primary + pg-analytics + signals"
dc up -d --build

echo "==> 3/5 waiting for the Signals API"
for _ in $(seq 1 60); do curl -fsS "$BASE/health" >/dev/null 2>&1 && break; sleep 2; done
curl -fsS "$BASE/health" >/dev/null 2>&1 || { echo "Signals API never came up:"; dc logs --tail=40 signals; exit 1; }

echo "==> 4/5 forcing a collection and waiting for both targets to report"
curl -fsS -X POST "$BASE/collect/now?force=true" -H "Authorization: Bearer $TOKEN" -d '{}' >/dev/null
ok=0
for _ in $(seq 1 45); do
  status="$(curl -fsS "$BASE/status" -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo '{}')"
  if printf '%s' "$status" | python3 -c '
import sys, json
try: s = json.load(sys.stdin)
except Exception: sys.exit(1)
tg = {t.get("name"): t for t in s.get("targets", [])}
need = {"default", "analytics"}
if not need.issubset(tg): sys.exit(1)
# require both to have actually collected
sys.exit(0 if all(tg[n].get("last_collected") for n in need) else 1)
'; then ok=1; break; fi
  sleep 2
done

echo "==> 5/5 result"
status="$(curl -fsS "$BASE/status" -H "Authorization: Bearer $TOKEN")"
printf '%s' "$status" | python3 -c '
import sys, json
s = json.load(sys.stdin)
print("  instance : " + str(s.get("instance_id", "?")))
print("  targets  : " + str(s.get("target_count")))
for t in s.get("targets", []):
    print("    - %-10s %s:%s/%s  last_collected=%s" % (
        t["name"], t["host"], t["port"], t["dbname"], t.get("last_collected", "-")))
'
if [ "$ok" != "1" ]; then
  echo "FAIL: both targets did not report a completed collection in time."; dc logs --tail=40 signals; exit 1
fi
echo
echo "PASS: one Signals install collected from BOTH databases via the chart-rendered targets: config."
echo "Tear down with: $0 --down"
