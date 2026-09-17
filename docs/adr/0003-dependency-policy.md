# ADR 0003: Dependency policy: explicit approval and append-only audit log

Status: Accepted

## Context

The dependency and trust surface must remain minimal and auditable.

## Decision

No third-party dependency is added without recorded maintainer approval. Each approval is appended to `docs/dependency-approvals.md`, which is an append-only log. Approved dependencies remain narrowly scoped and confined to the layer that needs them. The tool currently uses only the standard library.

## Consequences

The trust surface stays predictable, and dependency decisions have a durable audit trail. Adding a dependency intentionally requires an approval and logging step.
