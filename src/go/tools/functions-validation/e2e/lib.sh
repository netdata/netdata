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
