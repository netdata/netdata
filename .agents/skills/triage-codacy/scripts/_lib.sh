#!/usr/bin/env bash
# Common helpers for triage-codacy scripts.
#
# Public wrappers use already configured shell values; action
# scripts explicitly call codacyaudit_load_env when needed.
# Credentials are not logged. Successful response bodies are
# forwarded unchanged and can contain private service data.
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

# Resolve this lib's path (zsh + bash compatible). The triage-agent-events
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
# actions). Some public PR endpoints have allowed anonymous reads;
# these scripts use the configured token wrapper. Offline self-tests
# do not load the environment or query a service.
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
    local request_path="$2"
    local data="${3:-}"
    local url="${CODACY_HOST}${request_path}"

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

    local body curl_status
    if body="$(curl "${curl_args[@]}" "$url" 2>/dev/null)"; then
        printf '%s' "$body"
    else
        curl_status=$?
        # A service/proxy can reflect credentials or request data in errors.
        printf '[ERROR] Codacy %s request failed (curl status %s)\n' "$method" "$curl_status" >&2
        return "$curl_status"
    fi
}

# Public GET. Stdout is the successful response body, unchanged.
codacyaudit_get() {
    local request_path="$1"
    _codacyaudit_run GET "$request_path"
}

# Public POST.
codacyaudit_post() {
    local request_path="$1"
    local data="$2"
    _codacyaudit_run POST "$request_path" "$data"
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
        local request_path="${path_base}${sep}limit=${limit}"
        if [ -n "$cursor" ]; then
            request_path="${request_path}&cursor=${cursor}"
        fi

        local resp
        resp="$(_codacyaudit_run GET "$request_path")" || return $?

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
# No-token-leak self-test. Exercise real wrappers with an offline
# transport in a subshell; caller variables/functions remain unchanged.
codacyaudit_selftest_no_token_leak() (
    CODACY_TOKEN="test-codacy-secret"
    CODACY_HOST="https://codacy.example.invalid"
    CODACY_PROVIDER="gh"
    CODACY_ORG="fixture-org"
    CODACY_REPO="fixture-repo"

    curl() {
        local arg url=""
        for arg in "$@"; do url="$arg"; done
        case "$url" in
            "$CODACY_HOST"/api/v3/*cursor=fixture-next)
                printf '%s' '{"data":[{"fixture":"second"}],"pagination":{}}' ;;
            "$CODACY_HOST"/api/v3/*limit=*)
                printf '%s' '{"data":[{"fixture":"first"}],"pagination":{"cursor":"fixture-next"}}' ;;
            "$CODACY_HOST"/api/v3/*)
                printf '%s' '{"fixture":"single"}' ;;
            *) return 97 ;;
        esac
    }

    local wrapper out selftest_status
    for wrapper in get post paged issues repository; do
        if out="$(
            case "$wrapper" in
                get) codacyaudit_get /api/v3/user ;;
                post) codacyaudit_post /api/v3/user '{"noop":true}' ;;
                paged) codacyaudit_get_paged /api/v3/fixture ;;
                issues) codacyaudit_pr_issues 1 ;;
                repository) codacyaudit_repo_info ;;
            esac 2>&1
        )"; then selftest_status=0; else selftest_status=$?; fi
        if [ "$selftest_status" -ne 0 ] || [[ "$out" != *'"fixture"'* || "$out" == *"$CODACY_TOKEN"* ]]; then
            printf 'FAIL: Codacy offline wrapper self-test (%s)\n' "$wrapper" >&2
            return 1
        fi
    done
    printf '%s\n' 'PASS: codacyaudit_selftest_no_token_leak'
)
