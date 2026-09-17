#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
binary_path=${PLANESYNC_E2E_BINARY:-${1:-}}
if [ -z "$binary_path" ] || [ ! -x "$binary_path" ]; then
	echo "usage: PLANESYNC_E2E_BINARY=/path/to/planesync $0" >&2
	exit 2
fi
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/planesync-e2e.XXXXXX")
server_pid=

cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$fixture_dir"
}
trap cleanup EXIT HUP INT TERM

go build -o "$fixture_dir/mockserver" "$repo_dir/tests/mockserver"
"$fixture_dir/mockserver" -ready "$fixture_dir/ready" -violations "$fixture_dir/violations" &
server_pid=$!

attempt=0
while [ ! -s "$fixture_dir/ready" ]; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 100 ]; then
		echo "mock server did not become ready" >&2
		exit 1
	fi
	sleep 0.05
done
base_url=$(sed -n '1p' "$fixture_dir/ready")

mkdir -p "$fixture_dir/config"
printf '%s\n' '{"version":1,"links":{"item-missing":"archived-target"}}' > "$fixture_dir/config/planesync-links.json"
cat > "$fixture_dir/config/planesync.jsonc" <<EOF
{
  "jira": {
    "base_url": "$base_url",
    "cloud_id": "fixture-cloud",
    "email": "fixture@example.com",
    "auth_type": "basic",
    "token": "fixture-token"
  },
  "plane": {
    "base_url": "$base_url",
    "workspace": "source-workspace",
    "token": "fixture-token"
  },
  "defaults": {
    "since": "7d",
    "body_format": "rich",
    "deleted_status": "Archived"
  },
  "projects": [{
    "plane_project": "source-project",
    "jira_project": "target-project",
    "jira_issue_type": "Task",
    "title_prefix": "SRC",
    "status_map": {"Open": "In Progress"}
  }]
}
EOF

if ! output=$(PLANESYNC_ALLOW_INSECURE_BASE_URLS=1 "$binary_path" sync --full --dry-run --config "$fixture_dir/config/planesync.jsonc" 2>&1); then
	printf '%s\n' "$output" >&2
	exit 1
fi
printf '%s\n' "$output"

printf '%s\n' "$output" | grep -F 'created=1 updated=0 status-set=1 deleted=1 skipped=0' >/dev/null
printf '%s\n' "$output" | grep -F 'create source=item-current' >/dev/null
printf '%s\n' "$output" | grep -F 'set-status source=item-current' >/dev/null
printf '%s\n' "$output" | grep -F 'delete source=item-missing target=archived-target detail="Archived"' >/dev/null

if [ -s "$fixture_dir/violations" ]; then
	echo "dry-run made forbidden writes:" >&2
	sed -n '1,40p' "$fixture_dir/violations" >&2
	exit 1
fi

link_state=$(sed -n '1p' "$fixture_dir/config/planesync-links.json")
if [ "$link_state" != '{"version":1,"links":{"item-missing":"archived-target"}}' ]; then
	echo "dry-run changed the local link map" >&2
	exit 1
fi
