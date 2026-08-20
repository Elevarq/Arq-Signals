# Publishing Elevarq products to the Control Plane Template Catalog

This directory holds the **Control Plane (cpln.io) Template Catalog** package for
Signals, plus the reusable runbook for publishing any free Elevarq container
product to the catalog. It is the Control Plane analogue of `docs/marketplace/`
(which is the AWS Marketplace kit).

The Signals template under `signals/` is a **drop-in copy** of the directory
layout Control Plane expects at `controlplane-com/templates/signals/`. Publishing
= contributing that directory to Control Plane's public templates repo.

---

## How the Control Plane Template Catalog works (verified 2026-08-20)

- The catalog is a set of **Helm charts** in the public GitHub repo
  [`controlplane-com/templates`](https://github.com/controlplane-com/templates).
- On merge to `main`, a GitHub Actions workflow (`publish-charts.yml`) packages
  each changed version and pushes it to `oci://ghcr.io/controlplane-com/templates`.
  Users then install via the Control Plane console, `cpln`, Terraform
  (`cpln_catalog_template`), or Pulumi (`CatalogTemplate`).
- **We do not self-publish.** The publication gate is a **pull request to
  `controlplane-com/templates` that Control Plane reviews and merges.** Everything
  before that PR is ours to prepare and validate; the merge is external.
- Each template resolves resources against an existing GVC (`createsGvc: false`);
  the platform injects the GVC name as `{{ .Values.global.cpln.gvc }}`.

Sources: [templates repo README](https://github.com/controlplane-com/templates),
[workload container reference](https://docs.controlplane.com/reference/workload/containers),
[template catalog docs](https://docs.controlplane.com/template-catalog/install-manage/pulumi).

---

## Required template layout

```
<product>/
├── icon.png                        # square, transparent background
└── versions/
    └── <semver>/                   # folder name MUST equal Chart.yaml version
        ├── Chart.yaml              # required annotations (below)
        ├── README.md               # user-facing install/verify instructions
        ├── values.yaml             # documented defaults, no plaintext secrets
        └── templates/
            ├── _helpers.tpl        # naming + validation; tags via cpln-common
            ├── identity.yaml       # workload identity
            ├── policy.yaml         # reveal permission on the secrets
            ├── secret-*.yaml       # type: dictionary (or opaque for files)
            ├── volumeset.yaml      # only if the workload is stateful
            └── workload-*.yaml     # the workload spec
```

Required `Chart.yaml` annotations:

```yaml
annotations:
  created: "YYYY-MM-DD"      # first publication date
  lastModified: "YYYY-MM-DD" # bump on every change
  category: "observability"  # see category vocabulary below
  createsGvc: false          # true only if the template provisions its own GVC
dependencies:
  - name: cpln-common        # shared tags/labels library
    version: 1.0.0
    repository: "oci://ghcr.io/controlplane-com/templates"
```

Category vocabulary — **examples** observed in the live catalog (not an
exhaustive/authoritative list; confirm against current templates before relying
on one): `observability`, `database`, `security`, `analytics`, `storage`,
`search`, `proxy`, `secrets-management`, `event-streaming`, `app`, `library`.
Signals uses **`observability`**.

> `LISTING.md` and this runbook are **internal** artifacts — the upstream catalog
> has no `LISTING.md` convention and its workflow does not consume one. Keep listing
> copy here for our own reference; hand it to Control Plane only if they request it
> during review. Only `<product>/icon.png` + `versions/<semver>/…` is submitted.

---

## Generic Control Plane steps (same for every product)

1. **Model on the closest existing template.** For a workload that connects to an
   external service with credentials, `debezium-server` is the reference.
2. **Secrets:** never inline. Put sensitive values in a `type: dictionary`
   secret; reference them from the workload as `cpln://secret/<name>.<key>`.
   Mount file-type secrets (certs) from a `type: opaque` secret
   (`encoding: plain`, `payload: |-`) at a path via `cpln://secret/<name>.payload`.
   Any workload that reads a secret needs an **identity** + a **policy** granting
   that identity `reveal` on the secret.
3. **Naming:** derive every resource name from `.Release.Name` in `_helpers.tpl`
   so multiple installs never collide. No hardcoded names in resource files.
4. **Tags:** depend on `cpln-common` and label every resource with
   `include "<product>.tags"` → `cpln-common.tags` (adds the required
   `cpln/marketplace*` labels).
5. **Image:** pin by **immutable digest** (`repo@sha256:…`). Control Plane pulls
   `linux/amd64` for managed locations, so the image index **must** contain an
   amd64 manifest. Public `ghcr.io` images pull without a pull secret.
6. **Validation (all local, no account writes):**
   ```bash
   cd <product>/versions/<semver>
   helm dependency build
   helm lint . -f test-values.yaml        # cpln resources warn on metadata.name — expected
   helm template rel . -f test-values.yaml   # inspect the rendered cpln YAML
   ```
   The `metadata.name` lint warnings are benign: Control Plane resources use a
   top-level `name:`, which Helm's Kubernetes linter doesn't recognize. They
   appear for every catalog template.
7. **Publish (external gate):** open a PR to `controlplane-com/templates` adding
   `<product>/…`. Control Plane reviews and merges; the workflow publishes the
   chart to `oci://ghcr.io/controlplane-com/templates`. **Do not** expect a
   self-service publish path.

## Product-specific steps (fill in per product)

- The container's config surface → the `values.yaml` inputs (host/port/creds/TLS/
  log level/metrics/resources). Expose only what a deployer needs.
- Fail-fast validation in `_helpers.tpl` for required inputs and invalid combos.
- `stateful` + `volumeset` only if the container needs persistence; otherwise
  `standard`.
- `securityOptions.filesystemGroupId` = the image's runtime GID, so mounted
  volumes are writable by the non-root user.
- Probes: use the container's real health endpoint (`httpGet`), not a shell hack.
- A square, transparent `icon.png` (the Elevarq brand mark).

---

## Reusable checklist — free container product → Control Plane Catalog

**Preconditions**
- [ ] GA image published to `ghcr.io/<org>/<product>` (real SemVer, no `-rc`).
- [ ] Multi-arch index includes `linux/amd64`; cosign-signed; SBOM attested.
- [ ] Resolve the immutable index digest (`docker buildx imagetools inspect …`).

**Authoring** (in `deploy/controlplane/<product>/versions/<semver>/`)
- [ ] `Chart.yaml` with `created`/`lastModified`/`category`/`createsGvc:false`
      and the `cpln-common` dependency.
- [ ] `values.yaml` — documented inputs, digest-pinned image, **no plaintext
      secrets**, renders with defaults.
- [ ] `_helpers.tpl` — `.Release.Name` naming, fail-fast validation, `*.tags`.
- [ ] `identity.yaml` + `policy.yaml` (reveal on the secrets).
- [ ] `secret-*.yaml` (`dictionary` for creds; `opaque` for mounted files).
- [ ] `volumeset.yaml` if stateful.
- [ ] `workload-*.yaml` — digest image, ports, env (`cpln://` secret refs),
      probes, `defaultOptions`, `securityOptions`, `firewallConfig`.
- [ ] version `README.md` (install + verify) and a square `icon.png`.

**Local validation**
- [ ] `helm dependency build` pulls `cpln-common`.
- [ ] `helm lint` → 0 failures.
- [ ] `helm template` renders valid cpln YAML for required + optional paths.
- [ ] Negative tests: missing required inputs and invalid combos fail fast.
- [ ] Live `docker run` of the digest confirms: non-root, binds `0.0.0.0:<port>`,
      health endpoint returns 200, graceful SIGTERM.
- [ ] `cosign verify` the image; `cpln image query` from the target org (read-only).

**Publish (human/external gates)**
- [ ] Open PR to `controlplane-com/templates` adding `<product>/…`.
- [ ] Control Plane review + merge → auto-published to the OCI registry.
- [ ] Smoke-install from the catalog into a test GVC; verify the workload is
      ready and the product's health/status endpoints report success.
