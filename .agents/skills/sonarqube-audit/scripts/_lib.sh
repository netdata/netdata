#!/usr/bin/env bash
# Common helpers for sonarqube-audit scripts.
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# IMPORTANT: define with $'...' so the variables contain real ESC bytes,
# not the literal four-character string "\033". This way both `echo -e
# "${SQ_RED}..."` and `printf '%s' "${SQ_RED}..."` render correctly --
# without forcing every printf format string to be the variable itself
# (which trips shellcheck SC2059) or %b (which adds inconsistency).
#
# Color vars are referenced by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
SQ_RED=$'\033[0;31m'
# shellcheck disable=SC2034
SQ_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
SQ_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
SQ_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
SQ_NC=$'\033[0m'

sq_repo_root() {
    git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel
}

sq_load_env() {
    local root env
    root="$(sq_repo_root)"
    env="${root}/.env"
    if [[ ! -f "${env}" || ! -r "${env}" ]]; then
        echo -e "${SQ_RED}[ERROR]${SQ_NC} Missing ${env}. See SKILL.md for the .env template." >&2
        return 1
    fi
    set -a
    # shellcheck disable=SC1090
    source "${env}"
    set +a

    : "${SONAR_TOKEN:?SONAR_TOKEN is empty in .env}"
    : "${SONAR_HOST_URL:=https://sonarcloud.io}"
    : "${SONAR_PROJECT:?SONAR_PROJECT is empty in .env (e.g. netdata_netdata)}"
    # SONAR_ORG is optional today -- the existing scripts don't pass it to
    # the API, but qualityprofile management calls (documented in SKILL.md)
    # require it. Default empty; consumers should fail loudly if they need it.
    : "${SONAR_ORG:=}"
    export SONAR_TOKEN SONAR_HOST_URL SONAR_PROJECT SONAR_ORG
}

