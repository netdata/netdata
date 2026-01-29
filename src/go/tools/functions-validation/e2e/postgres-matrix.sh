#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# PostgreSQL versions 14-18 with pg_stat_statements (standard)
# Version 13 reached EOL in November 2025
POSTGRES_VARIANTS=(
  "postgres:14|postgres-14|pg_stat_statements"
  "postgres:15|postgres-15|pg_stat_statements"
  "postgres:16|postgres-16|pg_stat_statements"
  "postgres:17|postgres-17|pg_stat_statements"
  "postgres:18|postgres-18|pg_stat_statements"
)

# pg_stat_monitor variants using Percona distribution
# Note: Percona images include pg_stat_monitor pre-installed
PGSM_VARIANTS=(
  "percona/percona-distribution-postgresql:16|percona-pgsm-16|pg_stat_monitor"
  "percona/percona-distribution-postgresql:17|percona-pgsm-17|pg_stat_monitor"
)

# Run pg_stat_statements tests
for entry in "${POSTGRES_VARIANTS[@]}"; do
  IFS='|' read -r image label ext <<< "$entry"
  printf '\n=== Running PostgreSQL collector E2E for %s (%s) with %s ===\n' "$label" "$image" "$ext" >&2
  POSTGRES_IMAGE="$image" POSTGRES_VARIANT="$label" POSTGRES_STATS_EXT="$ext" bash "$SCRIPT_DIR/postgres.sh"
done

# Run pg_stat_monitor tests
for entry in "${PGSM_VARIANTS[@]}"; do
  IFS='|' read -r image label ext <<< "$entry"
  printf '\n=== Running PostgreSQL collector E2E for %s (%s) with %s ===\n' "$label" "$image" "$ext" >&2
  POSTGRES_IMAGE="$image" POSTGRES_VARIANT="$label" POSTGRES_STATS_EXT="$ext" bash "$SCRIPT_DIR/postgres.sh"
done

echo ""
echo "=== All PostgreSQL E2E tests passed ==="
echo "  - pg_stat_statements: PostgreSQL 14, 15, 16, 17, 18"
echo "  - pg_stat_monitor: Percona 16, 17"
