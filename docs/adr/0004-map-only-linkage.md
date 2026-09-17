# ADR 0004: Map-only linkage

Status: Accepted

## Context

Mirrored destination issues must remain label-free. The durable label selected
by ADR 0002 conflicts with that product requirement.

## Decision

`config/planesync-links.json` is the sole source of truth for source-to-target
linkage. The synchronizer updates the mapped target when an entry exists and
creates a new target when it does not. It does not stamp or search for a
destination-side linkage label.

## Consequences

The link map must be preserved and backed up. If it is lost, linkage cannot be
reconstructed from the destination, so a subsequent run recreates the source
issues. Immediate persistence after each create limits map loss during a failed
run, but cannot protect against loss of the map file itself.