sq_audit_dir() {
    local root dir
    root="$(sq_repo_root)"
    dir="${root}/.local/audits/sonarqube"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Cloudflare in front of api.sonarcloud.io rejects non-ASCII bodies.
# Fail before the network round-trip rather than debug a 403 challenge.
#
# `tr -d '\000-\177'` deletes ALL ASCII bytes; anything left is non-ASCII.
# This is portable across GNU and BSD/macOS (unlike `grep -P`, which is GNU-only).
sq_require_ascii() {
    local s="$1"
    if LC_ALL=C printf '%s' "${s}" | LC_ALL=C tr -d '\000-\177' | grep -q .; then
        echo -e "${SQ_RED}[ERROR]${SQ_NC} Comment contains non-ASCII characters. Cloudflare blocks them. Replace em-dashes with '--' and curly quotes with straight quotes." >&2
        return 1
    fi
}

# Print a curl invocation with the token masked (for transparency without leaking).
# In SONAR_DRY_RUN mode the command is printed but not executed -- this only
# affects calls routed through sq_run, which is the WRITE path (api_post).
# Read-only API calls (issue/hotspot search used to enumerate findings) still
# run in dry-run so the caller can see what would be acted on.
sq_run() {
    local arg
    # Color vars contain real ESC bytes (defined with $'...'), so '%s' is
    # both safe (no SC2059) and correctly renders the colors.
    printf >&2 '%s> %s' "${SQ_GRAY}" "${SQ_YELLOW}"
    for arg in "$@"; do
        if [[ "${arg}" == "${SONAR_TOKEN}:" ]]; then
            printf >&2 '%q ' '<TOKEN>:'
        else
            printf >&2 '%q ' "${arg}"
        fi
    done
    printf >&2 '%s\n' "${SQ_NC}"
    if [[ "${SONAR_DRY_RUN:-0}" == "1" ]]; then
        return 0
    fi
    "$@"
}

# URL-encode a string for inclusion in a Sonar API query parameter.
# Sonar rule IDs (`c:S2245`, `shelldre:S131`, etc.) contain `:` which must
# become `%3A`; some rule namespaces also contain other reserved chars.
# Limitation: emits the codepoint, not UTF-8 bytes, for non-ASCII -- safe
# for Sonar rule IDs (always ASCII) but do NOT use this for arbitrary
# user-supplied strings without auditing.
sq_url_encode() {
    local s="$1" out="" i ch
    for (( i=0; i<${#s}; i++ )); do
        ch="${s:$i:1}"
        case "${ch}" in
            [a-zA-Z0-9._~-]) out+="${ch}" ;;
            *) out+="$(printf '%%%02X' "'${ch}")" ;;
        esac
    done
    printf '%s' "${out}"
}

# Read-only Sonar API call: prints the masked curl line (transparency)
# but always executes (dry-run only suppresses WRITE calls). Use this for
# enumeration calls that drive subsequent decisions (issue/hotspot search).
sq_run_read() {
    local arg
    printf >&2 '%s> %s' "${SQ_GRAY}" "${SQ_YELLOW}"
    for arg in "$@"; do
        if [[ "${arg}" == "${SONAR_TOKEN}:" ]]; then
            printf >&2 '%q ' '<TOKEN>:'
        else
            printf >&2 '%q ' "${arg}"
        fi
    done
    printf >&2 '%s\n' "${SQ_NC}"
    "$@"
}

# Paginate a Sonar API listing endpoint until paging.total is reached.
# Emits each page's body to stdout (one JSON value per page); the caller
# composes them with `jq -s 'reduce .[] as $p (...)'` to sum or merge.
#
# Args:
#   $1 = path with all query params EXCEPT ps and p (e.g.
#        "/api/issues/search?componentKeys=...&resolved=false")
#
# Sonar's max page size is 500; we use it. The function uses sq_run_read
# so the masked curl is logged but execution is not skipped in dry-run
# (read-only enumeration should still happen so the caller sees what
# would be acted on).
sq_paginate() {
    local path="$1"
    local sep
    if [[ "${path}" == *\?* ]]; then sep='&'; else sep='?'; fi
    local page=1 total=-1 fetched=0
    while :; do
        local resp
        resp="$(sq_run_read curl --fail --silent --show-error -u "${SONAR_TOKEN}:" \
            "${SONAR_HOST_URL}${path}${sep}ps=500&p=${page}")"

        # Validate the response is a JSON object with a .paging field --
        # otherwise we can't decide when to stop and would loop forever
        # (or terminate prematurely on 0). Bail loudly.
        if ! jq -e 'type=="object" and has("paging")' >/dev/null 2>&1 <<< "${resp}"; then
            echo -e "${SQ_RED}[sq_paginate]${SQ_NC} response missing .paging on ${path} page ${page}; first 200 chars:" >&2
            head -c 200 <<< "${resp}" >&2; echo >&2
            return 1
        fi

        printf '%s\n' "${resp}"
        # Sonar wraps results either under .issues / .hotspots / .components
        # / .rules / .users; .paging is consistent across endpoints. Reject
        # any response whose array key we don't recognise so the caller is
        # forced to add support rather than silently get zero rows.
        local array_key in_page
        array_key=$(jq -r '
            (.issues|values|"issues") //
            (.hotspots|values|"hotspots") //
            (.components|values|"components") //
            (.rules|values|"rules") //
            (.users|values|"users") //
            empty
        ' <<< "${resp}")
        if [[ -z "${array_key}" ]]; then
            echo -e "${SQ_RED}[sq_paginate]${SQ_NC} unrecognized payload from ${path}; expected one of issues/hotspots/components/rules/users." >&2
            return 1
        fi
        in_page=$(jq -r --arg k "${array_key}" '.[$k] | length' <<< "${resp}")
        total=$(jq -r '.paging.total' <<< "${resp}")
        fetched=$(( fetched + in_page ))
        (( fetched >= total || in_page == 0 )) && break
        page=$(( page + 1 ))
    done
}

