# planesync

> **Note:** `CLAUDE.md` in this repo is a symlink to this file (`AGENTS.md`).

planesync is a deterministic Go command that mirrors a Plane tracker into Jira.

## Documentation discipline

Before changing code, read the docs that touch it: [README.md](README.md), this
file, and everything under `docs/`. When your change makes any of those drift
from reality — flags, env vars, config keys, statuses, behaviour descriptions —
update the docs **in the same commit**. Don't ship code changes and "fix the
docs later".

If while reading docs (or code) you spot something weird, wrong, or inconsistent
that isn't part of your current task, **ask the maintainer** before fixing it.
The maintainer decides scope — don't silently widen the diff.

## Architecture

The repository follows a hexagonal layout:

- `cmd/planesync` is the executable and composition root.
- `internal/ui` contains inbound user-interface adapters.
- `internal/application` coordinates use cases.
- `internal/domain` contains tracker-independent rules and models.
- `internal/infrastructure` contains outbound adapters.

Keep domain and application code independent of concrete infrastructure. Keep composition in `cmd/planesync/internal`.

## Working rules

- Do not put organization or team names, tracker issue identifiers, or other private identifiers in committed files, commit messages, or branch names.
- Do not add, remove, or update dependencies without explicit human approval.
- Keep the tool deterministic and non-interactive.
- The room leader reviews and commits changes; the human pushes them.

## Commit messages

Use long-form commit messages for any non-trivial change; title-only commits are
not acceptable when the diff changes behaviour, adds tests, or updates docs.

- Conventional-commit subject (`type(scope): summary`), roughly 50–72 characters.
- Blank line after the subject.
- Body wrapped at roughly 72 characters explaining the **why**: failure mode,
  design choice, tradeoff, or scope boundary that justified the change.
- Commit as the repository identity; do not add AI attribution or `Co-Authored-By` trailers.
