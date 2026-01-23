#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

run_top_queries_retry() {
  local module="$1"
  local attempts="${2:-20}"
  local delay="${3:-0.5}"
  local output="$WORKDIR/${module}-top-queries.json"
  local i=1

  while [ "$i" -le "$attempts" ]; do
    run "$WORKDIR/go.d.plugin" \
      --config-dir "$WORKDIR/config" \
      --function "${module}:top-queries" \
      --function-args __job:local \
      > "$output"
    validate "$output"
    if has_min_rows "$output" 1; then
      return 0
    fi
    sleep "$delay"
    i=$((i + 1))
  done

  echo "top-queries returned 0 rows after ${attempts} attempts" >&2
  return 1
}

init_workspace "elasticsearch"
trap cleanup EXIT

ELASTICSEARCH_PORT="$(reserve_port)"
write_env "ELASTICSEARCH_PORT" "$ELASTICSEARCH_PORT"
replace_in_file "$WORKDIR/config/go.d/elasticsearch.conf" "127.0.0.1:9200" "127.0.0.1:${ELASTICSEARCH_PORT}"

compose_up elasticsearch
wait_healthy elasticsearch 120
compose_run elasticsearch-init
compose_up elasticsearch-searcher

build_plugin
run_info elasticsearch
run_top_queries_retry elasticsearch

echo "E2E checks passed for elasticsearch." >&2
