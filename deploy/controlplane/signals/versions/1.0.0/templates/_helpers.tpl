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
{{- $validSsl := list "disable" "prefer" "require" "verify-ca" "verify-full" -}}
{{- if not (has .Values.postgres.sslMode $validSsl) -}}
{{- fail (printf "postgres.sslMode '%s' is invalid. Must be one of: %s" .Values.postgres.sslMode (join ", " $validSsl)) -}}
{{- end -}}
{{- if or (eq .Values.postgres.sslMode "disable") (eq .Values.postgres.sslMode "prefer") -}}
{{- fail "postgres.sslMode 'disable'/'prefer' is not allowed: the workload runs with SIGNALS_ENV=prod, which requires TLS. Use require, verify-ca or verify-full." -}}
{{- end -}}
{{- if and (or (eq .Values.postgres.sslMode "verify-ca") (eq .Values.postgres.sslMode "verify-full")) (not .Values.postgres.caCert) -}}
{{- fail (printf "postgres.caCert (PEM CA bundle) is required when postgres.sslMode is '%s'" .Values.postgres.sslMode) -}}
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
