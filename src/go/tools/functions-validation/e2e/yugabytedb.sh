#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "yugabytedb"
trap cleanup EXIT

YUGABYTE_MASTER_PORT="$(reserve_port)"
YUGABYTE_YSQL_PORT="$(reserve_port)"
write_env "YUGABYTE_MASTER_PORT" "$YUGABYTE_MASTER_PORT"
write_env "YUGABYTE_YSQL_PORT" "$YUGABYTE_YSQL_PORT"
replace_in_file "$WORKDIR/config/go.d/yugabytedb.conf" "127.0.0.1:7000" "127.0.0.1:${YUGABYTE_MASTER_PORT}"
replace_in_file "$WORKDIR/config/go.d/yugabytedb.conf" "127.0.0.1:5433" "127.0.0.1:${YUGABYTE_YSQL_PORT}"

compose_up yugabytedb
wait_healthy yugabytedb 240
compose_run yugabytedb-seed

run_bg compose_run yugabytedb-sleep
SLEEP_PID="$LAST_BG_PID"
sleep 3

build_plugin
run_info yugabytedb
run_top_queries yugabytedb
run_running_queries yugabytedb 1

if kill -0 "$SLEEP_PID" 2>/dev/null; then
  kill "$SLEEP_PID"
  wait "$SLEEP_PID" || true
fi

echo "E2E checks passed for yugabytedb." >&2
