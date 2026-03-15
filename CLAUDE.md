# AI Development Policy — STDD

This repository follows **STDD — Specification & Test-Driven Development**.

Canonical methodology: https://github.com/fheikens/stdd

Claude Code must follow the workflow defined in this file.

The specification is the source of truth.
Code is disposable. Tests verify behavior.

## STDD Workflow

All development follows this sequence:

1. Specification
2. Acceptance rules
3. Tests derived from the specification
4. Implementation

Implementation must never start before a specification exists.

## Specification Requirements

Every feature specification must define:

- Inputs
- Outputs
- Invariants
- Failure conditions
- Non-functional requirements

Examples of non-functional requirements:

- Performance limits
- Compatibility constraints
- Safety guarantees
- Security requirements

Specifications live in `features/arq-signals/` and follow the
templates in `stdd/templates/`.

## Validation Requirements

All implementations must demonstrate traceability:

```
specification → tests → implementation
```

If code and specification diverge, **the specification is
authoritative**. Fix the code, not the spec — unless the spec itself
is wrong, in which case update the spec first, then the tests, then
the code.

Traceability is tracked in `features/arq-signals/traceability.md`.

## Project Safety Principles

For Arq Signals specifically:

- No write operations on PostgreSQL
- No superuser privileges required
- No hidden telemetry or external network calls
- Safe to run in production environments
- Credentials never stored, exported, or logged

Safety guarantees must never be weakened by any change. If a change
affects the safety model, the relevant specification and tests must be
updated first.

## Guardrail — Specification Before Code

If a request asks for code but no specification exists, Claude must
first propose a specification.

Claude must NOT immediately generate implementation code.

Instead, Claude must respond with:

1. Proposed specification (inputs, outputs, invariants, failure
   conditions)
2. Derived rules and acceptance criteria
3. Proposed tests

Only after the specification is confirmed may implementation code be
generated.

This applies to new features, new collectors, behavioral changes, and
safety model modifications. It does not apply to trivial fixes (typos,
formatting) or documentation-only changes.

## Repository Structure

```
features/arq-signals/
  specification.md          # Product requirements
  acceptance-tests.md       # Test cases derived from spec
  traceability.md           # Requirement → test mapping
  appendix-a-api-contract.md
  appendix-b-configuration-schema.md

stdd/templates/
  feature-spec-template.md  # Template for new feature specs
  test-spec-template.md     # Template for derived test cases
```
