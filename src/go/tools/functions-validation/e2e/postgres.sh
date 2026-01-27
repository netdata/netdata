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

POSTGRES_VARIANT_LABEL="${POSTGRES_VARIANT:-postgres}"
if [ -n "${POSTGRES_IMAGE:-}" ]; then
  write_env "POSTGRES_IMAGE" "$POSTGRES_IMAGE"
fi

compose_up postgres
wait_healthy postgres 90

build_plugin
run_info postgres
run_top_queries postgres
assert_column_visibility "$WORKDIR/postgres-top-queries.json" "top-queries"

echo "E2E checks passed for ${POSTGRES_VARIANT_LABEL}." >&2
