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

MSSQL_VARIANT_LABEL="${MSSQL_VARIANT:-mssql}"
if [ -n "${MSSQL_IMAGE:-}" ]; then
  write_env "MSSQL_IMAGE" "$MSSQL_IMAGE"
fi

compose_up mssql
wait_healthy mssql 120
compose_run mssql-init

build_plugin

MSSQL_JOB_RETRIES="${MSSQL_JOB_RETRIES:-6}"
MSSQL_JOB_RETRY_DELAY="${MSSQL_JOB_RETRY_DELAY:-5}"

mssql_is_no_jobs_started() {
  local input="$1"
  if [ ! -s "$input" ]; then
    return 1
  fi

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as fh:
        doc = json.load(fh)
except Exception:
    raise SystemExit(1)

status = doc.get("status")
msg = str(doc.get("errorMessage") or "").lower()
if status == 503 and "no jobs started for module" in msg:
    raise SystemExit(0)
raise SystemExit(1)
PY
    return $?
  fi

  python - "$input" <<'PY'
import json
import sys

path = sys.argv[1]
try:
    with open(path, "r") as fh:
        doc = json.load(fh)
except Exception:
    raise SystemExit(1)

status = doc.get("status")
msg = str(doc.get("errorMessage") or "").lower()
if status == 503 and "no jobs started for module" in msg:
    raise SystemExit(0)
raise SystemExit(1)
PY
}

run_mssql_info_with_retry() {
  local output="$WORKDIR/mssql-top-queries-info.json"
  local attempt=1

  while true; do
    if run "$WORKDIR/go.d.plugin" \
      --config-dir "$WORKDIR/config" \
      --function "mssql:top-queries" \
      --function-args info \
      > "$output"; then
      validate "$output"
      return 0
    fi

    if mssql_is_no_jobs_started "$output"; then
      if [ "$attempt" -ge "$MSSQL_JOB_RETRIES" ]; then
        echo "Timed out waiting for mssql jobs to start (info)" >&2
        return 1
      fi
      attempt=$((attempt + 1))
      sleep "$MSSQL_JOB_RETRY_DELAY"
      continue
    fi

    echo "Unexpected failure while running mssql top-queries info" >&2
    cat "$output" >&2
    return 1
  done
}

run_mssql_top_queries_with_retry() {
  local output="$WORKDIR/mssql-top-queries.json"
  local attempt=1

  while true; do
    if run "$WORKDIR/go.d.plugin" \
      --config-dir "$WORKDIR/config" \
      --function "mssql:top-queries" \
      --function-args __job:local \
      > "$output"; then
      validate "$output" --min-rows 1
      return 0
    fi

    if mssql_is_no_jobs_started "$output"; then
      if [ "$attempt" -ge "$MSSQL_JOB_RETRIES" ]; then
        echo "Timed out waiting for mssql jobs to start (top-queries)" >&2
        return 1
      fi
      attempt=$((attempt + 1))
      sleep "$MSSQL_JOB_RETRY_DELAY"
      continue
    fi

    echo "Unexpected failure while running mssql top-queries" >&2
    cat "$output" >&2
    return 1
  done
}

run_mssql_function_with_retry() {
  local method="$1"
  local args="${2:-__job:local}"
  local require_rows="${3:-true}"
  local output="$WORKDIR/mssql-${method}.json"
  local attempt=1

  while true; do
    if run "$WORKDIR/go.d.plugin" \
      --config-dir "$WORKDIR/config" \
      --function "mssql:${method}" \
      --function-args "$args" \
      > "$output"; then
      if [ "$require_rows" = "true" ]; then
        validate "$output" --min-rows 1
      else
        validate "$output"
      fi
      echo "$output"
      return 0
    fi

    if mssql_is_no_jobs_started "$output"; then
      if [ "$attempt" -ge "$MSSQL_JOB_RETRIES" ]; then
        echo "Timed out waiting for mssql jobs to start (${method})" >&2
        return 1
      fi
      attempt=$((attempt + 1))
      sleep "$MSSQL_JOB_RETRY_DELAY"
      continue
    fi

    echo "Unexpected failure while running mssql ${method}" >&2
    cat "$output" >&2
    return 1
  done
}

run_mssql_info_with_retry
run_mssql_top_queries_with_retry

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

