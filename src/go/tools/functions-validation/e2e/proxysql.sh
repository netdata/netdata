#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "proxysql"
trap cleanup EXIT

MYSQL_PORT="$(reserve_port)"
PROXYSQL_ADMIN_PORT="$(reserve_port)"
PROXYSQL_MYSQL_PORT="$(reserve_port)"
write_env "MYSQL_PORT" "$MYSQL_PORT"
write_env "PROXYSQL_ADMIN_PORT" "$PROXYSQL_ADMIN_PORT"
write_env "PROXYSQL_MYSQL_PORT" "$PROXYSQL_MYSQL_PORT"

compose_up mysql proxysql
wait_healthy mysql 90
compose_run proxysql-init

build_plugin

PROXYSQL_CID="$("${COMPOSE[@]}" ps -q proxysql)"
if [ -z "$PROXYSQL_CID" ]; then
  echo "Unable to resolve proxysql container ID" >&2
  exit 1
fi

run_info_proxysql() {
  local output="$WORKDIR/proxysql-info.json"
  run docker run --rm \
    --network "container:${PROXYSQL_CID}" \
    -v "$WORKDIR:/work" \
    -w /work \
    debian:bookworm-slim \
    /work/go.d.plugin \
    --config-dir /work/config \
    --function proxysql:top-queries \
    --function-args info \
    > "$output"
  validate "$output"
}

run_top_queries_proxysql() {
  local output="$WORKDIR/proxysql-top-queries.json"
  run docker run --rm \
    --network "container:${PROXYSQL_CID}" \
    -v "$WORKDIR:/work" \
    -w /work \
    debian:bookworm-slim \
    /work/go.d.plugin \
    --config-dir /work/config \
    --function proxysql:top-queries \
    --function-args __job:local \
    > "$output"
  validate "$output" --min-rows 1
}

run_info_proxysql
run_top_queries_proxysql

echo "E2E checks passed for proxysql." >&2
