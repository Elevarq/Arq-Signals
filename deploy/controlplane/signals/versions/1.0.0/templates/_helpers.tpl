{{/*
================================================================================
Resource Naming — all names derive from .Release.Name to avoid collisions
across multiple installs in the same GVC.
================================================================================
*/}}
{{- define "signals.name" -}}
{{- printf "%s-signals" .Release.Name -}}
{{- end -}}

{{- define "signals.identity.name" -}}
{{- printf "%s-signals-identity" .Release.Name -}}
{{- end -}}

{{- define "signals.policy.name" -}}
{{- printf "%s-signals-policy" .Release.Name -}}
{{- end -}}

{{- define "signals.credentials.name" -}}
{{- printf "%s-signals-credentials" .Release.Name -}}
{{- end -}}

{{- define "signals.ca.name" -}}
{{- printf "%s-signals-ca" .Release.Name -}}
{{- end -}}

{{- define "signals.volumeset.name" -}}
{{- printf "%s-signals-data" .Release.Name -}}
{{- end -}}

{{/*
================================================================================
Reviewed production image — mandatory and immutable. The renderer only ever
emits repository@digest; there is no mutable-tag fallback. The values are pinned
to the exact reviewed image and enforced in signals.validateConfig.
================================================================================
*/}}
{{- define "signals.expectedRepository" -}}ghcr.io/elevarq/signals{{- end -}}
{{- define "signals.expectedDigest" -}}sha256:76aeda85f41ee716797535824c62dee72b5524d98267deacf589983b9a6feadc{{- end -}}
{{- define "signals.image" -}}
{{- printf "%s@%s" (include "signals.expectedRepository" .) (include "signals.expectedDigest" .) -}}
{{- end -}}

