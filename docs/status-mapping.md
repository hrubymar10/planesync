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

`resolution_map` independently maps an exact source state name to a destination
resolution. This allows source states such as Done and Cancelled to share the
destination status `Done` while using resolutions `Done` and `Declined`.
Resolution lookup has no group fallback: one source state such as Cancelled
always maps to one fixed resolution.
Resolution is best-effort: when a target workflow rejects the resolution field
because it is absent from the transition screen, planesync retries the same
transition without resolution so the status still advances. Other transition
errors remain failures.

## Deletes

In `--full` and `--reconcile` runs, a source item that no longer exists has its
mirror transitioned to `defaults.deleted_status` (never hard-deleted) and its
link removed. If no `deleted_status` is configured, the item is reported as a
skipped delete.
When configured, `defaults.deleted_resolution` is applied with the terminal
status; the expected delete resolution is `Declined`.
