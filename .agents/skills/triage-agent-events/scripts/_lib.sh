#!/usr/bin/env bash
# Helper library for the triage-agent-events skill.
#
# Sources query-netdata-agents/scripts/_lib.sh and adds
# agentevents_* helpers. Token-safe: bearers and the cloud
# token are masked in request logs; event response bodies remain private evidence.
#
# Usage:
#   source "$(git rev-parse --show-toplevel)/.agents/skills/triage-agent-events/scripts/_lib.sh"
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
    # Kept across the skill rename so per-user caches survive; see AGENTS.md, Local-Only Working Directory.
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
# stdout: response body (JSON), forwarded without content redaction.

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
# agentevents_compute_default_versions VIA AFTER BEFORE
#
# Queries the journal for the AE_AGENT_VERSION facet and picks:
#   - the highest observed stable numeric release tuple
#   - up to 3 observed nightlies, ordered by numeric release tuple then count
#
# Outputs a JSON array of version strings to stdout.

agentevents_compute_default_versions() {
    local via="${1:-cloud}"
    local after="${2:--86400}"
    local before="${3:-0}"

    local namespace
    namespace="$(agentevents_namespace)"

    local payload
    payload=$(jq -nc \
        --arg ns "$namespace" \
        --argjson after "$after" \
        --argjson before "$before" \
        '{
            "after": $after,
            "before": $before,
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
            ($all | map(select(test("^v\\d+\\.\\d+\\.\\d+$"))) | sort_by(ltrimstr("v") | split(".") | map(tonumber)) | reverse | .[0:1])
            + ($all | map(select(test("^v\\d+\\.\\d+\\.\\d+-\\d+-nightly$")))
                    | sort_by(capture("^v(?<major>\\d+)\\.(?<minor>\\d+)\\.(?<patch>\\d+)-(?<n>\\d+)-nightly$")
                              | [.major, .minor, .patch, .n] | map(tonumber))
                    | reverse | .[0:3])
        )'
}

# ---------------------------------------------------------------
# No-token-leak self-test.

agentevents_selftest_no_token_leak() (
    # Subshell preserves caller functions/settings. No .env, cache, or live transport.
    local sentinel="deadbeef-1234-5678-9abc-def012345678"
    local test_bearer="11111111-2222-3333-4444-555555555555"
    local via out
    NETDATA_CLOUD_TOKEN="$sentinel"
    NETDATA_CLOUD_HOSTNAME=cloud.example.com
    AGENT_EVENTS_HOSTNAME=agent.example.com
    AGENT_EVENTS_NODE_ID=22222222-3333-4444-5555-666666666666
    AGENT_EVENTS_MACHINE_GUID=33333333-4444-5555-6666-777777777777
    AGENTS_DRY_RUN=0

    _agents_resolve_bearer() { _agents_set_outvar "$1" "$test_bearer"; }
    curl() {
        local arg
        for arg in "$@"; do
            case "$arg" in
                https://cloud.example.com/api/v2/nodes/*/function?function=systemd-journal)
                    printf '%s' '{"fixture":"cloud"}'; return 0 ;;
                http://agent.example.com:19999/host/*/api/v3/function?function=systemd-journal)
                    printf '%s' '{"fixture":"agent"}'; return 0 ;;
            esac
        done
        return 97
    }

    for via in cloud agent; do
        if out="$(agentevents_query_function "$via" '{"info":true}' 2>&1)"; then
            if [[ "$out" == *"$sentinel"* || "$out" == *"$test_bearer"* ||
                  "$out" != *"\"fixture\":\"$via\""* ]]; then
                printf 'FAIL: %s dispatch output or masking check\n' "$via" >&2
                return 1
            fi
        else
            printf 'FAIL: %s dispatch did not complete\n' "$via" >&2
            return 1
        fi
    done
    printf '%s\n' 'PASS: agentevents_selftest_no_token_leak'
)
