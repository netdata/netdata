#!/usr/bin/env bash
set -euo pipefail

# Colors for output
RED='\033[0;31m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Execute command with visibility
run() {
  # Print the command being executed
  printf >&2 '%s%s >%s ' "$GRAY" "$(pwd)" "$NC"
  printf >&2 '%s' "$YELLOW"
  printf >&2 "%q " "$@"
  printf >&2 '%s\n' "$NC"

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
E2E_DIR="$SCRIPT_DIR/e2e"

DBS=(postgres mysql mssql mongodb redis clickhouse elasticsearch couchbase proxysql cockroachdb yugabytedb oracledb rethinkdb)
JOBS=1
ONLY=""

usage() {
  cat <<'USAGE'
Usage: ./e2e.sh [--only db1,db2] [--jobs N] [--list]

Options:
  --only   Comma-separated list of DBs to run (e.g. postgres,mysql)
  --jobs   Max number of concurrent DB runs (default: 1)
  --list   Show available DBs
  --help   Show this help
USAGE
}

list_dbs() {
  printf '%s\n' "${DBS[@]}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --only)
      ONLY="${2:-}"
      if [ -z "$ONLY" ]; then
        echo "--only requires a value" >&2
        usage
        exit 1
      fi
      shift 2
      ;;
    --jobs)
      JOBS="${2:-}"
      if [ -z "$JOBS" ]; then
        echo "--jobs requires a value" >&2
        usage
        exit 1
      fi
      shift 2
      ;;
    --list)
      list_dbs
      exit 0
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! [[ "$JOBS" =~ ^[0-9]+$ ]] || [ "$JOBS" -le 0 ]; then
  echo "--jobs must be a positive integer" >&2
  exit 1
fi

if [ -n "$ONLY" ]; then
  IFS=',' read -r -a DBS <<< "${ONLY// /}"
fi

for db in "${DBS[@]}"; do
  if [ ! -f "$E2E_DIR/${db}.sh" ]; then
    echo "Unknown DB script: $db" >&2
    exit 1
  fi
done

pids=()
names=()
failures=()

wait_for_any() {
  local i pid status db
  while true; do
    for i in "${!pids[@]}"; do
      pid="${pids[$i]}"
      if ! kill -0 "$pid" 2>/dev/null; then
        set +e
        wait "$pid"
        status=$?
        set -e
        db="${names[$i]}"
        if [ $status -ne 0 ]; then
          failures+=("$db")
        fi
        unset 'pids[i]' 'names[i]'
        pids=("${pids[@]}")
        names=("${names[@]}")
        return 0
      fi
    done
    sleep 0.2
  done
}

start_job() {
  local db="$1"
  local script="$E2E_DIR/${db}.sh"
  local pid
  run_bg bash "$script"
  pid="$LAST_BG_PID"
  pids+=("$pid")
  names+=("$db")
}

for db in "${DBS[@]}"; do
  start_job "$db"
  while [ "${#pids[@]}" -ge "$JOBS" ]; do
    wait_for_any
  done
done

while [ "${#pids[@]}" -gt 0 ]; do
  wait_for_any
done

if [ "${#failures[@]}" -ne 0 ]; then
  printf >&2 "%s\n" "E2E failures: ${failures[*]}"
  exit 1
fi

echo "E2E checks passed." >&2
