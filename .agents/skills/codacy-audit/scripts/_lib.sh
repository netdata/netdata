#!/usr/bin/env bash
# Common helpers for codacy-audit scripts.
#
# Token-safe by design: CODACY_TOKEN never reaches the
# assistant-visible stdout. Internal helpers that handle
# credential bytes have leading-underscore names; public
# wrappers read .env internally and emit only the response
# body.
#
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# ANSI colors. Real ESC bytes via $'...' so the variables work
# uniformly with echo -e and printf. Color vars are referenced
# by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
CA_RED=$'\033[0;31m'
# shellcheck disable=SC2034
CA_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
CA_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
CA_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
CA_CYAN=$'\033[0;36m'
# shellcheck disable=SC2034
CA_NC=$'\033[0m'

# Resolve this lib's path (zsh + bash compatible). The query-agent-events
# skill uses the same idiom; mirror it here so sourcing from either shell
# works without warnings.
if [ -n "${ZSH_VERSION-}" ]; then
    eval '_codacyaudit_lib_self="${(%):-%x}"'
elif [ -n "${BASH_VERSION-}" ]; then
    _codacyaudit_lib_self="${BASH_SOURCE[0]}"
else
    _codacyaudit_lib_self="$0"
fi
_codacyaudit_lib_dir="$(cd "$(dirname "$_codacyaudit_lib_self")" && pwd)"

# Locate the repo root from this script's location.
codacyaudit_repo_root() {
    git -C "$_codacyaudit_lib_dir" rev-parse --show-toplevel
}

# Source <repo-root>/.env. CODACY_TOKEN is required for token-gated
# endpoints (issue search across master, repo metadata, future write
# actions). Read-only PR-issue queries also work anonymously, but
# this skill drives them through the token wrapper for consistency
# and to exercise the no-leak self-test on every run.
codacyaudit_load_env() {
    local root env
    root="$(codacyaudit_repo_root)"
    env="${root}/.env"
    if [[ ! -f "${env}" || ! -r "${env}" ]]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} Missing ${env}. See <repo>/.agents/ENV.md for the setup guide." >&2
        return 1
    fi
    set -a
    # shellcheck disable=SC1090
    source "${env}"
    set +a

    : "${CODACY_TOKEN:?CODACY_TOKEN is empty -- see <repo>/.agents/ENV.md to set it.}"
    : "${CODACY_HOST:=https://api.codacy.com}"
    : "${CODACY_PROVIDER:=gh}"
    : "${CODACY_ORG:=netdata}"
    : "${CODACY_REPO:=netdata}"

    export CODACY_TOKEN CODACY_HOST CODACY_PROVIDER CODACY_ORG CODACY_REPO
}

# Audit artifacts go under .local/audits/codacy/ at the repo root.
# .local/ is gitignored -- see AGENTS.md for the convention.
codacyaudit_audit_dir() {
    local root dir
    root="$(codacyaudit_repo_root)"
    dir="${root}/.local/audits/codacy"
    mkdir -p "${dir}"
    echo "${dir}"
}

# ---------------------------------------------------------------
# Token-safe HTTP wrappers.
#
# The internal helper handles the token bytes. Public wrappers
# call it and emit response body only on stdout. We never echo
# the curl command line (which would expose the token).

# _codacyaudit_run METHOD PATH [DATA]
# Returns the response body on stdout. HTTP non-2xx -> non-zero.
# stderr: minimal status line on error.
_codacyaudit_run() {
    local method="$1"
    local path="$2"
    local data="${3:-}"
    local url="${CODACY_HOST}${path}"

    local -a curl_args=(
        --silent --show-error --fail-with-body
        --max-time 60
        --request "$method"
        --header "api-token: ${CODACY_TOKEN}"
        --header 'Accept: application/json'
    )
    if [ -n "$data" ]; then
        curl_args+=(--header 'Content-Type: application/json' --data-raw "$data")
    fi

    local body
    if ! body="$(curl "${curl_args[@]}" "$url")"; then
        echo -e "${CA_RED}[ERROR]${CA_NC} ${method} ${path} failed (see body below)" >&2
        printf '%s\n' "$body" >&2
        return 1
    fi
    printf '%s' "$body"
}

