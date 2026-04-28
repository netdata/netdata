#!/usr/bin/env bash
# Apply per-finding triage decisions on SonarCloud:
#   issues   -> falsepositive | wontfix | confirm
#   hotspots -> REVIEWED + (SAFE | ACKNOWLEDGED | FIXED)
#
# AUTHENTICATION:
#   Reads SONAR_TOKEN, SONAR_HOST_URL, SONAR_PROJECT, SONAR_ORG from <repo-root>/.env.
#   Token is sent as basic-auth username with empty password.
#
# COMMENTS MUST BE ASCII-ONLY:
#   Cloudflare's WAF in front of api.sonarcloud.io blocks non-ASCII bodies
#   (em-dashes, smart quotes, accented characters). Stick to "--", '"', etc.
#
# USAGE:
#   sonar-mark.sh fp     <ISSUE_KEY>   <COMMENT>     # Bug/Vuln -> False Positive
#   sonar-mark.sh wontfix <ISSUE_KEY>  <COMMENT>     # Bug/Vuln -> Won't Fix
#   sonar-mark.sh confirm <ISSUE_KEY>  [COMMENT]     # Bug/Vuln -> Confirmed (real, will fix)
#   sonar-mark.sh safe   <HOTSPOT_KEY> <COMMENT>     # Hotspot  -> REVIEWED + SAFE
#   sonar-mark.sh ack    <HOTSPOT_KEY> <COMMENT>     # Hotspot  -> REVIEWED + ACKNOWLEDGED
#   sonar-mark.sh fixed  <HOTSPOT_KEY> <COMMENT>     # Hotspot  -> REVIEWED + FIXED
#
# FAMILY MODE (acts on every open finding for a rule):
#   sonar-mark.sh family-fp   <RULE_ID>   <COMMENT>  # e.g. go:S2077
#   sonar-mark.sh family-safe <RULE_ID>   <COMMENT>  # e.g. c:S5443
#
#   Family mode prints the matched keys and prompts before acting unless
#   SONAR_MARK_YES=1 is set in the environment.
#
# DRY RUN:
#   Set SONAR_DRY_RUN=1 to print the curl commands without executing.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
sq_load_env

api_post() {
    local path="$1"; shift
    sq_run curl --fail --silent --show-error \
        -u "${SONAR_TOKEN}:" \
        -X POST "${SONAR_HOST_URL}${path}" "$@"
}

issue_add_comment() {
    local key="$1" text="$2"
    sq_require_ascii "${text}"
    api_post "/api/issues/add_comment" \
        --data-urlencode "issue=${key}" \
        --data-urlencode "text=${text}" \
        -o /dev/null
}

issue_transition() {
    local key="$1" transition="$2"
    api_post "/api/issues/do_transition" \
        --data-urlencode "issue=${key}" \
        --data-urlencode "transition=${transition}" \
        -o /dev/null
}

mark_issue() {
    local transition="$1" key="$2" comment="${3:-}"
    if [[ -n "${comment}" ]]; then
        issue_add_comment "${key}" "${comment}"
    fi
    issue_transition "${key}" "${transition}"
    echo -e "${SQ_GREEN}[OK]${SQ_NC} issue ${key} -> ${transition}" >&2
}

hotspot_change_status() {
    local key="$1" resolution="$2" comment="$3"
    sq_require_ascii "${comment}"
    # Add the comment first so it persists even if the transition fails.
    api_post "/api/hotspots/add_comment" \
        --data-urlencode "hotspot=${key}" \
        --data-urlencode "comment=${comment}" \
        -o /dev/null
    api_post "/api/hotspots/change_status" \
        --data-urlencode "hotspot=${key}" \
        --data-urlencode "status=REVIEWED" \
        --data-urlencode "resolution=${resolution}" \
        -o /dev/null
    echo -e "${SQ_GREEN}[OK]${SQ_NC} hotspot ${key} -> REVIEWED/${resolution}" >&2
}

list_open_issues_for_rule() {
    # Sonar caps page size at 500; sq_paginate walks every page until
    # paging.total. Token is masked in transparency log.
    local rule="$1" rule_enc
    rule_enc="$(sq_url_encode "${rule}")"
    sq_paginate "/api/issues/search?componentKeys=${SONAR_PROJECT}&rules=${rule_enc}&resolved=false" \
        | jq -r '.issues[].key'
}

