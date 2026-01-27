#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Execute command with visibility
run() {
  local errexit_set=0
  case $- in
    *e*) errexit_set=1 ;;
  esac

  # Print the command being executed
  printf >&2 '%s%s >%s ' "$GRAY" "$(pwd)" "$NC"
  printf >&2 '%s' "$YELLOW"
  printf >&2 "%q " "$@"
  printf >&2 '%s\n' "$NC"

  # Execute the command
  set +e
  "$@"
  local exit_code=$?
  if [ $errexit_set -eq 1 ]; then
    set -e
  else
    set +e
  fi
  if [ $exit_code -ne 0 ]; then
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return $exit_code
  fi
}

# Execute command in background with visibility
LAST_BG_PID=""
run_bg() {
  printf >&2 '%s%s >%s ' "$GRAY" "$(pwd)" "$NC"
  printf >&2 '%s' "$YELLOW"
  printf >&2 "%q " "$@"
  printf >&2 '%s\n' "$NC"
  "$@" &
  LAST_BG_PID=$!
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FUNCTIONS_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(cd "$FUNCTIONS_DIR/../../../.." && pwd)"

WORKDIR=""
PROJECT=""
COMPOSE=()
COMPOSE_STARTED=""
USED_PORTS=""

cleanup() {
  local exit_code=$?
  set +e
  if [ -n "${COMPOSE_STARTED:-}" ]; then
    run "${COMPOSE[@]}" down -v --remove-orphans
  fi
  if [ "$exit_code" -eq 0 ]; then
    run rm -rf "$WORKDIR"
  else
    echo "E2E failed. Keeping workspace: $WORKDIR" >&2
  fi
  exit $exit_code
}

init_workspace() {
  local db="$1"
  WORKDIR="$(mktemp -d "/tmp/netdata-functions-e2e-${db}.XXXXXX")"
  local project_suffix
  project_suffix="$(basename "$WORKDIR")"
  project_suffix="$(printf '%s' "$project_suffix" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-')"
  project_suffix="${project_suffix%-}"
  PROJECT="netdata-func-e2e-${db}-${project_suffix}"
  COMPOSE=(docker compose -f "$WORKDIR/docker-compose.yml" -p "$PROJECT")

  run cp -a "$FUNCTIONS_DIR/docker-compose.yml" "$FUNCTIONS_DIR/seed" "$FUNCTIONS_DIR/config" "$WORKDIR/"
  : > "$WORKDIR/.env"
}

compose_up() {
  run "${COMPOSE[@]}" up -d "$@"
  COMPOSE_STARTED="yes"
}

compose_run() {
  run "${COMPOSE[@]}" run --rm "$@"
}

wait_healthy() {
  local service="$1"
  local timeout="${2:-60}"
  local start=$SECONDS

  while true; do
    local cid
    cid=$("${COMPOSE[@]}" ps -q "$service")
    if [ -z "$cid" ]; then
      if [ $((SECONDS - start)) -ge "$timeout" ]; then
        echo "No container found for service: $service" >&2
        return 1
      fi
      sleep 2
      continue
    fi

    local status
    status="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid")"
    if [ "$status" = "healthy" ]; then
      return 0
    fi
    if [ $((SECONDS - start)) -ge "$timeout" ]; then
      echo "Timed out waiting for $service to be healthy" >&2
      return 1
    fi
    sleep 2
  done
}

write_env() {
  local key="$1"
  local value="$2"
  echo "${key}=${value}" >> "$WORKDIR/.env"
  export "${key}=${value}"
}

pick_free_port() {
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("", 0))
print(s.getsockname()[1])
s.close()
PY
  elif command -v python >/dev/null 2>&1; then
    python - <<'PY'
import socket
s = socket.socket()
s.bind(("", 0))
print(s.getsockname()[1])
s.close()
PY
  else
    echo "python3 (or python) is required to select a free port" >&2
    return 1
  fi
}

reserve_port() {
  local port
  while true; do
    port="$(pick_free_port)"
    case " $USED_PORTS " in
      *" $port "*) ;;
      *)
        USED_PORTS="${USED_PORTS} ${port}"
        echo "$port"
        return 0
        ;;
    esac
  done
}

