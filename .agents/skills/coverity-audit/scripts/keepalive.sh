#!/usr/bin/env bash
# Keep the Coverity Scan session warm during a triage run.
#
# DESIGN — the script EXITS NON-ZERO the moment a ping fails. That fires the
# orchestrator's background-task completion notification, so the agent learns
# *immediately* that the cookie went bad and can ask the user to recapture it.
# DO NOT silently retry — silent retries hide the failure and waste a triage
# session's worth of work.
#
# The Coverity session cookie expires after a few minutes of inactivity AND
# requires the user's browser tab on https://scan.coverity.com to be open.
# Closing the browser tab kills the session immediately; pings stop working.
#
# Usage (from an orchestrator agent):
#   Bash tool with run_in_background=true,
#   command="bash .agents/skills/coverity-audit/scripts/keepalive.sh"
#
# Stop the background task at the end of the triage session.
#
# Environment overrides:
#   PING_INTERVAL   seconds between pings (default 300 = 5 min)

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
cov_load_env

PING_INTERVAL="${PING_INTERVAL:-300}"

# Pick a cheap, authenticated endpoint. /reports/table.json with the configured
# Outstanding view is ideal — it returns proper JSON when authenticated and
# HTML (Cloudflare challenge or login redirect) when not.
if [[ -z "${COVERITY_VIEW_OUTSTANDING:-}" ]]; then
    echo -e "${COV_RED}[ERROR]${COV_NC} COVERITY_VIEW_OUTSTANDING is not set in .env -- keepalive needs a viewId to ping. See SKILL.md." >&2
    exit 1
fi
if [[ ! "${COVERITY_VIEW_OUTSTANDING}" =~ ^[1-9][0-9]*$ ]]; then
    echo -e "${COV_RED}[ERROR]${COV_NC} COVERITY_VIEW_OUTSTANDING must be a positive integer (got: '${COVERITY_VIEW_OUTSTANDING}')" >&2
    exit 1
fi

PING_URL="${COVERITY_HOST}/reports/table.json?projectId=${COVERITY_PROJECT_ID}&viewId=${COVERITY_VIEW_OUTSTANDING}"

ping_once() {
    local body rc
    # Capture both body and HTTP code in one go.
    body="$(curl -sS --max-time 30 \
        -H "accept: application/json, text/plain, */*" \
        -H "user-agent: ${COVERITY_USER_AGENT}" \
        -H "referer: ${COVERITY_HOST}/" \
        -b "${COVERITY_COOKIE}" \
        "${PING_URL}" 2>/dev/null)" || {
            rc=$?
            echo "[$(date -Iseconds)] FAIL curl rc=${rc}" >&2
            return 1
        }

    if [[ -z "${body}" ]]; then
        echo "[$(date -Iseconds)] FAIL empty response -- session likely expired" >&2
        return 1
    fi

    # Validate the JSON shape -- a Cloudflare challenge or login redirect
    # returns HTML; we want the structured response.
    if ! printf '%s' "${body}" | jq -e '.resultSet.results' >/dev/null 2>&1; then
        echo "[$(date -Iseconds)] FAIL response shape invalid (first 80 chars: '${body:0:80}') -- session likely expired or browser tab closed" >&2
        return 1
    fi

    echo "[$(date -Iseconds)] OK" >&2
    return 0
}

# First ping immediately -- if we're already dead, fail fast.
ping_once || exit 1
echo "[$(date -Iseconds)] keepalive running (interval=${PING_INTERVAL}s)" >&2

while true; do
    sleep "${PING_INTERVAL}"
    ping_once || exit 1
done
