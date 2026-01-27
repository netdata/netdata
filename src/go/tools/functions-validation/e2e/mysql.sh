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

assert_top_queries_error_columns() {
  local input="$1"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)
columns = doc.get("columns") or {}
required = {"errorAttribution", "errorNumber", "sqlState", "errorMessage"}

found = set()
if isinstance(columns, dict):
    for key in columns.keys():
        found.add(key)
else:
    for col in columns:
        if isinstance(col, dict):
            field = col.get("field")
            if field:
                found.add(field)

missing = sorted(required - found)
if missing:
    raise SystemExit("missing top-queries error columns: {}".format(missing))
PY
    return
  fi
  python - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with open(path, "r") as fh:
    doc = json.load(fh)
columns = doc.get("columns") or {}
required = {"errorAttribution", "errorNumber", "sqlState", "errorMessage"}

found = set()
if isinstance(columns, dict):
    for key in columns.keys():
        found.add(key)
else:
    for col in columns:
        if isinstance(col, dict):
            field = col.get("field")
            if field:
                found.add(field)

missing = sorted(required - found)
if missing:
    raise SystemExit("missing top-queries error columns: %s" % missing)
PY
}

assert_top_queries_error_columns "$WORKDIR/mysql-top-queries.json"

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

mysql_query_root() {
  local sql="$1"
  local cid
  cid="$(mysql_container_id)"
  if [ -z "$cid" ]; then
    echo "MySQL container ID not found" >&2
    return 1
  fi
  run docker exec -i "$cid" "$MYSQL_CLIENT_PATH" -uroot -prootpw -N -s netdata -e "$sql"
}

mysql_exec_root_allow_error() {
  local sql="$1"
  local cid
  cid="$(mysql_container_id)"
  if [ -z "$cid" ]; then
    echo "MySQL container ID not found" >&2
    return 1
  fi
  set +e
  docker exec -i "$cid" "$MYSQL_CLIENT_PATH" -uroot -prootpw netdata -e "$sql" >/dev/null 2>&1
  set -e
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
import io
import json
import re
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: {!r}".format(doc.get("status")))

if status != 200:
    raise SystemExit("expected status 200, got {}".format(status))

if doc.get("errorMessage"):
    raise SystemExit("unexpected errorMessage on status 200: {!r}".format(doc.get("errorMessage")))

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
        raise SystemExit("missing expected column: {}".format(required))

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
if any(norm(get_value(row, "wait_resource")) == "" for row in waiting_rows):
    raise SystemExit("WAITING rows must include wait_resource")

lock_mode_re = re.compile(r"^[A-Za-z0-9_-]+$")
if any(norm(get_value(row, "lock_mode")) != "" and not lock_mode_re.match(norm(get_value(row, "lock_mode"))) for row in waiting_rows):
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
    if row_id != "{}:{}".format(deadlock_id, process_id):
        raise SystemExit("row_id {} does not match deadlock_id/process_id".format(row_id))
    victim_counts.setdefault(deadlock_id, 0)
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    db_val = norm(get_value(row, "database")).lower()
    if db_val:
        has_database = True
        if db_val != expected_db:
            raise SystemExit("unexpected database value {!r}, expected {!r}".format(db_val, expected_db))

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit("deadlock_id {} has victim count {}, expected 1".format(deadlock_id, count))
if not has_database:
    raise SystemExit("expected at least one row with database populated")
PY
  else
    python - "$input" <<'PY'
import io
import json
import re
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
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
        raise SystemExit("missing expected column: {}".format(required))

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
    if norm(get_value(row, "is_victim")).lower() == "true":
        victim_counts[deadlock_id] += 1
    db_val = norm(get_value(row, "database")).lower()
    if db_val:
        has_database = True
        if db_val != expected_db:
            raise SystemExit("unexpected database value %r, expected %r" % (db_val, expected_db))

for deadlock_id, count in victim_counts.items():
    if count != 1:
        raise SystemExit("deadlock_id {} has victim count {}, expected 1".format(deadlock_id, count))
if not has_database:
    raise SystemExit("expected at least one row with database populated")
PY
  fi
}

assert_deadlock_info_empty_success() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: {!r}".format(doc.get("status")))

if status != 200:
    raise SystemExit("expected status 200, got {}".format(status))

if doc.get("errorMessage"):
    raise SystemExit("unexpected errorMessage on status 200: {!r}".format(doc.get("errorMessage")))

data = doc.get("data") or []
if len(data) != 0:
    raise SystemExit("expected no rows, got {}".format(len(data)))
PY
    return
  fi

  python - "$input" <<'PY'
import io
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

assert_error_info_not_enabled() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: {!r}".format(doc.get("status")))

if status < 400:
    raise SystemExit("expected error status, got {}".format(status))

