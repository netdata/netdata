#!/usr/bin/env bash
# get-events.sh -- fetch events of interest from agent-events.
#
# Index-friendly defaults: 24h time, multi-value selections,
# auto version filter (latest stable + latest 3 nightlies).
#
# Output: JSON dump under
#   <repo>/.local/audits/query-agent-events/<timestamp>.json

set -euo pipefail

usage() {
    cat <<'EOF'
get-events.sh [options]

Transport:
  --via cloud|agent          (default: cloud)

Time window:
  --since '<n>h ago'|'<n>d ago'|<seconds>   (default: 24h ago)
  --before now|<seconds>                    (default: now)

Filters (all are AND'd; values within a flag are OR'd):
  --health <classes>         comma-separated; common: all, crash, healthy
                             (default: all)
                             "crash"   -> crash-first,crash-loop,crash-repeated,crash-entered
                             "healthy" -> healthy-first,healthy-loop,healthy-repeated,healthy-recovered
  --exit-cause <causes>      comma-separated; common: all, fatal, signal, graceful
                             (default: all)
                             "fatal"   -> deliberate-exit class (OOM, disk full, etc.)
                             "signal"  -> killed-hard variants
                             "graceful"-> exit instructed/updated/shutdown/no last status
  --signal <values>          comma-separated AE_FATAL_SIGNAL_CODE values
                             example: SIGSEGV/SEGV_MAPERR,SIGBUS/BUS_OBJERR
  --function <names>         comma-separated AE_FATAL_FUNCTION values
  --version <spec>           "auto" (default), "all", or a regex
                             auto -> latest stable + latest 3 nightlies (computed)
                             all  -> no version filter
                             else -> regex applied client-side after fetch
                                     (multi-value selections require explicit values;
                                      use --versions for that)
  --versions <list>          explicit comma-separated AE_AGENT_VERSION values
                             (overrides --version's auto/all/regex)
  --arch <values>            comma-separated AE_HOST_ARCHITECTURE values
  --os-family <values>       comma-separated AE_OS_FAMILY values
  --query <fts>              residual FTS narrower (after structured filters)
  --facets <names>           comma-separated; included in response for grouping

Response control:
  --last N                   page size, default 500
  --histogram FIELD          add a histogram bucket on FIELD

Output:
  --output PATH              path to write the JSON dump (default: auto under .local/audits/...)

Other:
  -h, --help                 this message
  -v, --verbose              show the constructed payload before fetching
EOF
}

# ---------------------------------------------------------------
# Argument parsing.

VIA=cloud
SINCE='24h ago'
BEFORE=now
HEALTH=all
EXIT_CAUSE=all
SIGNAL=
FUNCTION=
VERSION=auto
VERSIONS_EXPLICIT=
ARCH=
OS_FAMILY=
QUERY=
FACETS=
LAST=500
HISTOGRAM=
OUTPUT=
VERBOSE=0

while [ $# -gt 0 ]; do
    case "$1" in
        --via)         VIA="$2"; shift 2 ;;
        --since)       SINCE="$2"; shift 2 ;;
        --before)      BEFORE="$2"; shift 2 ;;
        --health)      HEALTH="$2"; shift 2 ;;
        --exit-cause)  EXIT_CAUSE="$2"; shift 2 ;;
        --signal)      SIGNAL="$2"; shift 2 ;;
        --function)    FUNCTION="$2"; shift 2 ;;
        --version)     VERSION="$2"; shift 2 ;;
        --versions)    VERSIONS_EXPLICIT="$2"; shift 2 ;;
        --arch)        ARCH="$2"; shift 2 ;;
        --os-family)   OS_FAMILY="$2"; shift 2 ;;
        --query)       QUERY="$2"; shift 2 ;;
        --facets)      FACETS="$2"; shift 2 ;;
        --last)        LAST="$2"; shift 2 ;;
        --histogram)   HISTOGRAM="$2"; shift 2 ;;
        --output)      OUTPUT="$2"; shift 2 ;;
        -v|--verbose)  VERBOSE=1; shift ;;
        -h|--help)     usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

# ---------------------------------------------------------------
# Lib + env.

# shellcheck source=SCRIPTDIR/_lib.sh disable=SC1091
source "$(cd "$(dirname "$0")" && pwd)/_lib.sh"
agentevents_load_env

# ---------------------------------------------------------------
# Time spec -> relative seconds.

parse_time() {
    local s="$1"
    case "$s" in
        now)               echo 0 ;;
        *' ago')           # "24h ago", "7d ago", "30m ago"
            local body="${s% ago}"
            case "$body" in
                *h) printf -- '-%d' "$(( ${body%h} * 3600 ))" ;;
                *d) printf -- '-%d' "$(( ${body%d} * 86400 ))" ;;
                *m) printf -- '-%d' "$(( ${body%m} * 60 ))" ;;
                *)  printf -- '-%d' "${body}" ;;
            esac
            ;;
        -*|0*|[1-9]*)      echo "$s" ;;
        *) echo "Invalid time spec: $s" >&2; exit 2 ;;
    esac
}

AFTER=$(parse_time "$SINCE")
BEFORE_PARSED=$(parse_time "$BEFORE")

# ---------------------------------------------------------------
# Build selections.

declare -a SELECTION_KEYS=()
declare -A SELECTION_VALUES=()

