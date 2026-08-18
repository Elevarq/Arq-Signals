# default_privileges_v1 — Collector Specification

Status: ACTIVE

## Purpose

Enumerate the **default** access privileges (`ALTER DEFAULT PRIVILEGES`
state) that FUTURE objects will inherit, so a downstream security-posture
analysis (Analyzer #1757) can reason about grants that do not yet exist as
concrete object ACLs but will be applied to every object a role creates —
a common vector for silently re-introducing over-broad (`PUBLIC`) grants
(#363). Complements `object_privileges_v1`, which carries the ACLs on
objects that already exist.

## Catalog source

- `pg_default_acl` — `defaclrole` (the role whose future objects are
  affected), `defaclnamespace` (schema scope, 0 = global), `defaclobjtype`
  (object class the default applies to), `defaclacl` (the `aclitem[]`)
- `aclexplode()` normalises `defaclacl` into one row per grant;
  `pg_get_userbyid()` resolves grantee OIDs to role names

`pg_default_acl` is empty until someone runs `ALTER DEFAULT PRIVILEGES`, so
this collector emits rows only when default privileges have been
customised.

## Output columns

| Column | Type | Description |
|---|---|---|
| for_role | text | Role whose newly-created objects inherit the default (`defaclrole`) |
| schema_name | text | Schema the default is scoped to, or empty string for a global (non-schema) default |
| object_kind | text | Object class the default applies to: `table`, `sequence`, `function`, `type`, or `schema` |
| grantee | text | Role granted the default privilege, or the literal `PUBLIC` (grantee OID 0) |
| privilege_type | text | Privilege granted (e.g. `SELECT`, `USAGE`, `EXECUTE`) |
| is_grantable | boolean | Whether the grantee may re-grant the privilege (`WITH GRANT OPTION`) |

## Scope filter

None beyond `pg_default_acl`'s own contents — every default-ACL entry is
reported. `defaclnamespace = 0` (global default) is rendered as an empty
`schema_name`.

## Invariants

- Deterministic ordering: `ORDER BY for_role, schema_name, object_kind,
  grantee, privilege_type, is_grantable`.
- Stable output column order.
- Read-only, passes linter (no side-effecting keyword or function).
- `PUBLIC` is rendered as the literal string `PUBLIC`.
- Emits zero rows on a database with no `ALTER DEFAULT PRIVILEGES` — a
  legitimate empty result, not an error.

## Failure Conditions

- FC-01: Permission denied reading `pg_default_acl` (unusual — it is
  world-readable) → standard collector error path.

## Configuration

- Category: security
- Cadence: 6h (Cadence6h)
- Retention: RetentionLong
- Min PG version: 14 (all supported majors)
- Requires extension: none
- Semantics: snapshot
- Enabled by default: yes

## Sensitivity

Low. Only catalog privilege *metadata* is read — no object data. Role
names are the same class of data already emitted by `login_roles_v1` and
are visible to every connected role, so no redaction is applied.

## Analyzer requirements unblocked

- **`broad-or-public-grants` finding (Analyzer #1757)** — the default-ACL
  half of the over-broad-grant analysis: catches defaults that will grant
  future objects to `PUBLIC` or a broad role.
