# Usage

planesync is a non-interactive, on-demand CLI. It performs a one-way mirror from
the source tracker to the destination; the destination is a read-only
reflection.

Run `bin/planesync`; it builds a cached binary for the current platform on first
use. Set `PLANESYNC_FORCE_BUILD=1` to force a rebuild.

## Command

```
planesync sync [--full | --reconcile] [--dry-run] [--since Nd|Nh|RFC3339] [--config <path>] [IDENTIFIER]
```

With an identifier, for example `planesync sync SRC-123`, planesync reads the
full source project and mirrors only the item whose Plane identifier exactly
matches. Identifier mode does not reconcile deletes and is mutually exclusive
with `--full` and `--reconcile`.

## Flags

- `--full` — read the entire source set (initial backfill) and reconcile removed
  items.
- `--reconcile` — read the entire set and reconcile removed items without limiting
  to a window. Mutually exclusive with `--full`.
- `--dry-run` — plan every create, update, status change, and delete, and print
  them, without performing a single write.
- `--since` — incremental window; `Nd` (days), `Nh` (hours), or an RFC3339
  timestamp. Default `1h`. Ignored by full, reconcile, and identifier modes.
- `--config` — path to the config file (default `config/planesync.jsonc`).

## Modes

- Incremental (default) — mirrors items changed within the window.
- Identifier — mirrors one exact, case-sensitive Plane identifier from the full
  source set without reconciling deletes.
- Full — mirrors everything and reconciles deletes.
- Reconcile — reconciles deletes over the full set.

## Exit codes

`0` success, `1` a runtime or configuration error, `2` a usage error.

## Example

```
export JIRA_TOKEN=... PLANE_TOKEN=...
planesync sync --full --dry-run      # preview a full backfill
planesync sync                       # incremental, last 1 hour
planesync sync SRC-123               # mirror one exact Plane identifier
```

Each configured project is processed in turn. Every processed item streams one
line immediately with its source reference, destination key, and outcome. The
counts summary prints after the project's final item:

```
SRC-16 -> CORE-4277: updated
SRC-17 -> CORE-4280: created
SRC-18 -> CORE-4281: unchanged
created=1 updated=1 unchanged=1 status-set=2 deleted=0 skipped=0
```

Dry-run output uses `(new)` as the destination key for pending creates:

```
SRC-17 -> (new): created
created=1 updated=0 unchanged=0 status-set=1 deleted=0 skipped=0
```

`unchanged` means the item's Plane `updated_at` and current write-configuration
salt match the link map. No Jira field update or status transition is sent for
that item.

Plane and Jira requests that receive HTTP 429 retry up to five attempts. The
delay honors `Retry-After` seconds or HTTP dates; without that header, retries
use exponential backoff. Each wait is capped at 60 seconds and stops promptly
when the run is canceled. Real runs also pause for the configured
`defaults.throttle_ms` between items to reduce rate-limit pressure proactively;
dry runs and the final item do not pause.

Use `--limit N` to process at most the first N source items, sorted by ID.
The default `0` is unlimited. Limited runs disable delete reconciliation because
unseen items may still exist outside the partial view.

## Single-instance lock

Before a real run, planesync creates `planesync.lock` beside the configured
link map (in the directory containing the config file). A lock less than 10
minutes old stops the run with a message showing its path and creation time, so
two processes cannot write the destination and local map concurrently. A lock
at least 10 minutes old is treated as stale and reclaimed; it can also be
deleted manually after confirming that no other instance is active. The lock is
removed when the run finishes. `--dry-run` performs no writes and does not
create a lock.
