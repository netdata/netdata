#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "mysql"
trap cleanup EXIT

MYSQL_PORT="$(reserve_port)"
write_env "MYSQL_PORT" "$MYSQL_PORT"
replace_in_file "$WORKDIR/config/go.d/mysql.conf" "127.0.0.1:3306" "127.0.0.1:${MYSQL_PORT}"

compose_up mysql
wait_healthy mysql 90

build_plugin
run_info mysql
run_top_queries mysql

mysql_container_id() {
  "${COMPOSE[@]}" ps -q mysql
}

mysql_exec_root() {
  local sql="$1"
  local cid
  cid="$(mysql_container_id)"
  if [ -z "$cid" ]; then
    echo "MySQL container ID not found" >&2
    return 1
  fi
  run docker exec -i "$cid" mysql -uroot -prootpw netdata -e "$sql"
}

induce_deadlock_once() {
  local tx1
  local tx2

  tx1="$(cat <<'SQL'
SET SESSION innodb_lock_wait_timeout = 5;
START TRANSACTION;
UPDATE deadlock_a SET value = value + 1 WHERE id = 1;
DO SLEEP(1);
UPDATE deadlock_b SET value = value + 1 WHERE id = 1;
COMMIT;
SQL
)"

  tx2="$(cat <<'SQL'
SET SESSION innodb_lock_wait_timeout = 5;
START TRANSACTION;
UPDATE deadlock_b SET value = value + 1 WHERE id = 1;
DO SLEEP(1);
UPDATE deadlock_a SET value = value + 1 WHERE id = 1;
COMMIT;
SQL
)"

  mysql_exec_root "$tx1" &
  local pid1=$!
  mysql_exec_root "$tx2" &
  local pid2=$!

  wait "$pid1" || true
  wait "$pid2" || true
}

assert_deadlock_info_content() {
  local input="$1"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
import re
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}
field_to_idx = {}
if isinstance(columns, dict):
    for field, col in columns.items():
        if not isinstance(col, dict):
            continue
        try:
            field_to_idx[field] = int(col.get("index"))
        except (TypeError, ValueError):
            continue
else:
    for idx, col in enumerate(columns):
        if not isinstance(col, dict):
            continue
        field = col.get("field")
        if field:
            field_to_idx[field] = idx

for required in ("deadlock_id", "is_victim", "lock_mode", "lock_status", "query_text"):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
if not data:
    raise SystemExit("deadlock-info returned no rows")

def get_value(row, field):
    idx = field_to_idx[field]
    return row[idx] if idx < len(row) else None

def norm(val):
    return "" if val is None else str(val).strip()

has_waiting = any(str(get_value(row, "lock_status")).upper() == "WAITING" for row in data)
if not has_waiting:
    raise SystemExit("no WAITING lock_status found in deadlock-info output")

table_pattern = re.compile(r"deadlock_(a|b)", re.IGNORECASE)
has_expected_query = any(table_pattern.search(str(get_value(row, "query_text"))) for row in data)
if not has_expected_query:
    raise SystemExit("query_text does not reference deadlock tables")

waiting_rows = [row for row in data if norm(get_value(row, "lock_status")).upper() == "WAITING"]
if any(norm(get_value(row, "lock_mode")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include lock_mode")

victim_counts = {}
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    victim_counts.setdefault(deadlock_id, 0)
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
PY
  else
    python - "$input" <<'PY'
import json
import re
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

columns = doc.get("columns") or {}
field_to_idx = {}
if isinstance(columns, dict):
    for field, col in columns.items():
        if not isinstance(col, dict):
            continue
        try:
            field_to_idx[field] = int(col.get("index"))
        except (TypeError, ValueError):
            continue
else:
    for idx, col in enumerate(columns):
        if not isinstance(col, dict):
            continue
        field = col.get("field")
        if field:
            field_to_idx[field] = idx

for required in ("deadlock_id", "is_victim", "lock_mode", "lock_status", "query_text"):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
if not data:
    raise SystemExit("deadlock-info returned no rows")

def get_value(row, field):
    idx = field_to_idx[field]
    return row[idx] if idx < len(row) else None

def norm(val):
    return "" if val is None else str(val).strip()

has_waiting = any(str(get_value(row, "lock_status")).upper() == "WAITING" for row in data)
if not has_waiting:
    raise SystemExit("no WAITING lock_status found in deadlock-info output")

table_pattern = re.compile(r"deadlock_(a|b)", re.IGNORECASE)
has_expected_query = any(table_pattern.search(str(get_value(row, "query_text"))) for row in data)
if not has_expected_query:
    raise SystemExit("query_text does not reference deadlock tables")

waiting_rows = [row for row in data if norm(get_value(row, "lock_status")).upper() == "WAITING"]
if any(norm(get_value(row, "lock_mode")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include lock_mode")

victim_counts = {}
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    victim_counts.setdefault(deadlock_id, 0)
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
PY
  fi
}

verify_deadlock_info() {
  local output=""
  local found="false"

  for attempt in 1 2 3 4 5; do
    induce_deadlock_once
    output="$(run_function mysql deadlock-info '__job:local' 'false')"
    if has_min_rows "$output" 1; then
      validate "$output" --min-rows 1
      if assert_deadlock_info_content "$output"; then
        found="true"
        break
      fi
    fi
    sleep 1
  done

  if [ "$found" != "true" ]; then
    echo "deadlock-info did not produce valid deadlock attribution after 5 attempts" >&2
    return 1
  fi
}

verify_deadlock_info

echo "E2E checks passed for mysql." >&2
