#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "mongodb"
trap cleanup EXIT

MONGO_PORT="$(reserve_port)"
write_env "MONGO_PORT" "$MONGO_PORT"
replace_in_file "$WORKDIR/config/go.d/mongodb.conf" "127.0.0.1:27017" "127.0.0.1:${MONGO_PORT}"

compose_up mongo
wait_healthy mongo 90
compose_run mongo-init

build_plugin
run_info mongodb
run_top_queries mongodb

echo "E2E checks passed for mongodb." >&2
