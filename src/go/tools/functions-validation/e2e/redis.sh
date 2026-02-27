#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "redis"
trap cleanup EXIT

REDIS_PORT="$(reserve_port)"
write_env "REDIS_PORT" "$REDIS_PORT"
replace_in_file "$WORKDIR/config/go.d/redis.conf" "127.0.0.1:6379" "127.0.0.1:${REDIS_PORT}"

compose_up redis
wait_healthy redis 60
compose_run redis-init

build_plugin
run_info redis
run_top_queries redis

echo "E2E checks passed for redis." >&2
