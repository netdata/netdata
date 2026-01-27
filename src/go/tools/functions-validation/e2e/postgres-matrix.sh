#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# PostgreSQL versions 14-18 (all currently supported versions)
# Version 13 reached EOL in November 2025
POSTGRES_VARIANTS=(
  "postgres:14|postgres-14"
  "postgres:15|postgres-15"
  "postgres:16|postgres-16"
  "postgres:17|postgres-17"
  "postgres:18|postgres-18"
)

for entry in "${POSTGRES_VARIANTS[@]}"; do
  IFS='|' read -r image label <<< "$entry"
  printf '\n=== Running PostgreSQL collector E2E for %s (%s) ===\n' "$label" "$image" >&2
  POSTGRES_IMAGE="$image" POSTGRES_VARIANT="$label" bash "$SCRIPT_DIR/postgres.sh"
done
