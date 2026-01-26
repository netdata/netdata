#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "mysql"
trap cleanup EXIT

MYSQL_PORT="$(reserve_port)"
write_env "MYSQL_PORT" "$MYSQL_PORT"
replace_in_file "$WORKDIR/config/go.d/mysql.conf" "127.0.0.1:3306" "127.0.0.1:${MYSQL_PORT}"

compose_up mysql
wait_healthy mysql 90

build_plugin
run_info mysql
run_top_queries mysql

echo "E2E checks passed for mysql." >&2
