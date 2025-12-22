#!/usr/bin/env bash
#
# Extract tool-call sequence, SQL, and final output from ai-agent session snapshots.
#
# Usage:
#   ./neda/bigquery-snapshot-dump.sh --txn <txn_id>
#   ./neda/bigquery-snapshot-dump.sh --log tmp/bigquery-tests/logs/case.log
#   ./neda/bigquery-snapshot-dump.sh --case realized_arr_kpi_customer_diff
#
set -euo pipefail

# Transparent execution helper (from project instructions)
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; GRAY='\033[0;90m'; NC='\033[0m'
run() {
  printf >&2 "${GRAY}$(pwd) >${NC} ${YELLOW}%q " "$@"; printf >&2 "${NC}\n"
  "$@"
  local code=$?
  if [[ $code -ne 0 ]]; then
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return $code
  fi
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SESSIONS_DIR="${SESSIONS_DIR:-${HOME}/.ai-agent/sessions}"
LOG_DIR="${LOG_DIR:-${SCRIPT_DIR}/../tmp/bigquery-tests/logs}"
OUT_BASE="${OUT_BASE:-${SCRIPT_DIR}/../tmp/bigquery-tests/snapshots}"

usage() {
  cat <<'EOF'
Usage: ./neda/bigquery-snapshot-dump.sh [--txn ID] [--log FILE] [--case NAME]

Options:
  --txn ID          Use a specific session txn_id.
  --log FILE        Extract txn_id from a log file.
  --case NAME       Look for LOG_DIR/NAME.log and extract txn_id.
  --sessions DIR    Override sessions dir (default: ~/.ai-agent/sessions)
  --log-dir DIR     Override log dir (default: tmp/bigquery-tests/logs)
  --out DIR         Output base dir (default: tmp/bigquery-tests/snapshots)
  -h, --help        Show help.
EOF
}

strip_ansi() {
  sed -E 's/\x1B\[[0-9;]*[a-zA-Z]//g'
}

collect_txn_from_log() {
  local log_file="$1"
  if [[ ! -f "$log_file" ]]; then
    echo "Missing log file: $log_file" >&2
    return 1
  fi
  strip_ansi < "$log_file" | sed -nE 's/.*txn_id=([a-f0-9-]+).*/\1/p' | sort -u
}

declare -a TXN_IDS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --txn)
      TXN_IDS+=("${2:-}")
      shift
      ;;
    --log)
      mapfile -t ids < <(collect_txn_from_log "${2:-}")
      TXN_IDS+=("${ids[@]}")
      shift
      ;;
    --case)
      mapfile -t ids < <(collect_txn_from_log "${LOG_DIR}/${2:-}.log")
      TXN_IDS+=("${ids[@]}")
      shift
      ;;
    --sessions)
      SESSIONS_DIR="${2:-}"
      shift
      ;;
    --log-dir)
      LOG_DIR="${2:-}"
      shift
      ;;
    --out)
      OUT_BASE="${2:-}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

if [[ ${#TXN_IDS[@]} -eq 0 ]]; then
  echo "No txn_id provided. Use --txn, --log, or --case." >&2
  usage >&2
  exit 1
fi

run mkdir -p "${OUT_BASE}"

for txn_id in "${TXN_IDS[@]}"; do
  if [[ -z "$txn_id" ]]; then
    continue
  fi
  snapshot="${SESSIONS_DIR}/${txn_id}.json.gz"
  if [[ ! -f "$snapshot" ]]; then
    echo "Missing snapshot: ${snapshot}" >&2
    continue
  fi

  out_dir="${OUT_BASE}/${txn_id}"
  run mkdir -p "${out_dir}"

  # Tool call events (request/response previews)
  run bash -c "zcat \"${snapshot}\" | jq -r '[.opTree.turns[].ops[] | select(.kind==\"tool\") | .logs[] | {ts:.timestamp, direction:.direction, tool:(.details.tool? // null), tool_namespace:(.details.tool_namespace? // null), request_preview:(.details.request_preview? // null), response_preview:(.details.response_preview? // null), message:.message}]' > \"${out_dir}/tool-events.json\""

  # All SQL statements seen in the session (order as encountered in JSON)
  run bash -c "zcat \"${snapshot}\" | jq -r '.. | objects | select(has(\"sql\")) | .sql' > \"${out_dir}/sql.txt\""

  # Final output preview (if present)
  run bash -c "zcat \"${snapshot}\" | jq -r '.. | objects | select(has(\"textPreview\")) | .textPreview' | tail -n 1 > \"${out_dir}/final.txt\""

  # High-level totals
  run bash -c "zcat \"${snapshot}\" | jq -r '.opTree.totals' > \"${out_dir}/totals.json\""

  echo "Snapshot extracted: ${out_dir}"
done
