# Helm Example

A starter Helm chart is provided at
[`deploy/helm/arq-signals/`](../../deploy/helm/arq-signals/).

## Install

```bash
helm install arq-signals deploy/helm/arq-signals/ \
  --set target.host=db.example.com \
  --set target.user=arq_signals \
  --set target.dbname=postgres \
  --set target.passwordSecretName=arq-pg-password
```

## Minimal required values

| Value | Description |
|-------|-------------|
| `target.host` | PostgreSQL hostname |
| `target.user` | PostgreSQL monitoring role |
| `target.dbname` | Database to monitor |
| `target.passwordSecretName` | K8s Secret containing the DB password |

## Custom values file

Create a `my-values.yaml`:

```yaml
target:
  host: db.prod.internal
  user: arq_signals
  dbname: myapp
  sslmode: verify-full
  passwordSecretName: arq-db-credentials

collector:
  pollInterval: 5m
  retentionDays: 14

env: prod
```

Install with:

```bash
helm install arq-signals deploy/helm/arq-signals/ \
  -f my-values.yaml
```

## What the chart provides

- **Deployment** with health/readiness probes on `/health`
- **Service** (ClusterIP) on port 8081
- **ConfigMap** with generated `signals.yaml`
- **PVC** for persistent SQLite storage
- Non-root security context (UID 10001)

## Current status

This is a **starter scaffold** suitable for evaluation and simple
deployments. For production Kubernetes use, you may want to add:
- Ingress or NetworkPolicy
- Pod disruption budget
- Custom resource limits
- Monitoring/alerting integration

## DB credentials

Create a Kubernetes Secret:

```bash
kubectl create secret generic arq-db-credentials \
  --from-literal=password='your-pg-password'
```

The chart injects this as the `PG_PASSWORD` environment variable via
`ARQ_SIGNALS_TARGET_PASSWORD_ENV`.
