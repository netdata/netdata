#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "mssql"
trap cleanup EXIT

MSSQL_PORT="$(reserve_port)"
write_env "MSSQL_PORT" "$MSSQL_PORT"
replace_in_file "$WORKDIR/config/go.d/mssql.conf" "127.0.0.1:1433" "127.0.0.1:${MSSQL_PORT}"

compose_up mssql
wait_healthy mssql 120
compose_run mssql-init

build_plugin
run_info mssql
run_top_queries mssql

echo "E2E checks passed for mssql." >&2