list_open_hotspots_for_rule() {
    # Sonar's hotspot search does not accept a rule filter -- we have to
    # fetch all TO_REVIEW hotspots and filter client-side. Pass the rule
    # via jq's --arg so values containing colons / quotes / shell
    # metacharacters cannot inject into the filter.
    local rule="$1"
    sq_paginate "/api/hotspots/search?projectKey=${SONAR_PROJECT}&status=TO_REVIEW" \
        | jq -r --arg rule "${rule}" '.hotspots[] | select(.ruleKey == $rule) | .key'
}

confirm_family() {
    local rule="$1" count="$2" action="$3"
    if [[ "${SONAR_MARK_YES:-0}" == "1" ]]; then
        return 0
    fi
    echo -e "${SQ_YELLOW}About to ${action} ${count} finding(s) for rule ${rule}.${SQ_NC}" >&2
    echo -en "${SQ_YELLOW}Proceed? [y/N] ${SQ_NC}" >&2
    local ans
    read -r ans
    # Lowercase via tr -- bash 4+ has ${var,,} but macOS ships bash 3.2.
    local ans_lc
    ans_lc=$(printf '%s' "${ans}" | tr '[:upper:]' '[:lower:]')
    [[ "${ans_lc}" == "y" || "${ans_lc}" == "yes" ]]
}

family_fp() {
    local rule="$1" comment="$2"
    sq_require_ascii "${comment}"
    local keys
    keys="$(list_open_issues_for_rule "${rule}")"
    local count
    count="$(printf '%s\n' "${keys}" | grep -c . || true)"
    if [[ "${count}" == "0" ]]; then
        echo -e "${SQ_YELLOW}No open issues for rule ${rule}.${SQ_NC}" >&2
        return 0
    fi
    echo "${keys}" >&2
    confirm_family "${rule}" "${count}" "mark as False Positive" || { echo "Aborted." >&2; return 1; }
    while IFS= read -r key; do
        [[ -z "${key}" ]] && continue
        mark_issue "falsepositive" "${key}" "${comment}"
    done <<< "${keys}"
}

family_safe() {
    local rule="$1" comment="$2"
    sq_require_ascii "${comment}"
    local keys
    keys="$(list_open_hotspots_for_rule "${rule}")"
    local count
    count="$(printf '%s\n' "${keys}" | grep -c . || true)"
    if [[ "${count}" == "0" ]]; then
        echo -e "${SQ_YELLOW}No open hotspots for rule ${rule}.${SQ_NC}" >&2
        return 0
    fi
    echo "${keys}" >&2
    confirm_family "${rule}" "${count}" "mark REVIEWED/SAFE" || { echo "Aborted." >&2; return 1; }
    while IFS= read -r key; do
        [[ -z "${key}" ]] && continue
        hotspot_change_status "${key}" "SAFE" "${comment}"
    done <<< "${keys}"
}

usage() {
    sed -n '4,33p' "$0"
    exit 2
}

cmd="${1:-}"; shift || true
case "${cmd}" in
    fp)        mark_issue "falsepositive" "${1:?key required}" "${2:?comment required}" ;;
    wontfix)   mark_issue "wontfix"       "${1:?key required}" "${2:?comment required}" ;;
    confirm)   mark_issue "confirm"       "${1:?key required}" "${2:-}" ;;
    safe)      hotspot_change_status      "${1:?key required}" "SAFE"         "${2:?comment required}" ;;
    ack)       hotspot_change_status      "${1:?key required}" "ACKNOWLEDGED" "${2:?comment required}" ;;
    fixed)     hotspot_change_status      "${1:?key required}" "FIXED"        "${2:?comment required}" ;;
    family-fp)   family_fp   "${1:?rule id required (e.g. go:S2077)}" "${2:?comment required}" ;;
    family-safe) family_safe "${1:?rule id required (e.g. c:S5443)}"  "${2:?comment required}" ;;
    ""|-h|--help|help) usage ;;
    *) echo -e "${SQ_RED}[ERROR]${SQ_NC} Unknown subcommand: ${cmd}" >&2; usage ;;
esac
