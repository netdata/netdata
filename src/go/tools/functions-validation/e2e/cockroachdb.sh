#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "cockroachdb"
trap cleanup EXIT

COCKROACH_HTTP_PORT="$(reserve_port)"
COCKROACH_SQL_PORT="$(reserve_port)"
write_env "COCKROACH_HTTP_PORT" "$COCKROACH_HTTP_PORT"
write_env "COCKROACH_SQL_PORT" "$COCKROACH_SQL_PORT"
replace_in_file "$WORKDIR/config/go.d/cockroachdb.conf" "127.0.0.1:8080" "127.0.0.1:${COCKROACH_HTTP_PORT}"
replace_in_file "$WORKDIR/config/go.d/cockroachdb.conf" "127.0.0.1:26258" "127.0.0.1:${COCKROACH_SQL_PORT}"

compose_up cockroachdb
wait_healthy cockroachdb 120
compose_run cockroachdb-seed

run_bg compose_run cockroachdb-sleep
SLEEP_PID="$LAST_BG_PID"
sleep 3

build_plugin
run_info cockroachdb
run_top_queries cockroachdb
run_running_queries cockroachdb 1

if kill -0 "$SLEEP_PID" 2>/dev/null; then
  kill "$SLEEP_PID"
  wait "$SLEEP_PID" || true
fi

echo "E2E checks passed for cockroachdb." >&2