# Public GET. Stdout is the response body; the token never leaks.
codacyaudit_get() {
    local path="$1"
    _codacyaudit_run GET "$path"
}

# Public POST.
codacyaudit_post() {
    local path="$1"
    local data="$2"
    _codacyaudit_run POST "$path" "$data"
}

# Paginated GET. Walks the v3 cursor protocol and concatenates
# `data[]` into a single JSON array on stdout.
#
# Codacy v3 pagination:
#   request : ?cursor=<c>&limit=<n>
#   response: { data: [...], pagination: { cursor, limit, total } }
codacyaudit_get_paged() {
    local path_base="$1"
    local limit="${2:-1000}"
    local sep cursor=""
    local first=true
    local out='[]'

    while :; do
        if [[ "$path_base" == *'?'* ]]; then sep='&'; else sep='?'; fi
        local path="${path_base}${sep}limit=${limit}"
        if [ -n "$cursor" ]; then
            path="${path}&cursor=${cursor}"
        fi

        local resp
        resp="$(_codacyaudit_run GET "$path")" || return 1

        # Append data[] to accumulator.
        out="$(printf '%s\n%s' "$out" "$resp" \
            | jq -sc '.[0] + (.[1].data // [])')"

        cursor="$(printf '%s' "$resp" | jq -r '.pagination.cursor // ""')"
        [ -z "$cursor" ] && break
        $first || [ "$first" = "false" ] # keep loop simple
        first=false
    done

    printf '%s' "$out"
}

# ---------------------------------------------------------------
# Convenience wrappers for the two endpoints this SOW ships.

# PR issues: GET /v3/analysis/organizations/<p>/<o>/repositories/<r>/pull-requests/<n>/issues
codacyaudit_pr_issues() {
    local pr="$1"
    if [[ ! "$pr" =~ ^[1-9][0-9]*$ ]]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} PR number must be a positive integer, got: '${pr}'" >&2
        return 1
    fi
    codacyaudit_get_paged \
      "/api/v3/analysis/organizations/${CODACY_PROVIDER}/${CODACY_ORG}/repositories/${CODACY_REPO}/pull-requests/${pr}/issues"
}

# Repo overview: GET /v3/organizations/<p>/<o>/repositories/<r>
codacyaudit_repo_info() {
    codacyaudit_get \
      "/api/v3/organizations/${CODACY_PROVIDER}/${CODACY_ORG}/repositories/${CODACY_REPO}"
}

# ---------------------------------------------------------------
# No-token-leak self-test.
#
# Drives every public wrapper with a sentinel CODACY_TOKEN and
# asserts the sentinel never appears on captured stdout. Run
# this after editing any wrapper.

codacyaudit_selftest_no_token_leak() {
    local sentinel="deadbeef-1234-5678-9abc-def012345678"

    local saved="${CODACY_TOKEN:-}"
    CODACY_TOKEN="$sentinel"
    export CODACY_TOKEN

    # Drive each public wrapper. The expected outcome is HTTP
    # 401 (sentinel is not a real token); we capture stdout and
    # assert the sentinel does not appear there.
    local out
    out="$( {
        codacyaudit_get  "/api/v3/user"                           2>/dev/null || true
        codacyaudit_post "/api/v3/user" '{"noop":true}'           2>/dev/null || true
        codacyaudit_pr_issues 22423                                2>/dev/null || true
        codacyaudit_repo_info                                      2>/dev/null || true
    } )"

    CODACY_TOKEN="$saved"
    export CODACY_TOKEN

    if printf '%s' "$out" | grep -q "$sentinel"; then
        echo -e "${CA_RED}FAIL${CA_NC}: sentinel ${sentinel} appeared on captured stdout" >&2
        return 1
    fi

    echo -e "${CA_GREEN}PASS${CA_NC}: codacyaudit_selftest_no_token_leak"
    return 0
}