replace_in_file() {
  local file="$1"
  local search="$2"
  local replace="$3"
  run sed -i "s|$search|$replace|g" "$file"
}

build_plugin() {
  run bash -c "cd \"$REPO_ROOT/src/go\" && go build -o \"$WORKDIR/go.d.plugin\" ./cmd/godplugin"
}

validate() {
  local input="$1"
  shift
  (cd "$REPO_ROOT/src/go" && run go run ./tools/functions-validation/validate --input "$input" "$@")
}

run_info_method() {
  local module="$1"
  local method="$2"
  local output="$WORKDIR/${module}-${method}-info.json"
  run "$WORKDIR/go.d.plugin" \
    --config-dir "$WORKDIR/config" \
    --function "${module}:${method}" \
    --function-args info \
    > "$output"
  validate "$output"
}

run_info() {
  local module="$1"
  run_info_method "$module" "top-queries"
}

run_function() {
  local module="$1"
  local method="$2"
  local args="${3:-__job:local}"
  local require_rows="${4:-true}"
  local output="$WORKDIR/${module}-${method}.json"

  run "$WORKDIR/go.d.plugin" \
    --config-dir "$WORKDIR/config" \
    --function "${module}:${method}" \
    --function-args "$args" \
    > "$output"

  if [ "$require_rows" = "true" ]; then
    validate "$output" --min-rows 1
  else
    validate "$output"
  fi

  echo "$output"
}

run_top_queries() {
  local module="$1"
  local output="$WORKDIR/${module}-top-queries.json"
  run "$WORKDIR/go.d.plugin" \
    --config-dir "$WORKDIR/config" \
    --function "${module}:top-queries" \
    --function-args __job:local \
    > "$output"
  validate "$output" --min-rows 1
}

run_running_queries() {
  local module="$1"
  local min_rows="${2:-1}"
  local output="$WORKDIR/${module}-running-queries.json"
  run "$WORKDIR/go.d.plugin" \
    --config-dir "$WORKDIR/config" \
    --function "${module}:running-queries" \
    --function-args __job:local \
    > "$output"
  validate "$output" --min-rows "$min_rows"
}

has_min_rows() {
  local input="$1"
  local min_rows="${2:-1}"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" "$min_rows" <<'PY'
import json
import sys

path = sys.argv[1]
min_rows = int(sys.argv[2])
with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)
rows = data.get("data", [])
sys.exit(0 if len(rows) >= min_rows else 1)
PY
  else
    python - "$input" "$min_rows" <<'PY'
import json
import sys

path = sys.argv[1]
min_rows = int(sys.argv[2])
with open(path, "r", encoding="utf-8") as fh:
    data = json.load(fh)
rows = data.get("data", [])
sys.exit(0 if len(rows) >= min_rows else 1)
PY
  fi
}

# Assert column visibility rules:
# - If fewer than 5 columns exist, ALL must be visible
# - Otherwise, at least 5 columns must be visible
assert_column_visibility() {
  local input="$1"
  local context="${2:-response}"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" "$context" <<'PY'
import json
import sys

path = sys.argv[1]
context = sys.argv[2]

with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}

# Build list of columns with their visibility
col_list = []
if isinstance(columns, dict):
    for field, col in columns.items():
        if isinstance(col, dict):
            col_list.append({"field": field, "visible": col.get("visible", False)})
else:
    for col in columns:
        if isinstance(col, dict):
            col_list.append({"field": col.get("field", ""), "visible": col.get("visible", False)})

total = len(col_list)
visible_count = sum(1 for c in col_list if c["visible"] is True)

if total < 5:
    # All columns must be visible
    if visible_count != total:
        invisible = [c["field"] for c in col_list if c["visible"] is not True]
        raise SystemExit(
            f"{context}: All {total} columns must be visible (fewer than 5 total), "
            f"but only {visible_count} are visible. Invisible columns: {invisible}"
        )