mssql_sqlcmd_supports_c() {
  local cid
  cid="$(mssql_container_id)"
  if [ -z "$cid" ]; then
    return 1
  fi
  set +e
  local help
  help="$(docker exec -i "$cid" "$MSSQL_SQLCMD_PATH" -? 2>&1)"
  local status=$?
  set -e
  if [ $status -ne 0 ] && [ -z "$help" ]; then
    return 1
  fi
  echo "$help" | grep -q " -C"
}

MSSQL_SQLCMD_CFLAG=()
case "$MSSQL_SQLCMD_PATH" in
  *mssql-tools18*)
    MSSQL_SQLCMD_CFLAG=(-C)
    ;;
  *)
    if mssql_sqlcmd_supports_c; then
      MSSQL_SQLCMD_CFLAG=(-C)
    fi
    ;;
esac

mssql_exec_sa() {
  local sql="$1"
  local cid
  cid="$(mssql_container_id)"
  if [ -z "$cid" ]; then
    echo "MSSQL container ID not found" >&2
    return 1
  fi
  run docker exec -i "$cid" "$MSSQL_SQLCMD_PATH" "${MSSQL_SQLCMD_CFLAG[@]}" -S localhost -U sa -P "Netdata123!" -d netdata -b -y 0 -Y 0 -Q "$sql"
}

