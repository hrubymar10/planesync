# Linkage and idempotency

planesync uses its local link map as the sole source of truth for source-to-target
linkage. Mirrored target issues carry no linkage label.

## The link map

A local JSON file (default `config/planesync-links.json`, gitignored) stores
`source-id -> target-key`. It is a
versioned `{ "version": 1, "links": { ... } }` document written atomically
(same-directory temp file, fsync, rename) with restrictive permissions.
The map is persisted immediately after every create, before later operations
such as status transitions. A mid-run failure therefore cannot lose a newly
established mapping. The end-of-run save remains as a final checkpoint.

- A missing or empty file loads as an empty map.
- Malformed JSON or an unsupported version is a clear error, not a silent wipe.
- Preserve and back up the map. If it is lost, linkage cannot be reconstructed
  from the destination and the next run recreates the source issues.

## Resolution order

For each source item: update the target key found in the link map, or create a
new target issue when the map has no entry. A create immediately records the
new key in the map. See [status-mapping.md](status-mapping.md) for deletes.
