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

# Support pg_stat_monitor testing with Percona distribution
# POSTGRES_STATS_EXT can be "pg_stat_statements" (default) or "pg_stat_monitor"
POSTGRES_STATS_EXT="${POSTGRES_STATS_EXT:-pg_stat_statements}"
write_env "POSTGRES_PRELOAD_LIBRARIES" "$POSTGRES_STATS_EXT"

# Use the appropriate init script based on extension
if [ "$POSTGRES_STATS_EXT" = "pg_stat_monitor" ]; then
  cp "$WORKDIR/seed/postgres/init-pgsm.sql" "$WORKDIR/seed/postgres/init.sql"
fi

compose_up postgres
wait_healthy postgres 90

build_plugin

# Test top-queries (pg_stat_statements or pg_stat_monitor)
run_info postgres
run_top_queries postgres
assert_column_visibility "$WORKDIR/postgres-top-queries.json" "top-queries"

# Verify the correct extension is being used
if [ "$POSTGRES_STATS_EXT" = "pg_stat_monitor" ]; then
  # pg_stat_monitor should have applicationName and cpuUserTime columns
  if ! jq -e '.columns | has("applicationName")' "$WORKDIR/postgres-top-queries.json" > /dev/null 2>&1; then
    echo "ERROR: pg_stat_monitor expected but applicationName column not found" >&2
    exit 1
  fi
  echo "Verified: pg_stat_monitor is being used (applicationName column present)" >&2
else
  # pg_stat_statements should NOT have applicationName column
  if jq -e '.columns | has("applicationName")' "$WORKDIR/postgres-top-queries.json" > /dev/null 2>&1; then
    echo "ERROR: pg_stat_statements expected but applicationName column found" >&2
    exit 1
  fi
  echo "Verified: pg_stat_statements is being used" >&2
fi

# Test running-queries (pg_stat_activity)
# Note: running-queries may return 0 rows if no active queries at test time
run_info_method postgres running-queries
run_running_queries postgres 0
assert_column_visibility "$WORKDIR/postgres-running-queries.json" "running-queries"

echo "E2E checks passed for ${POSTGRES_VARIANT_LABEL} with ${POSTGRES_STATS_EXT}." >&2