mssql_exec_sa_allow_error() {
  local sql="$1"
  local cid
  cid="$(mssql_container_id)"
  if [ -z "$cid" ]; then
    echo "MSSQL container ID not found" >&2
    return 1
  fi
  set +e
  docker exec -i "$cid" "$MSSQL_SQLCMD_PATH" "${MSSQL_SQLCMD_CFLAG[@]}" -S localhost -U sa -P "Netdata123!" -d netdata -b -y 0 -Y 0 -Q "$sql" >/dev/null 2>&1
  set -e
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

waiting_rows = [row for row in data if str(get_value(row, "lock_status")).upper() == "WAITING"]
if any(norm(get_value(row, "lock_mode")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include lock_mode")
if any(norm(get_value(row, "wait_resource")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include wait_resource")

lock_mode_re = re.compile(r"^[A-Za-z0-9_-]+$")
if any(not lock_mode_re.match(norm(get_value(row, "lock_mode"))) for row in waiting_rows):
    raise SystemExit("WAITING rows must include a valid lock_mode")

victim_counts = {}
expected_db = "netdata"
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
    if str(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    db_val = norm(get_value(row, "database")).lower()
    if db_val:
        has_database = True
        if db_val != expected_db:
            raise SystemExit(f"unexpected database value {db_val!r}, expected {expected_db!r}")

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit(f"deadlock_id {deadlock_id} has victim count {count}, expected 1")
if not has_database:
    raise SystemExit("expected at least one row with database populated")
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
if any(norm(get_value(row, "wait_resource")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include wait_resource")

lock_mode_re = re.compile(r"^[A-Za-z0-9_-]+$")
if any(not lock_mode_re.match(norm(get_value(row, "lock_mode"))) for row in waiting_rows):
    raise SystemExit("WAITING rows must include a valid lock_mode")

victim_counts = {}
expected_db = "netdata"
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
    if str(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    db_val = norm(get_value(row, "database")).lower()
    if db_val:
        has_database = True
        if db_val != expected_db:
            raise SystemExit("unexpected database value %r, expected %r" % (db_val, expected_db))

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit("deadlock_id %s has victim count %s, expected 1" % (deadlock_id, count))
if not has_database:
    raise SystemExit("expected at least one row with database populated")
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

data = doc.get("data") or []
if len(data) == 0:
    raise SystemExit(0)

query_idx = field_to_idx.get("query_text", None)
if query_idx is None:
    raise SystemExit(f"expected no rows, got {len(data)}")

for row in data:
    if query_idx >= len(row):
        continue
    query = str(row[query_idx]).lower()
    if "deadlock_a" in query or "deadlock_b" in query:
        raise SystemExit(f"unexpected deadlock rows for test tables, got {len(data)} rows")
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

data = doc.get("data") or []
if len(data) == 0:
    raise SystemExit(0)

query_idx = field_to_idx.get("query_text", None)
if query_idx is None:
    raise SystemExit("expected no rows, got %s" % len(data))

for row in data:
    if query_idx >= len(row):
        continue
    query = str(row[query_idx]).lower()
    if "deadlock_a" in query or "deadlock_b" in query:
        raise SystemExit("unexpected deadlock rows for test tables, got %s rows" % len(data))
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

assert_error_info_not_enabled() {
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

if status < 400:
    raise SystemExit(f"expected error status, got {status}")

err = str(doc.get("errorMessage") or "").lower()
if "not enabled" not in err:
    raise SystemExit(f"expected errorMessage to contain 'not enabled', got {err!r}")
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

if status < 400:
    raise SystemExit("expected error status, got %s" % status)

err = str(doc.get("errorMessage") or "").lower()
if "not enabled" not in err:
    raise SystemExit("expected errorMessage to contain 'not enabled', got %r" % err)
PY
}

assert_error_info_has_errors() {
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

for required in ("errorNumber", "errorMessage", "query"):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
if not data:
    raise SystemExit("error-info returned no rows")

num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]
query_idx = field_to_idx["query"]

# Error categories to verify:
# 208 - Invalid object name (table not found)
# 102 - Syntax error
# 2627 - Duplicate key / unique constraint violation
# 245 - Data type conversion error
# 8134 - Division by zero
error_categories = {
    "table_not_found": {"patterns": ["invalid object name", "netdata_error_map_e2e"], "found": False},
    "syntax_error": {"patterns": ["incorrect syntax", "form"], "found": False},
    "duplicate_key": {"patterns": ["duplicate key", "unique", "primary key", "error_test"], "found": False},
    "data_type": {"patterns": ["conversion failed", "converting"], "found": False},
    "divide_by_zero": {"patterns": ["divide by zero"], "found": False},
}

for row in data:
    if num_idx >= len(row) or row[num_idx] is None:
        continue
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) else ""
    query = str(row[query_idx]).lower() if query_idx < len(row) else ""
    combined = msg + " " + query
    for cat, info in error_categories.items():
        if info["found"]:
            continue
        for pattern in info["patterns"]:
            if pattern in combined:
                info["found"] = True
                break

missing = [cat for cat, info in error_categories.items() if not info["found"]]
if missing:
    raise SystemExit(f"error-info missing error categories: {', '.join(missing)}")
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

for required in ("errorNumber", "errorMessage", "query"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
if not data:
    raise SystemExit("error-info returned no rows")

num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]
query_idx = field_to_idx["query"]

# Error categories to verify:
# 208 - Invalid object name (table not found)
# 102 - Syntax error
# 2627 - Duplicate key / unique constraint violation
# 245 - Data type conversion error
# 8134 - Division by zero
error_categories = {
    "table_not_found": {"patterns": ["invalid object name", "netdata_error_map_e2e"], "found": False},
    "syntax_error": {"patterns": ["incorrect syntax", "form"], "found": False},
    "duplicate_key": {"patterns": ["duplicate key", "unique", "primary key", "error_test"], "found": False},
    "data_type": {"patterns": ["conversion failed", "converting"], "found": False},
    "divide_by_zero": {"patterns": ["divide by zero"], "found": False},
}

for row in data:
    if num_idx >= len(row) or row[num_idx] is None:
        continue
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) else ""
    query = str(row[query_idx]).lower() if query_idx < len(row) else ""
    combined = msg + " " + query
    for cat, info in error_categories.items():
        if info["found"]:
            continue
        for pattern in info["patterns"]:
            if pattern in combined:
                info["found"] = True
                break

missing = [cat for cat, info in error_categories.items() if not info["found"]]
if missing:
    raise SystemExit("error-info missing error categories: %s" % ", ".join(missing))
PY
}

assert_top_queries_error_attribution_not_enabled() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
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

if "errorAttribution" not in field_to_idx:
    raise SystemExit("missing expected column: errorAttribution")

data = doc.get("data") or []
idx = field_to_idx["errorAttribution"]
for row in data:
    if idx >= len(row):
        continue
    if str(row[idx]) != "not_enabled":
        raise SystemExit(f"expected errorAttribution 'not_enabled', got {row[idx]!r}")
PY
    return
  fi

  python - "$input" <<'PY'
import json
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

if "errorAttribution" not in field_to_idx:
    raise SystemExit("missing expected column: errorAttribution")

data = doc.get("data") or []
idx = field_to_idx["errorAttribution"]
for row in data:
    if idx >= len(row):
        continue
    if str(row[idx]) != "not_enabled":
        raise SystemExit("expected errorAttribution 'not_enabled', got %r" % row[idx])
PY
}

assert_top_queries_error_attribution_active() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
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

for required in ("errorAttribution",):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]

for row in data:
    if status_idx >= len(row):
        continue
    status = str(row[status_idx])
    if status not in ("enabled", "no_data"):
        raise SystemExit(f"unexpected errorAttribution status {status!r}")
PY
    return
  fi

  python - "$input" <<'PY'
import json
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

for required in ("errorAttribution",):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]

for row in data:
    if status_idx >= len(row):
        continue
    status = str(row[status_idx])
    if status not in ("enabled", "no_data"):
        raise SystemExit("unexpected errorAttribution status %r" % status)
PY
}

assert_top_queries_error_attribution_mapped() {
  local top_queries="$1"
  local error_info="$2"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$top_queries" "$error_info" <<'PY'
import json
import sys

top_path = sys.argv[1]
err_path = sys.argv[2]

with open(err_path, "r", encoding="utf-8") as fh:
    err_doc = json.load(fh)

err_cols = err_doc.get("columns") or {}
err_idx = {}
if isinstance(err_cols, dict):
    for field, col in err_cols.items():
        if not isinstance(col, dict):
            continue
        try:
            err_idx[field] = int(col.get("index"))
        except (TypeError, ValueError):
            continue
else:
    for idx, col in enumerate(err_cols):
        if not isinstance(col, dict):
            continue
        field = col.get("field")
        if field:
            err_idx[field] = idx

for required in ("errorMessage", "errorNumber", "query", "queryHash"):
    if required not in err_idx:
        raise SystemExit(f"missing expected error-info column: {required}")

def normalize(text: str) -> str:
    return " ".join(text.split()).strip().rstrip(";").strip()

error_rows = err_doc.get("data") or []
candidates = []
for row in error_rows:
    msg = str(row[err_idx["errorMessage"]]).lower() if err_idx["errorMessage"] < len(row) else ""
    err_no = row[err_idx["errorNumber"]] if err_idx["errorNumber"] < len(row) else None
    query = str(row[err_idx["query"]]).lower() if err_idx["query"] < len(row) else ""
    qh = row[err_idx["queryHash"]] if err_idx["queryHash"] < len(row) else None
    try:
        err_no_val = int(err_no)
    except Exception:
        continue
    if err_no_val != 208:
        continue
    if "invalid object name" in msg and "netdata_error_map_e2e" in query:
        candidates.append((str(qh) if qh else "", normalize(query)))

if not candidates:
    raise SystemExit("no error-info row contained invalid object name for netdata_error_map_e2e")

with open(top_path, "r", encoding="utf-8") as fh:
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

for required in ("query", "queryHash", "errorAttribution", "errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]
num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]
hash_idx = field_to_idx["queryHash"]

matched = False
for row in data:
    if status_idx >= len(row):
        continue
    if hash_idx >= len(row):
        continue
    status = str(row[status_idx]) if status_idx < len(row) else ""
    if status != "enabled":
        continue
    err_no = row[num_idx] if num_idx < len(row) else None
    try:
        err_no_val = int(err_no)
    except Exception:
        continue
    if err_no_val != 208:
        continue
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) and row[msg_idx] is not None else ""
    if "invalid object name" not in msg:
        continue
    row_hash = str(row[hash_idx]) if hash_idx < len(row) and row[hash_idx] is not None else ""
    row_query = normalize(str(row[field_to_idx["query"]]).lower()) if field_to_idx["query"] < len(row) else ""
    for cand_hash, cand_query in candidates:
        if cand_hash and row_hash == cand_hash:
            matched = True
            break
        if cand_query and row_query == cand_query:
            matched = True
            break
    if matched:
        break

if not matched:
    raise SystemExit("no top-queries row had enabled error attribution for netdata_error_map_e2e")
PY
    return
  fi

  python - "$top_queries" "$error_info" <<'PY'
import json
import sys

top_path = sys.argv[1]
err_path = sys.argv[2]

with open(err_path, "r") as fh:
    err_doc = json.load(fh)

err_cols = err_doc.get("columns") or {}
err_idx = {}
if isinstance(err_cols, dict):
    for field, col in err_cols.items():
        if not isinstance(col, dict):
            continue
        try:
            err_idx[field] = int(col.get("index"))
        except (TypeError, ValueError):
            continue
else:
    for idx, col in enumerate(err_cols):
        if not isinstance(col, dict):
            continue
        field = col.get("field")
        if field:
            err_idx[field] = idx

for required in ("errorMessage", "errorNumber", "query", "queryHash"):
    if required not in err_idx:
        raise SystemExit("missing expected error-info column: %s" % required)

def normalize(text):
    return " ".join(text.split()).strip().rstrip(";").strip()

error_rows = err_doc.get("data") or []
candidates = []
for row in error_rows:
    msg = str(row[err_idx["errorMessage"]]).lower() if err_idx["errorMessage"] < len(row) else ""
    err_no = row[err_idx["errorNumber"]] if err_idx["errorNumber"] < len(row) else None
    query = str(row[err_idx["query"]]).lower() if err_idx["query"] < len(row) else ""
    qh = row[err_idx["queryHash"]] if err_idx["queryHash"] < len(row) else None
    try:
        err_no_val = int(err_no)
    except Exception:
        continue
    if err_no_val != 208:
        continue
    if "invalid object name" in msg and "netdata_error_map_e2e" in query:
        candidates.append((str(qh) if qh else "", normalize(query)))

if not candidates:
    raise SystemExit("no error-info row contained invalid object name for netdata_error_map_e2e")

with open(top_path, "r") as fh:
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

for required in ("query", "queryHash", "errorAttribution", "errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]
num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]
hash_idx = field_to_idx["queryHash"]

matched = False
for row in data:
    if status_idx >= len(row):
        continue
    if hash_idx >= len(row):
        continue
    status = str(row[status_idx]) if status_idx < len(row) else ""
    if status != "enabled":
        continue
    err_no = row[num_idx] if num_idx < len(row) else None
    try:
        err_no_val = int(err_no)
    except Exception:
        continue
    if err_no_val != 208:
        continue
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) and row[msg_idx] is not None else ""
    if "invalid object name" not in msg:
        continue
    row_hash = str(row[hash_idx]) if hash_idx < len(row) and row[hash_idx] is not None else ""
    row_query = normalize(str(row[field_to_idx["query"]]).lower()) if field_to_idx["query"] < len(row) else ""
    for cand_hash, cand_query in candidates:
        if cand_hash and row_hash == cand_hash:
            matched = True
            break
        if cand_query and row_query == cand_query:
            matched = True
            break
    if matched:
        break

if not matched:
    raise SystemExit("no top-queries row had enabled error attribution for netdata_error_map_e2e")
PY
}

