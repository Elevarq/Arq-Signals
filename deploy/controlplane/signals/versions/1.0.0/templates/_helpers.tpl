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
Image reference — digest-pinned when a digest is provided (immutable), else the
human-readable tag.
================================================================================
*/}}
{{- define "signals.image" -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | toString) -}}
{{- end -}}
{{- end -}}

{{/*
================================================================================
Validation — fail fast on missing/invalid inputs before any resource renders.
================================================================================
*/}}
{{- define "signals.validateConfig" -}}
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
{{- if and .Values.export.onCollect (not .Values.export.dest) -}}
{{- fail "export.dest is required when export.onCollect is true (use a path on the persistent volume, e.g. /data/exports)" -}}
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
