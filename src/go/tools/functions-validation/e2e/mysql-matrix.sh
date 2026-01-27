#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

MYSQL_VARIANTS=(
  "mysql:5.7|mysql-5.7"
  "mysql:8.0|mysql-8.0"
  "mysql:8.4|mysql-8.4"
  "mariadb:10.3|mariadb-10.3"
  "mariadb:10.6|mariadb-10.6"
  "mariadb:11.4|mariadb-11.4"
  "percona:5.7|percona-5.7"
  "percona:8.0|percona-8.0"
)

for entry in "${MYSQL_VARIANTS[@]}"; do
  IFS='|' read -r image label <<< "$entry"
  printf '\n=== Running MySQL collector E2E for %s (%s) ===\n' "$label" "$image" >&2
  MYSQL_IMAGE="$image" MYSQL_VARIANT="$label" bash "$SCRIPT_DIR/mysql.sh"
done
