# Jira target

The target adapter (`internal/infrastructure/jira`) creates and updates issues
in the destination tracker through its REST v3 API.

## Authentication

Configurable: `basic` (an `Authorization: Basic base64(email:token)` header) or
`bearer` (`Authorization: Bearer <token>`). The header is built once and stored
as a redacting value.

## Operations

- `Create(spec)` — creates an issue with project, issue type, summary, an ADF
  description, and configured mirrored fields. It does not set labels.
- `Update(key, spec)` — updates summary and description.
- `CurrentUserAccountID()` — resolves the authenticating user's account ID via
  `/myself` when automatic assignment is enabled.
- `SetStatus(key, status)` — lists transitions and executes the one whose target
  status name matches (case-insensitive). Because the destination workflow is
  open, any status is reachable; an unmatched status is a clear error.

By default, create and update payloads set `assignee.accountId` to the account
returned by `/myself`, so manual reassignments are corrected on the next run.
This can be disabled with `defaults.assign_all_to_me`.
When a project has `epic_key`, create and update payloads also set
`parent.key`, keeping mirrored issues under that epic after manual re-parenting.
Configured `components` are also sent by name on every create and update. This
is required when the destination issue type has no default component.
Configured `priority` is sent by name on every create and update. When it is
empty, the priority field is omitted.

Descriptions are Atlassian Document Format. See [body-formatting.md](body-formatting.md).
The base URL must be HTTPS; redirects are disabled, requests time out, response
bodies are bounded, and the authorization header never appears in logs or errors.
Rate-limited requests retry with bounded `Retry-After` or exponential delays.
Non-success responses include a bounded prefix of the destination API's error
body so validation failures are actionable without exposing request headers.
