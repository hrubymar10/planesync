# Usage

planesync is a non-interactive, on-demand CLI. It performs a one-way mirror from
the source tracker to the destination; the destination is a read-only
reflection.

Run `bin/planesync`; it builds a cached binary for the current platform on first
use. Set `PLANESYNC_FORCE_BUILD=1` to force a rebuild.

## Command

```
planesync sync [--full | --reconcile] [--dry-run] [--since Nd|Nh|RFC3339] [--config <path>]
```

## Flags

- `--full` — read the entire source set (initial backfill) and reconcile removed
  items.
- `--reconcile` — read the entire set and reconcile removed items without limiting
  to a window. Mutually exclusive with `--full`.
- `--dry-run` — plan every create, update, status change, and delete, and print
  them, without performing a single write.
- `--since` — incremental window; `Nd` (days), `Nh` (hours), or an RFC3339
  timestamp. Default `7d`. Ignored by full and reconcile modes.
- `--config` — path to the config file (default `config/planesync.jsonc`).

## Modes

- Incremental (default) — mirrors items changed within the window.
- Full — mirrors everything and reconciles deletes.
- Reconcile — reconciles deletes over the full set.

## Exit codes

`0` success, `1` a runtime or configuration error, `2` a usage error.

## Example

```
export JIRA_TOKEN=... PLANE_TOKEN=...
planesync sync --full --dry-run      # preview a full backfill
planesync sync                       # incremental, last 7 days
```

Each configured project is processed in turn and a per-project report of counts
(and, in dry-run, the planned actions) is printed.
Use `--limit N` to process at most the first N source items, sorted by ID.
The default `0` is unlimited. Limited runs disable delete reconciliation because
unseen items may still exist outside the partial view.
