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
#   CP-TMPL-R006  required inputs (host/user/database/password/api.token/caCert) fail-fast when empty
#   CP-TMPL-R007  prod TLS: only verify-ca/verify-full accepted
#   CP-TMPL-R008  api.token >= 32 chars AND >= 8 distinct chars (sentinel exempt)
#   CP-TMPL-R009  logLevel / firewall.inboundAllowType / storage.performanceClass enums
#   CP-TMPL-R010  postgres.port range; storage.capacity range; high-throughput-ssd >= 200 GiB
#   CP-TMPL-R011  export.dest required + must be under /data when export.onCollect
#   CP-TMPL-R012  safe defaults: export drop OFF, high-sensitivity OFF; boolean-typed values
#   CP-TMPL-R013  collection.pollInterval positive Go duration; retentionDays >= 1
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/versions/1.0.0"
EXPECTED_DIGEST="sha256:76aeda85f41ee716797535824c62dee72b5524d98267deacf589983b9a6feadc"
fails=0

pass() { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; fails=$((fails + 1)); }
render() { helm template validation "$CHART_DIR" --set global.cpln.gvc=validation-gvc "$@" 2>&1; }

echo "== deps =="
helm dependency build "$CHART_DIR" >/dev/null 2>&1 || { echo "helm dependency build failed"; exit 1; }

echo "== positive (R001..R004, R012 output) =="
if OUT="$(render)"; then
  pass "R001 renders with defaults + dummy gvc"
  for t in 'cpln/marketplace: "true"' 'cpln/marketplace-template:' 'cpln/marketplace-template-version:' 'cpln/marketplace-gvc:'; do
    if echo "$OUT" | grep -q "$t"; then pass "R002 tag present: $t"; else fail "R002 tag missing: $t"; fi
  done
  if echo "$OUT" | grep -qE "image: ghcr.io/elevarq/signals@${EXPECTED_DIGEST}\b"; then
    pass "R004 image is reviewed repository@digest"
  else
    fail "R004 image not digest-pinned to the reviewed digest"
  fi
  # R012 — safe defaults must appear in the rendered output.
  if echo "$OUT" | grep -q 'name: SIGNALS_EXPORT_'; then
    fail "R012 default render emits scheduled export env (should be absent)"
  else
    pass "R012 scheduled export env absent by default"
  fi
  if echo "$OUT" | grep -A1 'SIGNALS_HIGH_SENSITIVITY_COLLECTORS_ENABLED' | grep -q 'value: "false"'; then
    pass "R012 high-sensitivity defaults false"
  else
    fail "R012 high-sensitivity default drift (expected \"false\")"
  fi
  if echo "$OUT" | grep -qE 'protocol: tcp'; then
    pass "R012 egress restricted to a TCP port (outboundAllowPort)"
  else
    fail "R012 egress not port-scoped (outboundAllowPort missing)"
  fi
else
  fail "R001 default render failed:"; echo "$OUT" | tail -3
fi

DESC="$(sed -n 's/^description:[[:space:]]*//p' "$CHART_DIR/Chart.yaml" | head -1)"
WORDS="$(echo "$DESC" | tr ' ' '\n' | grep -cvE '^$|^(—|–|-)$')"
if [ "$WORDS" -le 15 ]; then pass "R003 description is $WORDS words (<=15)"; else fail "R003 description is $WORDS words (>15)"; fi

echo "== negative (each MUST fail to render) =="
neg() { # <rule> <label> <helm args...>
  local rule="$1" label="$2"; shift 2
  if render "$@" >/dev/null 2>&1; then fail "$rule rendered but should have failed: $label"; else pass "$rule rejects $label"; fi
}
neg R005 "changed image.repository"  --set image.repository=evil/signals
neg R005 "changed image.digest"      --set image.digest=sha256:dead
neg R006 "empty postgres.host"       --set-string postgres.host=
neg R006 "empty postgres.database"   --set-string postgres.database=
neg R006 "empty postgres.user"       --set-string postgres.user=
neg R006 "empty postgres.password"   --set-string postgres.password=
neg R006 "empty postgres.caCert"     --set-string postgres.caCert=
neg R006 "empty api.token"           --set-string api.token=
neg R007 "sslMode=require"           --set postgres.sslMode=require
neg R008 "api.token < 32"            --set-string api.token=short
neg R008 "api.token < 8 distinct"    --set-string api.token=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
neg R009 "logLevel=trace"            --set logLevel=trace
neg R009 "inboundAllowType=public"   --set firewall.internal.inboundAllowType=public
neg R009 "bad performanceClass"      --set storage.performanceClass=nvme
neg R010 "port < 1"                  --set postgres.port=0
neg R010 "port > 65535"              --set postgres.port=70000
neg R010 "capacity < 10"             --set storage.capacity=9
neg R010 "capacity > 65536"          --set storage.capacity=65537
neg R010 "high-throughput + 10GiB"   --set storage.performanceClass=high-throughput-ssd
neg R011 "export.dest outside /data" --set export.onCollect=true --set export.dest=/tmp/exports
neg R011 "empty export.dest"         --set export.onCollect=true --set-string export.dest=
neg R012 "string export.onCollect"   --set-string export.onCollect=false
neg R012 "string high-sensitivity"   --set-string collection.highSensitivityCollectorsEnabled=false
neg R012 "string metrics.enabled"    --set-string metrics.enabled=false
neg R013 "invalid pollInterval"      --set-string collection.pollInterval=garbage
neg R013 "zero pollInterval"         --set-string collection.pollInterval=0s
neg R013 "retentionDays < 1"         --set collection.retentionDays=0

echo "== positive opt-ins (must render) =="
if render --set export.onCollect=true >/dev/null 2>&1; then
  pass "R011 export.onCollect=true renders (default dest under /data)"
else
  fail "R011 export.onCollect=true failed to render"
fi
if render --set collection.highSensitivityCollectorsEnabled=true >/dev/null 2>&1; then
  pass "R012 highSensitivityCollectorsEnabled=true renders"
else
  fail "R012 highSensitivityCollectorsEnabled=true failed to render"
fi

echo
if [ "$fails" -eq 0 ]; then echo "ALL PASS"; exit 0; else echo "$fails FAILED"; exit 1; fi
