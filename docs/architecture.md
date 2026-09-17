# Architecture

planesync follows a catalog-style hexagonal (ports-and-adapters) layout. The
dependency direction points inward: infrastructure depends on the application,
the application depends on the domain, and the domain depends on nothing but the
standard library.

## Layers

- `cmd/planesync` — the executable and composition root. Its `internal/config`
  and `internal/factory` packages wire concrete adapters to the application's
  ports; no business logic lives here.
- `internal/ui/cli` — argument parsing, flags, non-interactive output, and exit
  codes.
- `internal/application/sync` — the synchronization use case. It defines the
  ports it needs (source, target, links, status resolver) and orchestrates a
  run. It imports only the domain and the standard library.
- `internal/domain` — tracker-independent rules and models: `mirror` (summary,
  label, back-link, plain-text body), `statusmap` (state-to-status resolution),
  `linkmap` (map-then-label-then-create decision), and `configuration` (the
  typed config model).
- `internal/infrastructure` — outbound adapters: `plane` (source client),
  `jira` (target client), `linkstore` (link-map persistence), `config` (loader),
  and `httpbase` (shared base-URL guard).

## Data flow

A run reads work items from the source (Plane), resolves each to a target issue
(Jira) through the link map or the durable label, applies the mirrored summary,
status, and body, and records the link. In full and reconcile modes it also
transitions issues whose source item has disappeared. `--dry-run` plans every
action without performing a write.
