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
    if norm(get_value(row, "database")):
        has_database = True

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
    if norm(get_value(row, "database")):
        has_database = True

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
}

verify_deadlock_info_no_deadlock
verify_deadlock_info

echo "E2E checks passed for ${MSSQL_VARIANT_LABEL}." >&2
