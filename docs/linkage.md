# Linkage and idempotency

planesync must never create a duplicate target issue, even if its local state is
lost. Two mechanisms guarantee this.

## The durable label

Every mirrored issue is stamped with the label `plane-<source-id>`. Labels are a
built-in, searchable field, so the source id is always recoverable from the
target itself. This label is the authoritative fallback.

## The link map

A local JSON file (default `config/planesync-links.json`, gitignored) caches
`source-id -> target-key` so a normal run avoids a search per item. It is a
versioned `{ "version": 1, "links": { ... } }` document written atomically
(same-directory temp file, fsync, rename) with restrictive permissions.

- A missing or empty file loads as an empty map (a fresh host is fine).
- Malformed JSON or an unsupported version is a clear error, not a silent wipe —
  the label fallback still prevents duplicates.

## Resolution order

For each source item: look up the link map, then search for the `plane-<id>`
label, then create. A create records the new key in the map; a label recovery
records it too. See [status-mapping.md](status-mapping.md) for deletes.
