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

MYSQL_VARIANT_LABEL="${MYSQL_VARIANT:-mysql}"
if [ -n "${MYSQL_IMAGE:-}" ]; then
  write_env "MYSQL_IMAGE" "$MYSQL_IMAGE"
fi

compose_up mysql
MYSQL_HEALTH_TIMEOUT="${MYSQL_HEALTH_TIMEOUT:-180}"
wait_healthy mysql "$MYSQL_HEALTH_TIMEOUT"

build_plugin
run_info mysql
run_top_queries mysql

mysql_container_id() {
  "${COMPOSE[@]}" ps -q mysql
}

mysql_client_path() {
  local cid
  cid="$(mysql_container_id)"
  if [ -z "$cid" ]; then
    echo "MySQL container ID not found" >&2
    return 1
  fi
  docker exec -i "$cid" sh -lc 'command -v mysql || command -v mariadb'
}

MYSQL_CLIENT_PATH="$(mysql_client_path)"
echo "Using mysql client: $MYSQL_CLIENT_PATH" >&2

mysql_exec_root() {
  local sql="$1"
  local cid
  cid="$(mysql_container_id)"
  if [ -z "$cid" ]; then
    echo "MySQL container ID not found" >&2
    return 1
  fi
  run docker exec -i "$cid" "$MYSQL_CLIENT_PATH" -uroot -prootpw netdata -e "$sql"
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

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit(f"unexpected status value: {doc.get('status')!r}")

if status != 200:
    raise SystemExit(f"expected status 200, got {status}")

if doc.get("errorMessage"):
    raise SystemExit(f"unexpected errorMessage on status 200: {doc.get('errorMessage')!r}")

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

for required in ("row_id", "deadlock_id", "process_id", "is_victim", "lock_mode", "lock_status", "query_text", "wait_resource", "database"):
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
if any(norm(get_value(row, "wait_resource")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include wait_resource")

lock_mode_re = re.compile(r"^[A-Za-z0-9_-]+$")
if any(not lock_mode_re.match(norm(get_value(row, "lock_mode"))) for row in waiting_rows):
    raise SystemExit("WAITING rows must include a valid lock_mode")

victim_counts = {}
has_database = False
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    process_id = norm(get_value(row, "process_id"))
    if process_id == "":
        raise SystemExit("process_id missing from deadlock-info output")
    row_id = norm(get_value(row, "row_id"))
    if row_id != f"{deadlock_id}:{process_id}":
        raise SystemExit(f"row_id {row_id} does not match deadlock_id/process_id")
    victim_counts.setdefault(deadlock_id, 0)
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    if norm(get_value(row, "database")):
        has_database = True

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
if not has_database:
    raise SystemExit("expected at least one row with database populated")
PY
  else
    python - "$input" <<'PY'
import json
import re
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: %r" % (doc.get("status"),))

if status != 200:
    raise SystemExit("expected status 200, got %s" % status)

if doc.get("errorMessage"):
    raise SystemExit("unexpected errorMessage on status 200: %r" % (doc.get("errorMessage"),))

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

for required in ("row_id", "deadlock_id", "process_id", "is_victim", "lock_mode", "lock_status", "query_text", "wait_resource", "database"):
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
if any(norm(get_value(row, "wait_resource")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include wait_resource")

lock_mode_re = re.compile(r"^[A-Za-z0-9_-]+$")
if any(not lock_mode_re.match(norm(get_value(row, "lock_mode"))) for row in waiting_rows):
    raise SystemExit("WAITING rows must include a valid lock_mode")

victim_counts = {}
has_database = False
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    process_id = norm(get_value(row, "process_id"))
    if process_id == "":
        raise SystemExit("process_id missing from deadlock-info output")
    row_id = norm(get_value(row, "row_id"))
    if row_id != "%s:%s" % (deadlock_id, process_id):
        raise SystemExit("row_id %s does not match deadlock_id/process_id" % row_id)
    victim_counts.setdefault(deadlock_id, 0)
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    if norm(get_value(row, "database")):
        has_database = True

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
if not has_database:
    raise SystemExit("expected at least one row with database populated")
PY
  fi
}

assert_deadlock_info_empty_success() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit(f"unexpected status value: {doc.get('status')!r}")

if status != 200:
    raise SystemExit(f"expected status 200, got {status}")

if doc.get("errorMessage"):
    raise SystemExit(f"unexpected errorMessage on status 200: {doc.get('errorMessage')!r}")

data = doc.get("data") or []
if len(data) != 0:
    raise SystemExit(f"expected no rows, got {len(data)}")
PY
    return
  fi

  python - "$input" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, "r") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: %r" % (doc.get("status"),))

if status != 200:
    raise SystemExit("expected status 200, got %s" % status)

if doc.get("errorMessage"):
    raise SystemExit("unexpected errorMessage on status 200: %r" % (doc.get("errorMessage"),))

data = doc.get("data") or []
if len(data) != 0:
    raise SystemExit("expected no rows, got %s" % len(data))
PY
}

verify_deadlock_info_no_deadlock() {
  local output

  output="$(run_function mysql deadlock-info '__job:local' 'false')"
  validate "$output"
  assert_deadlock_info_empty_success "$output"
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

verify_deadlock_info_no_deadlock
verify_deadlock_info

echo "E2E checks passed for ${MYSQL_VARIANT_LABEL}." >&2