assert_top_queries_plan_ops() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import json
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

for required in ("query", "hashMatch", "sorts"):
    if required not in field_to_idx:
        raise SystemExit(f"missing expected column: {required}")

data = doc.get("data") or []
query_idx = field_to_idx["query"]
hash_idx = field_to_idx["hashMatch"]
sort_idx = field_to_idx["sorts"]

matched = False
for row in data:
    if query_idx >= len(row):
        continue
    query = str(row[query_idx]).lower()
    if "join" not in query or "sample" not in query:
        continue
    hash_val = row[hash_idx] if hash_idx < len(row) else 0
    sort_val = row[sort_idx] if sort_idx < len(row) else 0
    try:
        hash_val = int(hash_val)
    except Exception:
        hash_val = 0
    try:
        sort_val = int(sort_val)
    except Exception:
        sort_val = 0
    if hash_val > 0 and sort_val > 0:
        matched = True
        break

if not matched:
    raise SystemExit("no top-queries row had hashMatch and sorts counts for the join query")
PY
    return
  fi

  python - "$input" <<'PY'
import json
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

for required in ("query", "hashMatch", "sorts"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
query_idx = field_to_idx["query"]
hash_idx = field_to_idx["hashMatch"]
sort_idx = field_to_idx["sorts"]

matched = False
for row in data:
    if query_idx >= len(row):
        continue
    query = str(row[query_idx]).lower()
    if "join" not in query or "sample" not in query:
        continue
    hash_val = row[hash_idx] if hash_idx < len(row) else 0
    sort_val = row[sort_idx] if sort_idx < len(row) else 0
    try:
        hash_val = int(hash_val)
    except Exception:
        hash_val = 0
    try:
        sort_val = int(sort_val)
    except Exception:
        sort_val = 0
    if hash_val > 0 and sort_val > 0:
        matched = True
        break

if not matched:
    raise SystemExit("no top-queries row had hashMatch and sorts counts for the join query")
PY
}

