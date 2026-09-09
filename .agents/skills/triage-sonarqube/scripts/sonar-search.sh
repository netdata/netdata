#!/usr/bin/env bash
# Search SonarCloud findings for the configured project.
#
# Usage:
#   sonar-search.sh issues   [--rule RULE_ID] [--resolved=false|true]
#   sonar-search.sh hotspots [--status=TO_REVIEW|REVIEWED]
#   sonar-search.sh summary                                            # rule + count for open issues + hotspots
#
# Output: a single merged JSON object on stdout (.issues / .hotspots is the
# concatenation of all pages). Always paginated to .paging.total -- a `--ps`
# arg is no longer accepted because it was a footgun (only the first page
# was ever returned).
#
# This is a READ-ONLY script -- it does not mutate Sonar state. Safe to
# run anytime to inspect what's outstanding.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
sq_load_env

cmd="${1:-summary}"; shift || true

# Whitelist common URL-param values to avoid raw user input ending up in
# the URL. Sonar would reject malformed params anyway, but the error
# messages are confusing -- fail-fast locally instead.
_validate_resolved() {
    local v="$1"
    case "${v}" in true|false) ;; *)
        echo -e "${SQ_RED}[ERROR]${SQ_NC} --resolved must be 'true' or 'false', got: '${v}'" >&2
        return 1 ;;
    esac
}
_validate_status() {
    local v="$1"
    case "${v}" in TO_REVIEW|REVIEWED) ;; *)
        echo -e "${SQ_RED}[ERROR]${SQ_NC} --status must be 'TO_REVIEW' or 'REVIEWED', got: '${v}'" >&2
        return 1 ;;
    esac
}

case "${cmd}" in
    issues)
        rule=""
        resolved="false"
        while [[ $# -gt 0 ]]; do
            arg="$1"
            case "${arg}" in
                --rule)
                    if [[ $# -lt 2 ]]; then
                        echo -e "${SQ_RED}[ERROR]${SQ_NC} --rule requires a value (e.g. --rule c:S2245)" >&2
                        exit 2
                    fi
                    rule="$2"
                    shift 2
                    ;;
                --resolved=*) resolved="${arg#*=}"; shift ;;
                *) echo "Unknown arg: ${arg}" >&2; exit 2 ;;
            esac
        done
        _validate_resolved "${resolved}"
        path="/api/issues/search?componentKeys=${SONAR_PROJECT}&resolved=${resolved}"
        # URL-encode the rule id so values containing `:` (always),
        # spaces, or other reserved chars don't inject extra params.
        [[ -n "${rule}" ]] && path="${path}&rules=$(sq_url_encode "${rule}")"
        sq_paginate "${path}" \
            | jq -s '{paging: .[0].paging, issues: [.[].issues[]]}'
        ;;

    hotspots)
        status="TO_REVIEW"
        while [[ $# -gt 0 ]]; do
            arg="$1"
            case "${arg}" in
                --status=*) status="${arg#*=}"; shift ;;
                *) echo "Unknown arg: ${arg}" >&2; exit 2 ;;
            esac
        done
        _validate_status "${status}"
        path="/api/hotspots/search?projectKey=${SONAR_PROJECT}&status=${status}"
        sq_paginate "${path}" \
            | jq -s '{paging: .[0].paging, hotspots: [.[].hotspots[]]}'
        ;;

    summary)
        echo "=== Open issues by rule ===" >&2
        # Issue facets are computed server-side and returned on every page;
        # the first page's facet totals reflect ALL matching issues, so a
        # single fetch is correct here.
        sq_run_read curl --fail --silent --show-error -u "${SONAR_TOKEN}:" \
            "${SONAR_HOST_URL}/api/issues/search?componentKeys=${SONAR_PROJECT}&resolved=false&ps=1&facets=rules" \
            | jq -r '.facets[] | select(.property=="rules") | .values[] | "  \(.val) (\(.count))"' \
            | sort -k2 -t'(' -nr | head -30

        echo >&2
        echo "=== Open hotspots by rule ===" >&2
        # Hotspot search has no facets, so we have to walk every page.
        sq_paginate "/api/hotspots/search?projectKey=${SONAR_PROJECT}&status=TO_REVIEW" \
            | jq -s '[.[].hotspots[].ruleKey]
                     | group_by(.) | map({rule: .[0], count: length})
                     | sort_by(-.count) | .[] | "  \(.rule) (\(.count))"' -r \
            | head -30
        ;;

    "")
        echo "usage: $0 issues|hotspots|summary [args...]" >&2
        exit 2
        ;;
    *)
        echo "Unknown command: ${cmd}" >&2
        exit 2
        ;;
esac
