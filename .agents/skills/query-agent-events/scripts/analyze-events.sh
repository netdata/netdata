#!/usr/bin/env bash
# analyze-events.sh -- group-by stats over a dump from get-events.sh.
#
# Reads either the response envelope (if a single payload) or
# raw rows (if pre-extracted). Emits a top-N counter table on
# the requested dimension.

set -euo pipefail

usage() {
    cat <<'EOF'
analyze-events.sh --by <dim> [options]

Required:
  --by <dim>     dimension to group by; one of:
                 signal, fatal_function, fatal_filename, version,
                 architecture, os_family, os_type, install_type,
                 db_mode, kubernetes, profile, aclk, health,
                 exit_cause, virtualization, chassis_type, host_cpus

Options:
  --input PATH   path to the JSON dump (default: latest under
                 <repo>/.local/audits/query-agent-events/)
  --top N        top N values (default 20)
  --filter "K=V" extra client-side filter (repeatable);
                 e.g. --filter "AE_OS_FAMILY=ubuntu"
  --format text|json  (default text)
  -h, --help

Tip: --by signal groups by AE_FATAL_SIGNAL_CODE (signal crashes).
For non-signal events, the value will be empty.
EOF
}

# Map --by alias -> AE_* field name.
field_for_dim() {
    local dim="$1"
    case "$dim" in
        signal)        echo AE_FATAL_SIGNAL_CODE ;;
        fatal_function) echo AE_FATAL_FUNCTION ;;
        fatal_filename) echo AE_FATAL_FILENAME ;;
        version)       echo AE_AGENT_VERSION ;;
        architecture)  echo AE_HOST_ARCHITECTURE ;;
        os_family)     echo AE_OS_FAMILY ;;
        os_type)       echo AE_OS_TYPE ;;
        install_type)  echo AE_AGENT_INSTALL_TYPE ;;
        db_mode)       echo AE_AGENT_DB_MODE ;;
        kubernetes)    echo AE_AGENT_KUBERNETES ;;
        profile)       echo AE_AGENT_PROFILE_0 ;;
        aclk)          echo AE_AGENT_ACLK ;;
        health)        echo AE_AGENT_HEALTH ;;
        exit_cause)    echo AE_EXIT_CAUSE ;;
        virtualization) echo AE_HOST_VIRTUALIZATION ;;
        chassis_type)  echo AE_HW_CHASSIS_TYPE ;;
        host_cpus)     echo AE_HOST_SYSTEM_CPUS ;;
        *) echo "Unknown --by '$dim'" >&2; exit 2 ;;
    esac
}

BY=
INPUT=
TOP=20
FORMAT=text
declare -a FILTERS=()

while [ $# -gt 0 ]; do
    case "$1" in
        --by)     BY="$2"; shift 2 ;;
        --input)  INPUT="$2"; shift 2 ;;
        --top)    TOP="$2"; shift 2 ;;
        --filter) FILTERS+=("$2"); shift 2 ;;
        --format) FORMAT="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

[ -z "$BY" ] && { usage >&2; exit 2; }

FIELD="$(field_for_dim "$BY")"

# shellcheck source=SCRIPTDIR/_lib.sh disable=SC1091
source "$(cd "$(dirname "$0")" && pwd)/_lib.sh"

# Pick the most recent dump if none given.
if [ -z "$INPUT" ]; then
    audit_dir="$(agentevents_audit_dir)"
    # shellcheck disable=SC2012  # ls -1t is fine for *.json under audit_dir; find pipeline is overkill
    INPUT="$(ls -1t "$audit_dir"/*.json 2>/dev/null | head -1 || true)"
    [ -z "$INPUT" ] && {
        echo "No input dump found under $audit_dir; pass --input PATH" >&2
        exit 2
    }
    echo "[analyze-events] using $INPUT" >&2
fi

# The Function envelope has top-level `data` (rows of arrays)
# and `columns` (name -> {index, ...}).
#
# Two paths:
#  - if the file looks like a Function envelope, project rows
#    via the columns map to extract FIELD;
#  - if it's a flat array of objects (pre-extracted), use jq
#    directly.

# Build filter expression.
filter_expr='true'
for f in "${FILTERS[@]}"; do
    k="${f%%=*}"
    v="${f#*=}"
    filter_expr="$filter_expr and (.\"$k\" == \"$v\")"
done

# Detect format and project.
records=$(jq -c --arg field "$FIELD" '
    if (type == "object" and has("columns") and has("data")) then
        .columns as $c
        | ($c | to_entries
              | map({(.key): (.value.index)})
              | add) as $idx
        | (.data // [])
        | map(
            . as $row
            | reduce ($idx | keys_unsorted)[] as $k
                ({}; .[$k] = $row[$idx[$k]])
          )
    elif (type == "array" and (.[0]? | type == "object")) then
        .
    else
        []
    end
' "$INPUT")

# Apply filters and group.
result=$(printf '%s' "$records" | jq -c --arg field "$FIELD" --argjson top "$TOP" "
    map(select($filter_expr))
    | group_by(.[\$field] // \"\")
    | map({key: (.[0][\$field] // \"(empty)\"), count: length})
    | sort_by(-.count)
    | .[:\$top]
")

case "$FORMAT" in
    json)
        echo "$result" | jq .
        ;;
    text|*)
        echo
        echo "Top $TOP by $BY ($FIELD):"
        printf '%s\n' "----------------------------------------"
        echo "$result" | jq -r '.[] | [.count, .key] | @tsv' \
            | awk -F'\t' '{ printf "%8d  %s\n", $1, $2 }'
        echo "----------------------------------------"
        total=$(echo "$result" | jq '[.[].count] | add // 0')
        echo "  Total in top: $total"
        ;;
esac