{{/*
================================================================================
Validation — fail fast on missing/invalid inputs before any resource renders.
================================================================================
*/}}
{{- define "signals.validateConfig" -}}
{{- /* The reviewed production image is mandatory and unchangeable. */ -}}
{{- if ne .Values.image.repository (include "signals.expectedRepository" .) -}}
{{- fail (printf "image.repository must remain %s; the reviewed production image is used unchanged" (include "signals.expectedRepository" .)) -}}
{{- end -}}
{{- if ne .Values.image.digest (include "signals.expectedDigest" .) -}}
{{- fail (printf "image.digest must remain %s; mutable tags and unreviewed digests are not allowed" (include "signals.expectedDigest" .)) -}}
{{- end -}}
{{- if not .Values.postgres.host -}}
{{- fail "postgres.host is required (the PostgreSQL hostname reachable from the GVC)" -}}
{{- end -}}
{{- if not .Values.postgres.database -}}
{{- fail "postgres.database is required" -}}
{{- end -}}
{{- if not .Values.postgres.user -}}
{{- fail "postgres.user is required (a read-only role — see README)" -}}
{{- end -}}
{{- if not .Values.postgres.password -}}
{{- fail "postgres.password is required (stored in a Control Plane secret)" -}}
{{- end -}}
{{- if not .Values.api.token -}}
{{- fail "api.token is required — generate with `openssl rand -base64 32` (>= 32 chars)" -}}
{{- end -}}
{{- /* The workload runs with SIGNALS_ENV=prod, which requires a verifying TLS
       mode with a CA cert. Signals rejects require/prefer/disable in prod. */ -}}
{{- $validSsl := list "verify-ca" "verify-full" -}}
{{- if not (has .Values.postgres.sslMode $validSsl) -}}
{{- fail (printf "postgres.sslMode '%s' is not allowed: the workload runs with SIGNALS_ENV=prod, which requires verify-ca or verify-full (require/prefer/disable are rejected by Signals in prod)." .Values.postgres.sslMode) -}}
{{- end -}}
{{- if not .Values.postgres.caCert -}}
{{- fail (printf "postgres.caCert (PEM CA bundle) is required for sslMode '%s'. For managed PostgreSQL use the provider bundle (e.g. Amazon RDS: https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem)." .Values.postgres.sslMode) -}}
{{- end -}}
{{- /* Boolean-typed values must be real booleans: `--set-string x=false` would
       otherwise be a truthy non-empty string and silently defeat a safe default. */ -}}
{{- if not (kindIs "bool" .Values.export.onCollect) -}}
{{- fail "export.onCollect must be a boolean (true or false, not a quoted string)" -}}
{{- end -}}
{{- if not (kindIs "bool" .Values.collection.highSensitivityCollectorsEnabled) -}}
{{- fail "collection.highSensitivityCollectorsEnabled must be a boolean (true or false, not a quoted string)" -}}
{{- end -}}
{{- if not (kindIs "bool" .Values.metrics.enabled) -}}
{{- fail "metrics.enabled must be a boolean (true or false, not a quoted string)" -}}
{{- end -}}
{{- /* Reject weak API tokens. The fail-closed placeholder sentinel is exempt: it
       renders for catalog validation but Signals rejects it at startup. */ -}}
{{- $token := .Values.api.token | toString -}}
{{- $placeholderToken := "00000000000000000000000000000000" -}}
{{- if and (ne $token $placeholderToken) (lt (len (uniq (splitList "" $token))) 8) -}}
{{- fail "api.token must contain at least 8 distinct characters (openssl rand -base64 32)" -}}
{{- end -}}
{{- $poll := .Values.collection.pollInterval | toString -}}
{{- if or (not (regexMatch "^([0-9]+(\\.[0-9]+)?(ns|us|ms|s|m|h))+$" $poll)) (not (regexMatch "[1-9]" $poll)) -}}
{{- fail "collection.pollInterval must be a positive Go duration (e.g. 30s, 5m, 1h)" -}}
{{- end -}}
{{- if lt (int .Values.collection.retentionDays) 1 -}}
{{- fail "collection.retentionDays must be at least 1 (a finite retention window)" -}}
{{- end -}}
{{- if and .Values.export.onCollect (not .Values.export.dest) -}}
{{- fail "export.dest is required when export.onCollect is true (use a path on the persistent volume, e.g. /data/exports)" -}}
{{- end -}}
{{- if and .Values.export.onCollect (not (or (eq (clean .Values.export.dest) "/data") (hasPrefix "/data/" (clean .Values.export.dest)))) -}}
{{- fail "export.dest must resolve to /data or a directory below /data so exports survive restarts" -}}
{{- end -}}
{{- if lt (len (.Values.api.token | toString)) 32 -}}
{{- fail "api.token must be at least 32 characters (openssl rand -base64 32)" -}}
{{- end -}}
{{- $logLevels := list "debug" "info" "warn" "error" -}}
{{- if not (has .Values.logLevel $logLevels) -}}
{{- fail (printf "logLevel '%s' is invalid; use one of: %s" .Values.logLevel (join ", " $logLevels)) -}}
{{- end -}}
{{- $port := int .Values.postgres.port -}}
{{- if or (lt $port 1) (gt $port 65535) -}}
{{- fail "postgres.port must be between 1 and 65535" -}}
{{- end -}}
{{- $classes := list "general-purpose-ssd" "high-throughput-ssd" -}}
{{- if not (has .Values.storage.performanceClass $classes) -}}
{{- fail (printf "storage.performanceClass must be one of: %s" (join ", " $classes)) -}}
{{- end -}}
{{- $capacity := int .Values.storage.capacity -}}
{{- if or (lt $capacity 10) (gt $capacity 65536) -}}
{{- fail "storage.capacity must be between 10 and 65536 GiB" -}}
{{- end -}}
{{- if and (eq .Values.storage.performanceClass "high-throughput-ssd") (lt $capacity 200) -}}
{{- fail "storage.performanceClass high-throughput-ssd requires storage.capacity >= 200 GiB" -}}
{{- end -}}
{{- $inboundTypes := list "none" "same-gvc" "same-org" "workload-list" -}}
{{- if not (has .Values.firewall.internal.inboundAllowType $inboundTypes) -}}
{{- fail (printf "firewall.internal.inboundAllowType must be one of: %s" (join ", " $inboundTypes)) -}}
{{- end -}}
{{- end -}}

{{/*
================================================================================
Labeling — delegate to the shared cpln-common library (adds the required
cpln/marketplace* tags).
================================================================================
*/}}
{{- define "signals.tags" -}}
{{- include "cpln-common.tags" . -}}
{{- end -}}