verify_deadlock_info_no_deadlock() {
  local output

  output="$(run_mssql_function_with_retry deadlock-info '__job:local' 'false')"
  validate "$output"
  assert_deadlock_info_empty_success "$output"
}

verify_deadlock_info() {
  local attempt
  local output
  local found="false"

  for attempt in 1 2 3 4 5; do
    induce_deadlock_once
    output="$(run_mssql_function_with_retry deadlock-info '__job:local' 'false')"
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

  # Verify column visibility rules
  assert_column_visibility "$output" "deadlock-info"
}

verify_deadlock_info_no_deadlock
verify_deadlock_info

assert_top_queries_error_attribution_not_enabled "$WORKDIR/mssql-top-queries.json"

error_output="$(run_mssql_function_with_retry error-info '__job:local' 'false')"
assert_error_info_not_enabled "$error_output"

mssql_exec_sa "IF EXISTS (SELECT 1 FROM sys.server_event_sessions WHERE name = 'netdata_errors') DROP EVENT SESSION [netdata_errors] ON SERVER;"
mssql_exec_sa "CREATE EVENT SESSION [netdata_errors] ON SERVER ADD EVENT sqlserver.error_reported(ACTION(sqlserver.sql_text, sqlserver.query_hash)) ADD TARGET package0.ring_buffer;"
mssql_exec_sa "ALTER EVENT SESSION [netdata_errors] ON SERVER STATE = START;"
mssql_exec_sa "ALTER DATABASE netdata SET QUERY_STORE (QUERY_CAPTURE_MODE = ALL, OPERATION_MODE = READ_WRITE);"

