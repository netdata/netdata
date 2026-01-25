#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "mssql"
trap cleanup EXIT

MSSQL_PORT="$(reserve_port)"
write_env "MSSQL_PORT" "$MSSQL_PORT"
MSSQL_CONF="$WORKDIR/config/go.d/mssql.conf"
replace_in_file "$MSSQL_CONF" "127.0.0.1:1433" "127.0.0.1:${MSSQL_PORT}"

compose_up mssql
wait_healthy mssql 120
compose_run mssql-init

build_plugin
run_info mssql
run_top_queries mssql

DSN_SA_PREFIX="sqlserver://sa:Netdata123!@"
DSN_LIMITED_PREFIX="sqlserver://netdata_limited:Netdata123!@"

set_dsn_limited() {
  replace_in_file "$MSSQL_CONF" "$DSN_SA_PREFIX" "$DSN_LIMITED_PREFIX"
}

set_dsn_sa() {
  replace_in_file "$MSSQL_CONF" "$DSN_LIMITED_PREFIX" "$DSN_SA_PREFIX"
}

mssql_container_id() {
  "${COMPOSE[@]}" ps -q mssql
}

mssql_sqlcmd_path() {
  local cid
  cid="$(mssql_container_id)"
  if [ -z "$cid" ]; then
    echo "MSSQL container ID not found" >&2
    return 1
  fi
  docker exec -i "$cid" bash -lc 'if [ -x /opt/mssql-tools18/bin/sqlcmd ]; then echo /opt/mssql-tools18/bin/sqlcmd; elif [ -x /opt/mssql-tools/bin/sqlcmd ]; then echo /opt/mssql-tools/bin/sqlcmd; else exit 1; fi'
}

MSSQL_SQLCMD_PATH="$(mssql_sqlcmd_path)"
echo "Using sqlcmd path: $MSSQL_SQLCMD_PATH" >&2

mssql_exec_sa() {
  local sql="$1"
  local cid
  cid="$(mssql_container_id)"
  if [ -z "$cid" ]; then
    echo "MSSQL container ID not found" >&2
    return 1
  fi
  run docker exec -i "$cid" "$MSSQL_SQLCMD_PATH" -C -S localhost -U sa -P "Netdata123!" -d netdata -b -y 0 -Y 0 -Q "$sql"
}

induce_deadlock_once() {
  local tx1
  local tx2

  tx1="$(cat <<'SQL'
SET NOCOUNT ON;
SET LOCK_TIMEOUT 5000;
BEGIN TRAN;
UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1;
WAITFOR DELAY '00:00:01';
UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1;
COMMIT;
SQL
)"

  tx2="$(cat <<'SQL'
SET NOCOUNT ON;
SET LOCK_TIMEOUT 5000;
BEGIN TRAN;
UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1;
WAITFOR DELAY '00:00:01';
UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1;
COMMIT;
SQL
)"

  mssql_exec_sa "$tx1" &
  local pid1=$!
  mssql_exec_sa "$tx2" &
  local pid2=$!

  set +e
  wait "$pid1"
  wait "$pid2"
  set -e
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

waiting_rows = [row for row in data if str(get_value(row, "lock_status")).upper() == "WAITING"]
if any(norm(get_value(row, "lock_mode")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include lock_mode")

victim_counts = {}
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    victim_counts.setdefault(deadlock_id, 0)
    if str(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
PY
    return
  fi

  python - "$input" <<'PY'
import json
import re
import sys

path = sys.argv[1]
with open(path, "r") as fh:
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
        raise SystemExit("missing expected column: %s" % required)

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

waiting_rows = [row for row in data if str(get_value(row, "lock_status")).upper() == "WAITING"]
if any(norm(get_value(row, "lock_mode")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include lock_mode")

victim_counts = {}
for row in data:
    deadlock_id = norm(get_value(row, "deadlock_id"))
    if deadlock_id == "":
        raise SystemExit("deadlock_id missing from deadlock-info output")
    victim_counts.setdefault(deadlock_id, 0)
    if str(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit("deadlock_id %s has victim count %s, expected 1" % (deadlock_id, count))
PY
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

assert_deadlock_info_error_contains() {
  local input="$1"
  local expected_status="$2"
  local expected_substr="$3"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" "$expected_status" "$expected_substr" <<'PY'
import json
import sys

path = sys.argv[1]
expected_status = int(sys.argv[2])
expected = sys.argv[3].strip().lower()
with open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit(f"unexpected status value: {doc.get('status')!r}")

if status != expected_status:
    raise SystemExit(f"expected status {expected_status}, got {status}")

err = str(doc.get("errorMessage") or "").lower()
if expected not in err:
    raise SystemExit(f"expected errorMessage to contain {expected!r}, got {err!r}")
PY
    return
  fi

  python - "$input" "$expected_status" "$expected_substr" <<'PY'
import json
import sys

path = sys.argv[1]
expected_status = int(sys.argv[2])
expected = sys.argv[3].strip().lower()
with open(path, "r") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: %r" % (doc.get("status"),))

if status != expected_status:
    raise SystemExit("expected status %s, got %s" % (expected_status, status))

err = str(doc.get("errorMessage") or "").lower()
if expected not in err:
    raise SystemExit("expected errorMessage to contain %r, got %r" % (expected, err))
PY
}

verify_deadlock_info_no_deadlock() {
  local output

  set_dsn_sa
  output="$(run_function mssql deadlock-info '__job:local' 'false')"
  validate "$output"
  assert_deadlock_info_empty_success "$output"
}

verify_deadlock_info_limited_user() {
  local output

  set_dsn_limited
  output="$(run_function mssql deadlock-info '__job:local' 'false')"
  validate "$output"
  assert_deadlock_info_empty_success "$output"
  set_dsn_sa
}

verify_deadlock_info() {
  local attempt
  local output
  local found="false"

  set_dsn_sa
  for attempt in 1 2 3 4 5; do
    induce_deadlock_once
    output="$(run_function mssql deadlock-info '__job:local' 'false')"
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
verify_deadlock_info_limited_user
verify_deadlock_info

echo "E2E checks passed for mssql." >&2