err = str(doc.get("errorMessage") or "").lower()
if "not enabled" not in err:
    raise SystemExit("expected errorMessage to contain 'not enabled', got {!r}".format(err))
PY
    return
  fi

  python - "$input" <<'PY'
import io
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
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
    doc = json.load(fh)

try:
    status = int(doc.get("status"))
except (TypeError, ValueError):
    raise SystemExit("unexpected status value: {!r}".format(doc.get("status")))

if status != 200:
    raise SystemExit("expected status 200, got {}".format(status))

if doc.get("errorMessage"):
    raise SystemExit("unexpected errorMessage on status 200: {!r}".format(doc.get("errorMessage")))

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

for required in ("errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: {}".format(required))

data = doc.get("data") or []
if not data:
    raise SystemExit("error-info returned no rows")

err_idx = field_to_idx["errorMessage"]
num_idx = field_to_idx["errorNumber"]
def normalize(val):
    return "" if val is None else str(val)

# Error categories to verify:
# 1146 - Table doesn't exist (missing_table)
# 1062 - Duplicate key (constraint violation)
# 1064 - Syntax error (should be captured with synthetic digest)
error_categories = {
    "table_not_found": {"patterns": ["missing_table", "doesn't exist", "does not exist"], "error_nums": [1146], "found": False},
    "duplicate_key": {"patterns": ["duplicate", "primary", "unique"], "error_nums": [1062], "found": False},
    "syntax_error": {"patterns": ["syntax", "error in your sql"], "error_nums": [1064], "found": False},
}

for row in data:
    if num_idx >= len(row) or row[num_idx] is None:
        continue
    err_num = row[num_idx]
    msg = normalize(row[err_idx]).lower()
    for cat, info in error_categories.items():
        if info["found"]:
            continue
        # Check by error number first
        if err_num in info.get("error_nums", []):
            info["found"] = True
            continue
        # Fallback to pattern matching
        for pattern in info["patterns"]:
            if pattern in msg:
                info["found"] = True
                break

missing = [cat for cat, info in error_categories.items() if not info["found"]]
if missing:
    raise SystemExit("error-info missing error categories: {}".format(", ".join(missing)))
PY
    return
  fi

  python - "$input" <<'PY'
import io
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

for required in ("errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
if not data:
    raise SystemExit("error-info returned no rows")

err_idx = field_to_idx["errorMessage"]
num_idx = field_to_idx["errorNumber"]
def normalize(val):
    return "" if val is None else str(val)

# Error categories to verify:
# 1146 - Table doesn't exist (missing_table)
# 1062 - Duplicate key (constraint violation)
# 1064 - Syntax error (should be captured with synthetic digest)
error_categories = {
    "table_not_found": {"patterns": ["missing_table", "doesn't exist", "does not exist"], "error_nums": [1146], "found": False},
    "duplicate_key": {"patterns": ["duplicate", "primary", "unique"], "error_nums": [1062], "found": False},
    "syntax_error": {"patterns": ["syntax", "error in your sql"], "error_nums": [1064], "found": False},
}

for row in data:
    if num_idx >= len(row) or row[num_idx] is None:
        continue
    err_num = row[num_idx]
    msg = normalize(row[err_idx]).lower()
    for cat, info in error_categories.items():
        if info["found"]:
            continue
        if err_num in info.get("error_nums", []):
            info["found"] = True
            continue
        for pattern in info["patterns"]:
            if pattern in msg:
                info["found"] = True
                break

missing = [cat for cat, info in error_categories.items() if not info["found"]]
if missing:
    raise SystemExit("error-info missing error categories: %s" % ", ".join(missing))
PY
}

assert_top_queries_error_attribution_enabled() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
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

for required in ("errorAttribution", "errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: {}".format(required))

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]
num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]

matched = False
for row in data:
    if status_idx >= len(row):
        continue
    if str(row[status_idx]) != "enabled":
        continue
    num = row[num_idx] if num_idx < len(row) else None
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) else ""
    if num is not None and "missing_table" in msg:
        matched = True
        break

if not matched:
    raise SystemExit("no top-queries row had enabled error attribution for missing_table")
PY
    return
  fi

  python - "$input" <<'PY'
import io
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

for required in ("errorAttribution", "errorNumber", "errorMessage"):
    if required not in field_to_idx:
        raise SystemExit("missing expected column: %s" % required)

data = doc.get("data") or []
status_idx = field_to_idx["errorAttribution"]
num_idx = field_to_idx["errorNumber"]
msg_idx = field_to_idx["errorMessage"]

matched = False
for row in data:
    if status_idx >= len(row):
        continue
    if str(row[status_idx]) != "enabled":
        continue
    num = row[num_idx] if num_idx < len(row) else None
    msg = str(row[msg_idx]).lower() if msg_idx < len(row) else ""
    if num is not None and "missing_table" in msg:
        matched = True
        break