else:
    # At least 5 columns must be visible
    if visible_count < 5:
        invisible = [c["field"] for c in col_list if c["visible"] is not True]
        raise SystemExit(
            f"{context}: At least 5 columns must be visible, "
            f"but only {visible_count} of {total} are visible. Invisible columns: {invisible}"
        )
PY
  else
    python - "$input" "$context" <<'PY'
import json
import sys

path = sys.argv[1]
context = sys.argv[2]

with open(path, "r") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}

col_list = []
if isinstance(columns, dict):
    for field, col in columns.items():
        if isinstance(col, dict):
            col_list.append({"field": field, "visible": col.get("visible", False)})
else:
    for col in columns:
        if isinstance(col, dict):
            col_list.append({"field": col.get("field", ""), "visible": col.get("visible", False)})

total = len(col_list)
visible_count = sum(1 for c in col_list if c["visible"] is True)

if total < 5:
    if visible_count != total:
        invisible = [c["field"] for c in col_list if c["visible"] is not True]
        raise SystemExit(
            "%s: All %d columns must be visible (fewer than 5 total), "
            "but only %d are visible. Invisible columns: %s" % (context, total, visible_count, invisible)
        )
else:
    if visible_count < 5:
        invisible = [c["field"] for c in col_list if c["visible"] is not True]
        raise SystemExit(
            "%s: At least 5 columns must be visible, "
            "but only %d of %d are visible. Invisible columns: %s" % (context, visible_count, total, invisible)
        )
PY
  fi
}

# Assert UniqueKey column has non-empty values in all rows
assert_unique_key_populated() {
  local input="$1"
  local context="${2:-response}"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" "$context" <<'PY'
import json
import sys

path = sys.argv[1]
context = sys.argv[2]

with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}
data = doc.get("data") or []

# Find UniqueKey column index
unique_key_idx = None
unique_key_field = None

if isinstance(columns, dict):
    for field, col in columns.items():
        if isinstance(col, dict) and col.get("unique_key") is True:
            unique_key_idx = col.get("index")
            unique_key_field = field
            break
else:
    for idx, col in enumerate(columns):
        if isinstance(col, dict) and col.get("unique_key") is True:
            unique_key_idx = idx
            unique_key_field = col.get("field", f"column_{idx}")
            break

if unique_key_idx is None:
    # No UniqueKey column defined, skip check
    sys.exit(0)

# Check all rows have non-empty UniqueKey
for i, row in enumerate(data):
    if unique_key_idx >= len(row):
        raise SystemExit(f"{context}: Row {i} missing UniqueKey column (index {unique_key_idx})")
    val = row[unique_key_idx]
    if val is None or str(val).strip() == "":
        raise SystemExit(
            f"{context}: Row {i} has empty UniqueKey ({unique_key_field}) - "
            f"deduplication will fail. This may indicate NULL digest handling is broken."
        )
PY
  else
    python - "$input" "$context" <<'PY'
import json
import sys

path = sys.argv[1]
context = sys.argv[2]

with open(path, "r") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}
data = doc.get("data") or []

unique_key_idx = None
unique_key_field = None

if isinstance(columns, dict):
    for field, col in columns.items():
        if isinstance(col, dict) and col.get("unique_key") is True:
            unique_key_idx = col.get("index")
            unique_key_field = field
            break
else:
    for idx, col in enumerate(columns):
        if isinstance(col, dict) and col.get("unique_key") is True:
            unique_key_idx = idx
            unique_key_field = col.get("field", "column_%d" % idx)
            break

if unique_key_idx is None:
    sys.exit(0)

for i, row in enumerate(data):
    if unique_key_idx >= len(row):
        raise SystemExit("%s: Row %d missing UniqueKey column (index %d)" % (context, i, unique_key_idx))
    val = row[unique_key_idx]
    if val is None or str(val).strip() == "":
        raise SystemExit(
            "%s: Row %d has empty UniqueKey (%s) - "
            "deduplication will fail. This may indicate NULL digest handling is broken." % (context, i, unique_key_field)
        )
PY
  fi
}
