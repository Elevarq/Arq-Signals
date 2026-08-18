# object_privileges_v1 — Collector Specification

Status: ACTIVE

## Purpose

Enumerate the actual object-level access privileges (ACLs) granted on
user objects, so a downstream security-posture analysis (Analyzer #1757)
can flag **over-broad privileges** — grants to `PUBLIC`, or broad
privileges on the `public` schema — which is a common misconfiguration.
`login_roles_v1` / `pg_role_capabilities_v1` describe role *capabilities*;
this collector adds the object *grants* that were previously unobserved
(#363).

## Catalog source

- `pg_class.relacl` — tables, partitioned tables, views, materialized
  views, sequences, foreign tables
- `pg_namespace.nspacl` — schema-level grants (the `public`-schema case)
- `pg_proc.proacl` — function/procedure `EXECUTE` grants
- `aclexplode()` normalises each `aclitem[]` array into one row per grant;
  `pg_get_userbyid()` resolves grantee/grantor OIDs to role names

A `NULL` acl means "owner default, no explicit grants" and yields no rows
— the presence of an explicit row for a broad grantee (`PUBLIC` / a broad
role) is exactly the signal the consumer needs.

## Output columns

| Column | Type | Description |
|---|---|---|
| object_kind | text | `table`, `partitioned_table`, `view`, `materialized_view`, `sequence`, `foreign_table`, `schema`, or `function` |
| schema_name | text | Schema of the object (the schema name itself for `object_kind = schema`) |
| object_name | text | Object name (the schema name for `object_kind = schema`) |
| grantee | text | Role granted the privilege, or the literal `PUBLIC` (grantee OID 0) |
| privilege_type | text | Privilege granted (e.g. `SELECT`, `INSERT`, `USAGE`, `CREATE`, `EXECUTE`) |
| is_grantable | boolean | Whether the grantee may re-grant the privilege (`WITH GRANT OPTION`) |
| grantor | text | Role that granted the privilege |

## Scope filter

- Object relkinds `r`, `p`, `v`, `m`, `S`, `f` with a non-NULL `relacl`;
  all schemas with a non-NULL `nspacl`; all functions with a non-NULL
  `proacl`.
- System schemas excluded: `pg_catalog`, `information_schema`, `pg_toast`,
  and any `pg_temp_%` / `pg_toast_temp_%`.

## Invariants

- Deterministic ordering: `ORDER BY object_kind, schema_name,
  object_name, grantee, privilege_type, is_grantable`.
- Stable output column order.
- Read-only, passes linter (no side-effecting keyword or function).
- `PUBLIC` is rendered as the literal string `PUBLIC`, never a NULL or
  empty grantee, so "granted to PUBLIC" is unambiguous.
- Emits at least the `public`-schema default grants on any database
  (`nspacl` is non-NULL by default on every supported major), so the
  collector is never vacuously empty.

## Failure Conditions

- FC-01: Permission denied reading a catalog (unusual — `pg_class`,
  `pg_namespace`, `pg_proc` are world-readable) → standard collector
  error path.

## Configuration

- Category: security
- Cadence: 6h (Cadence6h)
- Retention: RetentionLong
- Min PG version: 14 (all supported majors; `aclexplode` predates them)
- Requires extension: none
- Semantics: snapshot
- Enabled by default: yes

## Sensitivity

Low. Only catalog privilege *metadata* is read — no object data. Role
names are the same class of data already emitted by `login_roles_v1` /
`pg_role_capabilities_v1` and are visible to every connected role, so no
redaction is applied.

## Analyzer requirements unblocked

- **`broad-or-public-grants` finding (Analyzer #1757)** — flags grants
  to `PUBLIC` and broad privileges on the `public` schema; deferred there
  until this collector lands.