if not matched:
    raise SystemExit("no top-queries row had enabled error attribution for missing_table")
PY
}

assert_top_queries_error_attribution_not_enabled() {
  local input="$1"

  if command -v python3 >/dev/null 2>&1; then
    python3 - "$input" <<'PY'
import io
import json
import sys

path = sys.argv[1]
with io.open(path, "r", encoding="utf-8") as fh:
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
        raise SystemExit("expected errorAttribution 'not_enabled', got {!r}".format(row[idx]))
PY
    return
  fi

  python - "$input" <<'PY'
import io
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

capture_statement_history_states() {
  local output
  output="$(mysql_query_root "
SELECT
  COALESCE(MAX(CASE WHEN NAME = 'events_statements_history_long' THEN ENABLED END), 'NO') AS history_long,
  COALESCE(MAX(CASE WHEN NAME = 'events_statements_history' THEN ENABLED END), 'NO') AS history,
  COALESCE(MAX(CASE WHEN NAME = 'events_statements_current' THEN ENABLED END), 'NO') AS history_current
FROM performance_schema.setup_consumers
WHERE NAME IN ('events_statements_history_long','events_statements_history','events_statements_current');")"
  local history_long
  local history
  local history_current
  IFS=$'\t' read -r history_long history history_current <<<"$output"
  MYSQL_HISTORY_LONG_STATE="$history_long"
  MYSQL_HISTORY_STATE="$history"
  MYSQL_HISTORY_CURRENT_STATE="$history_current"
}

disable_statement_history_consumers() {
  mysql_exec_root "UPDATE performance_schema.setup_consumers SET ENABLED = 'NO' WHERE NAME IN ('events_statements_history_long','events_statements_history','events_statements_current');"
}

enable_statement_history_consumers() {
  mysql_exec_root "UPDATE performance_schema.setup_consumers SET ENABLED = 'YES' WHERE NAME IN ('events_statements_history_long','events_statements_history','events_statements_current');"
}

restore_statement_history_consumers() {
  if [ -n "${MYSQL_HISTORY_LONG_STATE:-}" ]; then
    mysql_exec_root "UPDATE performance_schema.setup_consumers SET ENABLED = '${MYSQL_HISTORY_LONG_STATE}' WHERE NAME = 'events_statements_history_long';"
  fi
  if [ -n "${MYSQL_HISTORY_STATE:-}" ]; then
    mysql_exec_root "UPDATE performance_schema.setup_consumers SET ENABLED = '${MYSQL_HISTORY_STATE}' WHERE NAME = 'events_statements_history';"
  fi
  if [ -n "${MYSQL_HISTORY_CURRENT_STATE:-}" ]; then
    mysql_exec_root "UPDATE performance_schema.setup_consumers SET ENABLED = '${MYSQL_HISTORY_CURRENT_STATE}' WHERE NAME = 'events_statements_current';"
  fi
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

  # Verify column visibility rules
  assert_column_visibility "$output" "deadlock-info"
}

verify_deadlock_info_no_deadlock
verify_deadlock_info

capture_statement_history_states
disable_statement_history_consumers

error_output="$(run_function mysql error-info '__job:local' 'false')"
assert_error_info_not_enabled "$error_output"

run_top_queries mysql
assert_top_queries_error_attribution_not_enabled "$WORKDIR/mysql-top-queries.json"

enable_statement_history_consumers

# Generate errors for multiple categories:
# 1. Table not found (error 1146)
for _ in 1 2 3; do
  mysql_exec_root_allow_error "SELECT * FROM missing_table;"
done

# 2. Duplicate key / constraint violation (error 1062)
for _ in 1 2 3; do
  mysql_exec_root_allow_error "INSERT INTO error_test (id, unique_col, int_col) VALUES (1, 'new_value', 200);"
  mysql_exec_root_allow_error "INSERT INTO error_test (id, unique_col, int_col) VALUES (99, 'existing_value', 300);"
done

# 3. Syntax errors (error 1064)
# These have NULL DIGEST in performance_schema because they fail during parsing
# before instrumentation. The collector should generate a synthetic digest.
for _ in 1 2 3; do
  mysql_exec_root_allow_error "SELECT * FROM WHERE 1=1;"
  mysql_exec_root_allow_error "SELEC * FROM error_test;"
done

sleep 1

error_output="$(run_function mysql error-info '__job:local' 'true')"
assert_error_info_has_errors "$error_output"
assert_column_visibility "$error_output" "error-info"
assert_unique_key_populated "$error_output" "error-info"

run_top_queries mysql
assert_top_queries_error_attribution_enabled "$WORKDIR/mysql-top-queries.json"
assert_column_visibility "$WORKDIR/mysql-top-queries.json" "top-queries"

restore_statement_history_consumers

echo "E2E checks passed for ${MYSQL_VARIANT_LABEL}." >&2
