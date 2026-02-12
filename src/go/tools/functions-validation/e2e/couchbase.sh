#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "couchbase"
trap cleanup EXIT

COUCHBASE_HTTP_PORT="$(reserve_port)"
COUCHBASE_QUERY_PORT="$(reserve_port)"
write_env "COUCHBASE_HTTP_PORT" "$COUCHBASE_HTTP_PORT"
write_env "COUCHBASE_QUERY_PORT" "$COUCHBASE_QUERY_PORT"
replace_in_file "$WORKDIR/config/go.d/couchbase.conf" "127.0.0.1:8091" "127.0.0.1:${COUCHBASE_HTTP_PORT}"
replace_in_file "$WORKDIR/config/go.d/couchbase.conf" "127.0.0.1:8093" "127.0.0.1:${COUCHBASE_QUERY_PORT}"

compose_up couchbase
wait_healthy couchbase 180
compose_run couchbase-init

wait_query_service() {
  local attempt
  for attempt in $(seq 1 60); do
    : "$attempt"  # loop counter
    if curl -fsS -u "Administrator:password" "http://127.0.0.1:${COUCHBASE_QUERY_PORT}/query/service" \
      --data-urlencode "statement=SELECT 1" > /dev/null; then
      return 0
    fi
    sleep 2
  done
  echo "Couchbase query service not ready on host port ${COUCHBASE_QUERY_PORT}" >&2
  return 1
}

wait_query_service
curl -fsS -u "Administrator:password" "http://127.0.0.1:${COUCHBASE_QUERY_PORT}/query/service" \
  --data-urlencode "statement=SELECT 1" > /dev/null

build_plugin
run_info couchbase
run_top_queries couchbase

echo "E2E checks passed for couchbase." >&2
