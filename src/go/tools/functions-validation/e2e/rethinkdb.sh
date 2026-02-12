#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib.sh"

init_workspace "rethinkdb"
trap cleanup EXIT

RETHINKDB_PORT="$(reserve_port)"
write_env "RETHINKDB_PORT" "$RETHINKDB_PORT"
replace_in_file "$WORKDIR/config/go.d/rethinkdb.conf" "127.0.0.1:28015" "127.0.0.1:${RETHINKDB_PORT}"

compose_up rethinkdb
wait_healthy rethinkdb 60

build_plugin

wait_port() {
  local port="$1"
  for _ in $(seq 1 30); do
    if python3 - <<PY
import socket
s = socket.socket()
s.settimeout(1)
try:
    s.connect(("127.0.0.1", int("${port}")))
    s.close()
    raise SystemExit(0)
except Exception:
    raise SystemExit(1)
PY
    then
      return 0
    fi
    sleep 2
  done
  echo "Timed out waiting for RethinkDB to accept connections on ${port}" >&2
  return 1
}

wait_port "$RETHINKDB_PORT"

run_bg bash -c "cd \"$REPO_ROOT/src/go\" && go run ./tools/functions-validation/seed/rethinkdb/hold.go --addr 127.0.0.1:${RETHINKDB_PORT} --duration 40s"
HOLD_PID="$LAST_BG_PID"
sleep 3

run_info_method rethinkdb running-queries
run_running_queries rethinkdb 1

if kill -0 "$HOLD_PID" 2>/dev/null; then
  kill "$HOLD_PID"
  wait "$HOLD_PID" || true
fi

echo "E2E checks passed for rethinkdb." >&2
