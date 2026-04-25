# Release verification

Every Arq Signals release publishes a multi-arch container image to
`ghcr.io/elevarq/arq-signals` along with:

- A **SLSA build provenance** attestation (`mode=max`).
- An **SPDX SBOM** attached to the image as an OCI attestation, **and**
  available as a downloadable `sbom.spdx.json` release asset.
- A **cosign keyless signature** bound to the GitHub Actions workflow
  identity that produced the build.
- **Multi-arch manifest list** (`linux/amd64`, `linux/arm64`).

This document is the operator-facing checklist for verifying a release
before deploying it. Replace `<VERSION>` (e.g. `0.3.0`) below.

## Required tooling

```bash
# Image inspection
docker --version          # any modern Docker with buildx
docker buildx imagetools  # bundled with Docker

# Supply-chain verification
cosign version            # >= 2.0
trivy --version           # >= 0.50

# Optional but useful for SBOM inspection
syft version              # >= 0.90
```

## Quick verify (single command)

```bash
cosign verify ghcr.io/elevarq/arq-signals:<VERSION> \
  --certificate-identity-regexp='github.com/Elevarq/Arq-Signals/.github/workflows/release.yml@' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com'
```

A successful verification prints the signing certificate and exits 0.
Any other output means the image is **not** trustworthy — do not deploy.

## Full checklist

### 1. Workflow run logs

Locate the GitHub Actions run for the version you're verifying:

```
https://github.com/Elevarq/Arq-Signals/actions/workflows/release.yml
```

Confirm:

- Run was triggered by a tag matching `v<VERSION>`.
- All jobs succeeded: `validate`, `test`, `lint`, `security-scan`,
  `publish`, `build-binaries`, `release`.
- The `publish` job's "Build and push" step shows a **manifest list
  digest** in the form `sha256:…`. Note this digest — you'll use it
  below.

### 2. Multi-arch manifest list

```bash
docker buildx imagetools inspect ghcr.io/elevarq/arq-signals:<VERSION>
```

Expect to see two platform entries:

```
Manifests:
  …  Platform:    linux/amd64
  …  Platform:    linux/arm64
```

If only one platform appears, the image is single-arch — reject and
investigate.

### 3. Cosign signature

```bash
cosign verify ghcr.io/elevarq/arq-signals:<VERSION> \
  --certificate-identity-regexp='github.com/Elevarq/Arq-Signals/.github/workflows/release.yml@' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
  | jq .
```

Confirm:

- The certificate identity ends with `release.yml@refs/tags/v<VERSION>`.
- The OIDC issuer is `https://token.actions.githubusercontent.com`.
- The bundle's `Subject` matches the image digest from step 1.

### 4. SLSA build provenance

The release workflow uses `docker/build-push-action` with
`provenance: mode=max`, which publishes per-platform SLSA build
provenance as **BuildKit-native OCI attestation manifests** attached
to the image index. These are inspected via buildx, not cosign:

```bash
docker buildx imagetools inspect ghcr.io/elevarq/arq-signals:<VERSION> \
  --format '{{json .Provenance}}'
```

The output is a JSON map keyed by platform (`linux/amd64`,
`linux/arm64`). For each platform, inspect the SLSA predicate:

- The `SLSA` block carries the build statement.
- Materials / source references point at this repository and the
  exact commit being built.
- The builder identity records the GitHub Actions runner.

The attestation manifests themselves are also visible in the
multi-arch listing from step 2 — entries marked
`vnd.docker.reference.type: attestation-manifest`.

> Note: `cosign verify-attestation --type slsaprovenance` does **not**
> retrieve these. That command looks for cosign-signed attestations,
> which are a different OCI shape than the BuildKit-native
> attestations this release publishes. Use the `buildx imagetools`
> command above.

### 5. SBOM

The SBOM is published in two forms:

**5a. BuildKit-native OCI attestation** attached to the image index
(per-platform). Inspect via buildx:

```bash
docker buildx imagetools inspect ghcr.io/elevarq/arq-signals:<VERSION> \
  --format '{{json .SBOM}}'
```

The output is a JSON map keyed by platform. For each platform, the
embedded SPDX document has:

