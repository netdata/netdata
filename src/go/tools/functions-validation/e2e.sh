#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Execute command with visibility
run() {
  # Print the command being executed
  printf >&2 "${GRAY}$(pwd) >${NC} "
  printf >&2 "${YELLOW}"
  printf >&2 "%q " "$@"
  printf >&2 "${NC}\n"

  # Execute the command
  set +e
  "$@"
  local exit_code=$?
  set -e
  if [ $exit_code -ne 0 ]; then
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return $exit_code
  fi
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
WORKDIR="$(mktemp -d /tmp/netdata-functions-e2e.XXXXXX)"
PROJECT_SUFFIX="$(basename "$WORKDIR")"
PROJECT_SUFFIX="$(printf '%s' "$PROJECT_SUFFIX" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9_-' '-')"
PROJECT_SUFFIX="${PROJECT_SUFFIX%-}"
PROJECT="netdata-func-e2e-$PROJECT_SUFFIX"
COMPOSE=(docker compose -f "$WORKDIR/docker-compose.yml" -p "$PROJECT")
COMPOSE_STARTED=""

cleanup() {
  local exit_code=$?
  set +e
  if [ -n "$COMPOSE_STARTED" ]; then
    run "${COMPOSE[@]}" down -v --remove-orphans
  fi
  if [ "$exit_code" -eq 0 ]; then
    run rm -rf "$WORKDIR"
  else
    echo "E2E failed. Keeping workspace: $WORKDIR" >&2
  fi
  exit $exit_code
}
trap cleanup EXIT

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

run cp -a "$SCRIPT_DIR/docker-compose.yml" "$SCRIPT_DIR/seed" "$SCRIPT_DIR/config" "$WORKDIR/"

run "${COMPOSE[@]}" up -d
COMPOSE_STARTED="yes"

wait_healthy postgres 90
wait_healthy mysql 90
wait_healthy mssql 120
wait_healthy mongo 90

run "${COMPOSE[@]}" run --rm mongo-init

run bash -c "cd \"$REPO_ROOT/src/go\" && go build -o \"$WORKDIR/go.d.plugin\" ./cmd/godplugin"

validate() {
  local input="$1"
  shift
  (cd "$REPO_ROOT/src/go" && run go run ./tools/functions-validation/validate --input "$input" "$@")
}

run_info() {
  local module="$1"
  local output="$WORKDIR/${module}-info.json"
  run "$WORKDIR/go.d.plugin" \
    --config-dir "$WORKDIR/config" \
    --function "${module}:top-queries" \
    --function-args info \
    > "$output"
  validate "$output"
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

run_info postgres
run_top_queries postgres

run_info mysql
run_top_queries mysql

run_info mssql
run_top_queries mssql

run_info mongodb
run_top_queries mongodb

echo "E2E checks passed." >&2
