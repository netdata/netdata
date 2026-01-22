#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "clickhouse"
trap cleanup EXIT

CLICKHOUSE_HTTP_PORT="$(reserve_port)"
write_env "CLICKHOUSE_HTTP_PORT" "$CLICKHOUSE_HTTP_PORT"
replace_in_file "$WORKDIR/config/go.d/clickhouse.conf" "127.0.0.1:8123" "127.0.0.1:${CLICKHOUSE_HTTP_PORT}"

compose_up clickhouse
wait_healthy clickhouse 90
compose_run clickhouse-init

build_plugin
run_info clickhouse
run_top_queries clickhouse

echo "E2E checks passed for clickhouse." >&2
