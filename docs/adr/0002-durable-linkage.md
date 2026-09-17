# ADR 0002: Linkage via a local JSON map plus a durable `plane-<id>` label

Status: Accepted

## Context

Synchronization must not create duplicate target issues, including when host-local state is lost.

## Decision

The synchronizer resolves each source item by checking the local map, then searching for its durable `plane-<id>` label, and finally creating a target issue only when neither lookup succeeds. The label is the authoritative fallback. The JSON map is a host-local, gitignored cache.

## Consequences

Runs are idempotent and remain safe after loss of the local map. An unmapped item requires one additional target search. The approach relies on the target's built-in label field.
