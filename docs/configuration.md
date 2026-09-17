# Configuration

planesync is driven entirely by a JSONC config file (comments and trailing
commas allowed) plus two secrets taken from the environment.

## Location

Default: `config/planesync.jsonc` (override with `--config <path>`). The live
file is gitignored; a committed `config/planesync.example.jsonc` shows the shape
with placeholder values only.

## Shape

```jsonc
{
  "jira":  { "base_url": "...", "cloud_id": "...", "email": "...", "auth_type": "basic", "token": "${JIRA_TOKEN}" },
  "plane": { "base_url": "...", "workspace": "...", "app_base_url": "...", "token": "${PLANE_TOKEN}" },
  "defaults": { "since": "7d", "body_format": "rich", "deleted_status": "Done" },
  "projects": [
    { "plane_project": "SRC", "jira_project": "DST", "jira_issue_type": "Task",
      "title_prefix": "[team]", "status_map": { }, "status_group_map": { } }
  ]
}
```

## Secrets and precedence

Tokens are read as `${ENV_VAR}` interpolations or literals, and resolved per
project in this order: project-level override, then the endpoint default, then
the canonical `JIRA_TOKEN` / `PLANE_TOKEN` environment variables. A missing
token after resolution is a clear error. Tokens use a redacting type, so they
never appear in logs, errors, or serialized output.

## Validation

Required: `jira.base_url`, `jira.cloud_id`, `plane.base_url`, `plane.workspace`,
`defaults.since`, `defaults.body_format` (`rich` or `text`), and at least one
project with its `plane_project`, `jira_project`, and `jira_issue_type`. When
`jira.auth_type` is `basic` (the default) an email is required; when `bearer`
the email must be empty.
