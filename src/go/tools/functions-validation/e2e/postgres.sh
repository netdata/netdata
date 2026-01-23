#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "postgres"
trap cleanup EXIT

POSTGRES_PORT="$(reserve_port)"
write_env "POSTGRES_PORT" "$POSTGRES_PORT"
replace_in_file "$WORKDIR/config/go.d/postgres.conf" "127.0.0.1:5432" "127.0.0.1:${POSTGRES_PORT}"

compose_up postgres
wait_healthy postgres 90

build_plugin
run_info postgres
run_top_queries postgres

echo "E2E checks passed for postgres." >&2
