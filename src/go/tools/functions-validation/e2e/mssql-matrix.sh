#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MSSQL_VARIANTS=(
  "mcr.microsoft.com/mssql/server:2017-latest|mssql-2017"
  "mcr.microsoft.com/mssql/server:2019-latest|mssql-2019"
  "mcr.microsoft.com/mssql/server:2022-latest|mssql-2022"
)

for entry in "${MSSQL_VARIANTS[@]}"; do
  IFS='|' read -r image label <<< "$entry"
  printf '\n=== Running MSSQL collector E2E for %s (%s) ===\n' "$label" "$image" >&2
  MSSQL_IMAGE="$image" MSSQL_VARIANT="$label" bash "$SCRIPT_DIR/mssql.sh"
done
