# Plane source

The source adapter (`internal/infrastructure/plane`) reads work items and states
from the source tracker's public v1 API. It never writes.

## Operations

- `States` — lists the project's workflow states (id, name, group).
- `List` — lists every work item in the project (used by full and reconcile
  runs and for delete detection).
- `ChangedSince(t)` — items updated at or after `t` (used by incremental runs).

Each item exposes its id, title, HTML body, resolved state name and group, and
update time. State name and group are joined from the states list.

## Transport

Requests use the `X-API-Key` header, target the current `/work-items/` and
`/states/` endpoints, and follow cursor pagination (`next_cursor` /
`next_page_results` / `results`) with loop protection. The base URL must be
HTTPS; redirects are disabled, requests time out, and the response body is
bounded. The token never appears in URLs, logs, or errors.
