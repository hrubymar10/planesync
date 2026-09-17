# Testing

All checks are offline and require no real credentials or network beyond
localhost.

## Make targets

- `make test` — unit tests.
- `make test-race` — unit tests under the race detector.
- `make test-e2e` — builds a fresh ignored platform binary and passes it to the
  end-to-end fixture.
- `make test-full` — `vet`, race tests, and the e2e fixture (the full gate).
- `make build` — host binary at `planesync-<os>-<arch>` (gitignored).

`bin/planesync` is a tracked build-and-run wrapper. On first use it builds a
cached root-level `planesync-<os>-<machine>` executable; later runs reuse it.
Set `PLANESYNC_FORCE_BUILD=1` to rebuild before running.

## End-to-end fixture

`tests/e2e.sh` starts a localhost mock of the source and destination APIs
(`tests/mockserver/`), points a temporary config at it, runs the explicit
binary supplied by `make test-e2e` in
`sync --full --dry-run`, and asserts the planned create, status-set, and delete
— and that no mutating request reached the mock and the link map is unchanged.

## Test-only HTTPS seam

Adapters require HTTPS base URLs. For the fixture only, an HTTP base pointing at
a loopback host is accepted when `PLANESYNC_ALLOW_INSECURE_BASE_URLS=1`. This is
off by default (unit-tested), so production runs stay HTTPS-only.