mssql_exec_sa "IF OBJECT_ID('dbo.netdata_error_map_e2e', 'U') IS NOT NULL DROP TABLE dbo.netdata_error_map_e2e;"
mssql_exec_sa "CREATE TABLE dbo.netdata_error_map_e2e (id int NOT NULL PRIMARY KEY);"
mssql_exec_sa "INSERT INTO dbo.netdata_error_map_e2e (id) VALUES (1), (2), (3);"

for _ in 1 2 3 4 5 6 7 8 9 10; do
  mssql_exec_sa "SELECT COUNT(*) FROM dbo.netdata_error_map_e2e;"
done

mssql_exec_sa "DROP TABLE dbo.netdata_error_map_e2e;"

# Generate errors for multiple categories:
# 1. Table not found (error 208)
for _ in 1 2 3; do
  mssql_exec_sa_allow_error "SELECT COUNT(*) FROM dbo.netdata_error_map_e2e;"
done

# 2. Syntax error (error 102)
for _ in 1 2 3; do
  mssql_exec_sa_allow_error "SELECT * FORM dbo.sample;"
done

# 3. Duplicate key / constraint violation (error 2627)
for _ in 1 2 3; do
  mssql_exec_sa_allow_error "INSERT INTO dbo.error_test (id, unique_col, int_col) VALUES (1, 'new_value', 200);"
  mssql_exec_sa_allow_error "INSERT INTO dbo.error_test (id, unique_col, int_col) VALUES (99, 'existing_value', 300);"
done

# 4. Data type conversion error (error 245)
for _ in 1 2 3; do
  mssql_exec_sa_allow_error "SELECT CAST('not_a_number' AS INT);"
done

# 5. Division by zero (error 8134)
for _ in 1 2 3; do
  mssql_exec_sa_allow_error "SELECT 1/0;"
done

for _ in 1 2 3 4 5; do
  mssql_exec_sa "SET NOCOUNT ON; SELECT a.id, b.name FROM dbo.sample a JOIN dbo.sample b ON a.id = b.id ORDER BY a.value + b.value DESC OPTION (HASH JOIN);"
done

mssql_exec_sa "EXEC sys.sp_query_store_flush_db;"
sleep 2

error_output="$(run_mssql_function_with_retry error-info '__job:local' 'true')"
assert_error_info_has_errors "$error_output"
assert_column_visibility "$error_output" "error-info"
assert_unique_key_populated "$error_output" "error-info"

run_mssql_top_queries_with_retry
assert_top_queries_error_attribution_active "$WORKDIR/mssql-top-queries.json"
assert_top_queries_error_attribution_mapped "$WORKDIR/mssql-top-queries.json" "$WORKDIR/mssql-error-info.json"
assert_top_queries_plan_ops "$WORKDIR/mssql-top-queries.json"
assert_column_visibility "$WORKDIR/mssql-top-queries.json" "top-queries"

echo "E2E checks passed for ${MSSQL_VARIANT_LABEL}." >&2
