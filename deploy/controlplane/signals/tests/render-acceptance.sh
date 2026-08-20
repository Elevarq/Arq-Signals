#!/usr/bin/env bash
# Acceptance tests for the Signals Control Plane Catalog template.
#
# Traceability (spec rule -> case). These mirror the upstream
# controlplane-com/templates validate-charts.yml gate plus this template's own
# fail-fast contract (deploy/controlplane/signals/versions/1.0.0/templates/_helpers.tpl):
#
#   CP-TMPL-R001  chart renders with DEFAULT values + a dummy GVC (upstream CI parity)
#   CP-TMPL-R002  rendered output carries the required cpln/marketplace* tags
#   CP-TMPL-R003  Chart.yaml description is <= 15 words
#   CP-TMPL-R004  workload image is the exact reviewed repository@digest (no mutable tag)
#   CP-TMPL-R005  image.repository / image.digest cannot be changed
#   CP-TMPL-R006  required inputs (host/user/password/api.token/caCert) fail-fast when empty
#   CP-TMPL-R007  prod TLS: only verify-ca/verify-full accepted
#   CP-TMPL-R008  api.token must be >= 32 chars
#   CP-TMPL-R009  logLevel / firewall.inboundAllowType / storage.performanceClass enums
#   CP-TMPL-R010  postgres.port range; storage.capacity range; high-throughput-ssd >= 200 GiB
#   CP-TMPL-R011  export.dest must be under /data
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/versions/1.0.0"
GVC="--set global.cpln.gvc=validation-gvc"
EXPECTED_DIGEST="sha256:76aeda85f41ee716797535824c62dee72b5524d98267deacf589983b9a6feadc"
fails=0

pass() { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; fails=$((fails + 1)); }

# shellcheck disable=SC2086  # $GVC is intentionally word-split into helm flags
render() { helm template validation "$CHART_DIR" $GVC "$@" 2>&1; }

echo "== deps =="
helm dependency build "$CHART_DIR" >/dev/null 2>&1 || { echo "helm dependency build failed"; exit 1; }

echo "== positive (CP-TMPL-R001..R004) =="
if OUT="$(render)"; then
  pass "R001 renders with defaults + dummy gvc"
  for t in 'cpln/marketplace: "true"' 'cpln/marketplace-template:' 'cpln/marketplace-template-version:' 'cpln/marketplace-gvc:'; do
    echo "$OUT" | grep -q "$t" && pass "R002 tag present: $t" || fail "R002 tag missing: $t"
  done
  echo "$OUT" | grep -qE "image: ghcr.io/elevarq/signals@${EXPECTED_DIGEST}\b" \
    && pass "R004 image is reviewed repository@digest" || fail "R004 image not digest-pinned to the reviewed digest"
else
  fail "R001 default render failed:"; echo "$OUT" | tail -3
fi

DESC="$(sed -n 's/^description:[[:space:]]*//p' "$CHART_DIR/Chart.yaml" | head -1)"
WORDS="$(echo "$DESC" | tr ' ' '\n' | grep -cvE '^$|^(—|–|-)$')"
[ "$WORDS" -le 15 ] && pass "R003 description is $WORDS words (<=15)" || fail "R003 description is $WORDS words (>15)"

echo "== negative (each MUST fail to render) =="
neg() { # <rule> <label> <helm args...>
  local rule="$1" label="$2"; shift 2
  if render "$@" >/dev/null 2>&1; then fail "$rule rendered but should have failed: $label"; else pass "$rule rejects $label"; fi
}
neg R005 "changed image.repository" --set image.repository=evil/signals
neg R005 "changed image.digest"     --set image.digest=sha256:dead
neg R006 "empty postgres.host"      --set-string postgres.host=
neg R006 "empty postgres.password"  --set-string postgres.password=
neg R006 "empty postgres.caCert"    --set-string postgres.caCert=
neg R006 "empty api.token"          --set-string api.token=
neg R007 "sslMode=require"          --set postgres.sslMode=require
neg R008 "api.token < 32"           --set-string api.token=short
neg R009 "logLevel=trace"           --set logLevel=trace
neg R009 "inboundAllowType=public"  --set firewall.internal.inboundAllowType=public
neg R009 "bad performanceClass"     --set storage.performanceClass=nvme
neg R010 "port out of range"        --set postgres.port=70000
neg R010 "capacity < 10"            --set storage.capacity=9
neg R010 "high-throughput + 10GiB"  --set storage.performanceClass=high-throughput-ssd
neg R011 "export.dest outside /data" --set export.dest=/tmp/exports

echo
if [ "$fails" -eq 0 ]; then echo "ALL PASS"; exit 0; else echo "$fails FAILED"; exit 1; fi
