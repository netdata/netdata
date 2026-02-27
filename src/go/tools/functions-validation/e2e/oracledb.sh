#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "oracledb"
trap cleanup EXIT

ORACLE_PORT="$(reserve_port)"
write_env "ORACLE_PORT" "$ORACLE_PORT"
replace_in_file "$WORKDIR/config/go.d/oracledb.conf" "127.0.0.1:1521" "127.0.0.1:${ORACLE_PORT}"

compose_up oracledb
wait_healthy oracledb 300

compose_run oracledb-seed

run_bg compose_run oracledb-sleep
SLEEP_PID="$LAST_BG_PID"
sleep 3

build_plugin
run_info oracledb
run_top_queries oracledb
run_running_queries oracledb 1

if kill -0 "$SLEEP_PID" 2>/dev/null; then
  kill "$SLEEP_PID"
  wait "$SLEEP_PID" || true
fi

echo "E2E checks passed for oracledb." >&2
