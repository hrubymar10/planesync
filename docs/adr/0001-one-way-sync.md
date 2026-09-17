# ADR 0001: One-way sync; Plane is the source of truth

Status: Accepted

## Context

An external tracker must mirror the working tracker without ongoing manual maintenance. Allowing both systems to change the same records would require conflict detection and resolution.

## Decision

Synchronization is strictly one-way, from the source to the target. The target is a read-only reflection: target-side edits are overwritten on subsequent runs, and no target data flows back to the source. When a source item is removed, its target issue transitions to a configured terminal status and is never hard-deleted.

## Consequences

The ownership model remains simple, and synchronization automatically corrects target drift. The system does not need bidirectional conflict handling. Target-side edits are not preserved.
