# Status mapping

The destination status is chosen deterministically from a per-project table,
with a group-based fallback so states added later still map sanely.

## Resolution

`internal/domain/statusmap` resolves in two steps:

1. Exact match — the source state name in `status_map` wins.
2. Group fallback — otherwise the source state's group (for example `backlog`,
   `unstarted`, `started`, `completed`, `cancelled`) is looked up in
   `status_group_map`.

If neither matches, the status is left unchanged for that item and the run
reports it as a skipped status (a warning, not a failure). The destination
workflow is open, so any resolved status is set directly.

## Deletes

In `--full` and `--reconcile` runs, a source item that no longer exists has its
mirror transitioned to `defaults.deleted_status` (never hard-deleted) and its
link removed. If no `deleted_status` is configured, the item is reported as a
skipped delete.
