# Linkage and idempotency

planesync uses its local link map as the sole source of truth for source-to-target
linkage. Mirrored target issues carry no linkage label.

## The link map

A local JSON file (default `config/planesync-links.json`, gitignored) stores
each source ID with its target key, the source item's last synchronized
`updated_at`, and the configuration salt used for that write. It is a versioned
document written atomically
(same-directory temp file, fsync, rename) with restrictive permissions.

```json
{
  "version": 1,
  "links": {
    "source-id": {
      "key": "TARGET-123",
      "updated_at": "2026-09-17T10:00:00Z",
      "config_salt": "sha256-hex-value"
    }
  }
}
```

Version 1 files whose link values are legacy key strings remain readable. Their
empty synchronization markers cause one resynchronization, after which the file
is saved in object form.

The map is persisted immediately after every create, before later operations
such as status transitions. A mid-run failure therefore cannot lose a newly
established mapping. The source timestamp and configuration salt are
checkpointed after all writes for the item succeed.

- A missing or empty file loads as an empty map.
- Malformed JSON or an unsupported version is a clear error, not a silent wipe.
- Preserve and back up the map. If it is lost, linkage cannot be reconstructed
  from the destination and the next run recreates the source issues.

## Resolution order

For each source item, planesync compares Plane's `updated_at` with the stored
timestamp and compares a hash of the effective write configuration with the
stored configuration salt. The salt covers every configured field written to
an issue, including sorted status maps and components and the resolved assignee.
If both markers match, the item is reported as `unchanged` and neither its
fields nor its status are written. A source timestamp or relevant configuration
change produces an update and replaces both markers. Destination timestamps are
not used for this decision because Jira-side automation may change them. When
the map has no entry, planesync creates a target issue and immediately records
its key. See
[status-mapping.md](status-mapping.md) for deletes.
