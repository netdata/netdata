#!/usr/bin/env bash
# Helper library for the query-agent-events skill.
#
# Sources query-netdata-agents/scripts/_lib.sh and adds
# agentevents_* helpers. Token-safe: bearers and the cloud
# token never appear on the assistant-visible stdout.
#
# Usage:
#   source "$(git rev-parse --show-toplevel)/.agents/skills/query-agent-events/scripts/_lib.sh"
#   agentevents_load_env

set -euo pipefail

# Resolve self path (zsh + bash compatible).
if [ -n "${ZSH_VERSION-}" ]; then
    eval '_agentevents_lib_self="${(%):-%x}"'
elif [ -n "${BASH_VERSION-}" ]; then
    _agentevents_lib_self="${BASH_SOURCE[0]}"
else
    _agentevents_lib_self="$0"
fi
_agentevents_lib_dir="$(cd "$(dirname "$_agentevents_lib_self")" && pwd)"

# Source query-netdata-agents helpers (transport + bearer mint).
# shellcheck disable=SC1091
source "$(git rev-parse --show-toplevel)/.agents/skills/query-netdata-agents/scripts/_lib.sh"

# ---------------------------------------------------------------
# Env loading

agentevents_load_env() {
    agents_load_env

    : "${AGENT_EVENTS_HOSTNAME:?AGENT_EVENTS_HOSTNAME is empty -- see <repo>/.agents/ENV.md to set it.}"
    : "${AGENT_EVENTS_NODE_ID:?AGENT_EVENTS_NODE_ID is empty -- see <repo>/.agents/ENV.md to set it.}"
    : "${AGENT_EVENTS_MACHINE_GUID:?AGENT_EVENTS_MACHINE_GUID is empty -- see <repo>/.agents/ENV.md to set it.}"
}

# ---------------------------------------------------------------
# Audit directory (gitignored under <repo>/.local/audits/)

agentevents_audit_dir() {
    local d
    d="$(agents_audit_dir)/../query-agent-events"
    mkdir -p "$d"
    (cd "$d" && pwd)
}

# Journal namespace for selections / __logs_sources.
# Hardcoded -- the ingestion server's log2journal --namespace
# is always 'agent-events', regardless of the host's network
# name (which lives in AGENT_EVENTS_HOSTNAME).
agentevents_namespace() {
    printf '%s' 'agent-events'
}

# ---------------------------------------------------------------
# Function call: agentevents_query_function VIA PAYLOAD
#
# VIA is "cloud" or "agent". PAYLOAD is the systemd-journal
# Function POST body (JSON string).
# stdout: response body (JSON). No tokens leak.

agentevents_query_function() {
    local via="$1"
    local payload="$2"

    case "$via" in
        cloud)
            agents_query_cloud \
                POST \
                "/api/v2/nodes/${AGENT_EVENTS_NODE_ID}/function?function=systemd-journal" \
                "$payload"
            ;;
        agent)
            agents_query_agent \
                --node "${AGENT_EVENTS_NODE_ID}" \
                --host "${AGENT_EVENTS_HOSTNAME}:19999" \
                --machine-guid "${AGENT_EVENTS_MACHINE_GUID}" \
                POST \
                "/api/v3/function?function=systemd-journal" \
                "$payload"
            ;;
        *)
            echo "agentevents_query_function: unknown VIA '$via' (use cloud|agent)" >&2
            return 2
            ;;
    esac
}

# ---------------------------------------------------------------
# Default version-filter computation.
#
# agentevents_compute_default_versions VIA SINCE_RELATIVE_SECONDS
#
# Queries the journal for the AE_AGENT_VERSION facet and picks:
#   - the latest stable (matches ^v\d+\.\d+\.\d+$, sorted desc)
#   - up to 3 latest nightlies (matches ^v\d+\.\d+\.\d+-\d+-nightly$, sorted desc by commit count)
#
# Outputs a JSON array of version strings to stdout.

agentevents_compute_default_versions() {
    local via="${1:-cloud}"
    local since_secs="${2:-86400}"

    local namespace
    namespace="$(agentevents_namespace)"

    local payload
    payload=$(jq -nc \
        --arg ns "$namespace" \
        --argjson after "-${since_secs}" \
        '{
            "after": $after,
            "before": 0,
            "last": 1,
            "__logs_sources": $ns,
            "facets": ["AE_AGENT_VERSION"]
        }')

    local resp
    resp="$(agentevents_query_function "$via" "$payload")"

    # Extract the AE_AGENT_VERSION facet's option values.
    # The response shape is documented in
    # docs/netdata-ai/skills/query-netdata-cloud/query-logs.md
    # under "Response shape".
    echo "$resp" | jq -c '
        ([.facets[]? | select(.id=="AE_AGENT_VERSION") | .options[]?.id] // [])
        as $all
        | (
            ($all | map(select(test("^v\\d+\\.\\d+\\.\\d+$"))) | sort | reverse | .[0:1])
            + ($all | map(select(test("^v\\d+\\.\\d+\\.\\d+-\\d+-nightly$")))
                    | sort_by(. | capture("-(?<n>\\d+)-nightly").n | tonumber)
                    | reverse | .[0:3])
        )'
}

# ---------------------------------------------------------------
# No-token-leak self-test.

agentevents_selftest_no_token_leak() {
    # Drive the public wrappers with sentinel values; assert no
    # sentinel ever appears on captured stdout.
    local sentinel="deadbeef-1234-5678-9abc-def012345678"

    # Set sentinels in the environment that COULD leak if a
    # wrapper logged its inputs.
    local saved_token="${NETDATA_CLOUD_TOKEN:-}"
    local saved_node="${AGENT_EVENTS_NODE_ID:-}"

    NETDATA_CLOUD_TOKEN="$sentinel"
    AGENT_EVENTS_NODE_ID="$sentinel"
    export NETDATA_CLOUD_TOKEN AGENT_EVENTS_NODE_ID

    # Run a no-op-ish payload through the helpers; capture stdout.
    local out
    out="$( {
        agentevents_query_function cloud '{"info":true}' 2>/dev/null || true
    } )"

    # Restore.
    NETDATA_CLOUD_TOKEN="$saved_token"
    AGENT_EVENTS_NODE_ID="$saved_node"

    if printf '%s' "$out" | grep -q "$sentinel"; then
        echo "FAIL: sentinel $sentinel appeared on captured stdout" >&2
        return 1
    fi

    echo "PASS: agentevents_selftest_no_token_leak"
    return 0
}