set_selection() {
    local key="$1"
    local csv="$2"
    [ -z "$csv" ] && return
    SELECTION_KEYS+=("$key")
    SELECTION_VALUES[$key]="$csv"
}

case "$HEALTH" in
    all|"") ;;
    crash)   set_selection AE_AGENT_HEALTH "crash-first,crash-loop,crash-repeated,crash-entered" ;;
    healthy) set_selection AE_AGENT_HEALTH "healthy-first,healthy-loop,healthy-repeated,healthy-recovered" ;;
    *)       set_selection AE_AGENT_HEALTH "$HEALTH" ;;
esac

case "$EXIT_CAUSE" in
    all|"") ;;
    fatal)
        set_selection AE_EXIT_CAUSE \
"no last status,out of memory,disk full,disk almost full,disk read-only,already running,fatal on start,fatal on exit,fatal and exit,exit timeout"
        ;;
    signal)
        set_selection AE_EXIT_CAUSE \
"deadly signal,deadly signal on start,deadly signal on exit,deadly signal and exit,killed hard,killed hard on start,killed hard on shutdown,killed hard on exit,killed hard on update,killed hard low ram,killed fatal"
        ;;
    graceful)
        set_selection AE_EXIT_CAUSE \
"exit instructed,exit and updated,exit on system shutdown,exit to update,exit no reason,no last status"
        ;;
    *)
        set_selection AE_EXIT_CAUSE "$EXIT_CAUSE"
        ;;
esac

[ -n "$SIGNAL" ]    && set_selection AE_FATAL_SIGNAL_CODE "$SIGNAL"
[ -n "$FUNCTION" ]  && set_selection AE_FATAL_FUNCTION    "$FUNCTION"
[ -n "$ARCH" ]      && set_selection AE_HOST_ARCHITECTURE "$ARCH"
[ -n "$OS_FAMILY" ] && set_selection AE_OS_FAMILY         "$OS_FAMILY"

# Version handling.
if [ -n "$VERSIONS_EXPLICIT" ]; then
    set_selection AE_AGENT_VERSION "$VERSIONS_EXPLICIT"
elif [ "$VERSION" = "auto" ]; then
    echo "[get-events] computing default version filter (latest stable + latest 3 nightlies)..." >&2
    versions_json="$(agentevents_compute_default_versions "$VIA" "${AFTER#-}")"
    if [ "$(echo "$versions_json" | jq 'length')" -gt 0 ]; then
        versions_csv="$(echo "$versions_json" | jq -r 'join(",")')"
        echo "[get-events] auto versions: $versions_csv" >&2
        set_selection AE_AGENT_VERSION "$versions_csv"
    else
        echo "[get-events] auto version detection found no versions; proceeding without version filter" >&2
    fi
elif [ "$VERSION" = "all" ]; then
    : # no filter
else
    # Pattern -- best-effort: not multi-value selection, leave it
    # to the caller to client-side-filter the dump after fetch.
    echo "[get-events] --version <regex> is not pushed to the server; filter the resulting JSON with jq" >&2
fi

# ---------------------------------------------------------------
# Compose payload.

ns="$(agentevents_namespace)"

# Build selections object using jq.
SELECTIONS_JSON='{}'
for key in "${SELECTION_KEYS[@]}"; do
    csv="${SELECTION_VALUES[$key]}"
    SELECTIONS_JSON=$(echo "$SELECTIONS_JSON" | jq --arg k "$key" --arg v "$csv" \
        '.[$k] = ($v | split(","))')
done

# Build the full payload.
PAYLOAD=$(jq -nc \
    --argjson after "$AFTER" \
    --argjson before "$BEFORE_PARSED" \
    --argjson last "$LAST" \
    --arg ns "$ns" \
    --argjson selections "$SELECTIONS_JSON" \
    --arg query "$QUERY" \
    --arg facets_csv "$FACETS" \
    --arg histogram "$HISTOGRAM" \
    '
    {
        "after":  $after,
        "before": $before,
        "last":   $last,
        "direction": "backward",
        "__logs_sources": $ns
    }
    | (if ($selections | length) > 0 then .selections = $selections else . end)
    | (if ($query | length) > 0 then .query = $query else . end)
    | (if ($facets_csv | length) > 0 then .facets = ($facets_csv | split(",")) else . end)
    | (if ($histogram | length) > 0 then .histogram = $histogram else . end)
    ')

if [ "$VERBOSE" -eq 1 ]; then
    echo "[get-events] payload:" >&2
    echo "$PAYLOAD" | jq . >&2
fi

# ---------------------------------------------------------------
# Output path.

if [ -z "$OUTPUT" ]; then
    audit_dir="$(agentevents_audit_dir)"
    OUTPUT="$audit_dir/$(date -u +%Y%m%dT%H%M%SZ).json"
fi

# ---------------------------------------------------------------
# Fetch.

echo "[get-events] fetching via $VIA (output: $OUTPUT)..." >&2
agentevents_query_function "$VIA" "$PAYLOAD" > "$OUTPUT"

rows="$(jq '.data | length // 0' "$OUTPUT" 2>/dev/null || echo 0)"
echo "[get-events] wrote $rows row(s) to $OUTPUT" >&2

# Print path on stdout for piping.
echo "$OUTPUT"
