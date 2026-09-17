# Jira target

The target adapter (`internal/infrastructure/jira`) creates and updates issues
in the destination tracker through its REST v3 API.

## Authentication

Configurable: `basic` (an `Authorization: Basic base64(email:token)` header) or
`bearer` (`Authorization: Bearer <token>`). The header is built once and stored
as a redacting value.

## Operations

- `FindByLabel(label)` — enhanced search (`/search/jql`) for `labels = "<label>"`,
  returning the first matching issue key.
- `Create(spec)` — creates an issue with project, issue type, summary, an ADF
  description, and labels.
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

Descriptions are Atlassian Document Format. See [body-formatting.md](body-formatting.md).
The base URL must be HTTPS; redirects are disabled, requests time out, response
bodies are bounded, and the authorization header never appears in logs or errors.