- `SPDXID = SPDXRef-DOCUMENT`
- A `creationInfo.creators` entry naming the SBOM tool (e.g. anchore)
- A non-zero `packages` count covering the Go module set

**5b. Downloadable SPDX file** attached to the GitHub Release as
`sbom.spdx.json`. Convenient when consumers want the SBOM without
pulling the image at all:

```bash
gh release download v<VERSION> --repo Elevarq/Arq-Signals \
  --pattern 'sbom.spdx.json' \
  --pattern 'SHA256SUMS'

# or via curl:
curl -L \
  https://github.com/Elevarq/Arq-Signals/releases/download/v<VERSION>/sbom.spdx.json \
  -o sbom.spdx.json
```

The `SHA256SUMS` file on the same release page covers the Go binary
assets (`arq-signals-*`, `arqctl-*`) — verify those before deploying
binaries to production hosts.

The two SBOMs (5a and 5b) may differ slightly in tool / ordering but
should agree on package identities.

> Note: `cosign download sbom` does **not** retrieve the registry
> SBOM. That command looks for cosign-signed SBOM attachments,
> which are a different OCI shape than the BuildKit-native
> attestations this release publishes. Use the `buildx imagetools`
> command above, or the file artifact from 5b.

### 6. Image security scan (Trivy)

CI runs Trivy and fails the release on `CRITICAL` / `HIGH`. Re-run
locally before deploying as a second-opinion check:

```bash
trivy image \
  --severity CRITICAL,HIGH \
  --ignore-unfixed \
  ghcr.io/elevarq/arq-signals:<VERSION>
```

Expect: `Total: 0 (HIGH: 0, CRITICAL: 0)`. Findings here that the CI
run did not see usually indicate a Trivy DB update — investigate
before shipping.

### 7. OCI labels (sanity check)

```bash
docker buildx imagetools inspect \
  --format '{{json .Manifest}}' \
  ghcr.io/elevarq/arq-signals:<VERSION> \
  | jq '.manifests[0].annotations // empty'
```

Confirm the image carries the standard `org.opencontainers.image.*`
labels: `title`, `description`, `licenses=BSD-3-Clause`, `source`,
`version`, `revision`, `created`.

## Failure modes and what they mean

| Symptom | Likely cause | Action |
|---|---|---|
| `cosign verify` reports "no matching signatures" | Image was not signed by this workflow | Reject. Could be a typo in the image reference, or an attempted impersonation. |
| `imagetools inspect --format '{{json .Provenance}}'` returns an empty object or shows materials that don't reference this repo | Provenance is missing or for a different artifact | Reject — confirm you're inspecting the right tag and that the build used `provenance: mode=max`. |
| `imagetools inspect` shows only `linux/amd64` | Build was single-arch (workflow not yet on this version) | Confirm the release commit predates `chore/release-hardening` — older releases are amd64-only. |
| Trivy reports CRITICAL/HIGH locally that CI didn't flag | Trivy DB update found a new CVE since CI ran | Don't deploy. Open an issue and rebuild from a fresh main. |
| `imagetools inspect --format '{{json .SBOM}}'` returns an empty object or `packages: 0` | `sbom: true` was disabled or buildkit failed | Reject — re-run the release workflow. The downloadable `sbom.spdx.json` on the GitHub release is a separate, independent artifact and can be checked as a cross-reference. |
| `cosign download sbom` or `cosign verify-attestation` returns "no matching attestations" | The release uses BuildKit-native attestations, not cosign-signed ones | Use `docker buildx imagetools inspect` instead — see steps 4 and 5. The cosign signature on the image itself (step 3) still verifies; only the attestation retrieval pathway differs. |

## Out of scope (not signed/attested today)

- The downloadable Go binaries (`dist/arq-signals-linux-amd64`, etc.)
  are not signed individually; the SHA256 sums file (`SHA256SUMS`)
  attached to the release lets you verify integrity but not provenance.
  Sign these separately if you ship binaries to production hosts.
- The Helm chart is published from the `deploy/helm/` directory but is
  not currently cosign-signed. Charts come from the same release commit
  but consumers should pin to the digest, not floating chart versions.

Both items are tracked as supply-chain follow-ups.
