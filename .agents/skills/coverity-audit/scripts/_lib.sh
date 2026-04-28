#!/usr/bin/env bash
# Common helpers for coverity-audit scripts.
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# ANSI colors for transparent output (per the project's run() pattern).
#
# IMPORTANT: define with $'...' so the variables contain real ESC bytes,
# not the literal four-character string "\033". This way both `echo -e
# "${COV_RED}..."` and `printf '%s' "${COV_RED}..."` render correctly --
# without forcing every printf format string to be the variable itself
# (which trips shellcheck SC2059) or %b (which adds inconsistency).
#
# Color vars are referenced by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
COV_RED=$'\033[0;31m'
# shellcheck disable=SC2034
COV_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
COV_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
COV_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
COV_NC=$'\033[0m'

# Locate the repo root by walking up from the script directory.
# This way the scripts work no matter where the user runs them from.
cov_repo_root() {
    git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel
}

# Source `<repo-root>/.env` if it exists. .env is the user's local-only
# secrets file (gitignored). It must export at least:
#   COVERITY_COOKIE        — the full Cookie header value pasted from a curl
#                            copy-as-cURL captured in DevTools
#   COVERITY_PROJECT_ID    — Coverity Scan numeric projectId (constant per project)
#   COVERITY_HOST          — defaults to https://scan4.scan.coverity.com
# Optional:
#   COVERITY_VIEW_OUTSTANDING — viewId of the Outstanding view
#   COVERITY_USER_AGENT      — overridable UA string
cov_load_env() {
    local root env
    root="$(cov_repo_root)"
    env="${root}/.env"
    if [[ ! -f "${env}" || ! -r "${env}" ]]; then
        echo -e "${COV_RED}[ERROR]${COV_NC} Missing ${env}. See SKILL.md for the .env template." >&2
        return 1
    fi
    set -a
    # shellcheck disable=SC1090
    source "${env}"
    set +a

    : "${COVERITY_COOKIE:?COVERITY_COOKIE is empty in .env — paste a fresh cookie from the browser}"
    : "${COVERITY_PROJECT_ID:?COVERITY_PROJECT_ID is empty in .env}"
    if [[ ! "${COVERITY_PROJECT_ID}" =~ ^[1-9][0-9]*$ ]]; then
        echo -e "${COV_RED}[ERROR]${COV_NC} COVERITY_PROJECT_ID must be a positive integer, got: '${COVERITY_PROJECT_ID}'" >&2
        return 1
    fi
    : "${COVERITY_HOST:=https://scan4.scan.coverity.com}"
    : "${COVERITY_USER_AGENT:=Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36}"
    export COVERITY_COOKIE COVERITY_PROJECT_ID COVERITY_HOST COVERITY_USER_AGENT

    # Extract XSRF-TOKEN from the cookie string.
    COVERITY_XSRF="$(printf '%s' "${COVERITY_COOKIE}" | sed -n 's/.*XSRF-TOKEN=\([^;]*\).*/\1/p')"
    if [[ -z "${COVERITY_XSRF}" ]]; then
        echo -e "${COV_RED}[ERROR]${COV_NC} XSRF-TOKEN not found inside COVERITY_COOKIE. Did you paste the full Cookie header?" >&2
        return 1
    fi
    export COVERITY_XSRF
}

# Audit artifacts go under .local/audits/coverity/ at the repo root.
# .local/ is gitignored -- see AGENTS.md for the convention.
# Creates the directory on first call so callers can redirect output into
# subpaths without thinking about it.
cov_audit_dir() {
    local root dir
    root="$(cov_repo_root)"
    dir="${root}/.local/audits/coverity"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Reject non-ASCII bytes in a string. Coverity's edge (Cloudflare) rejects
# em-dashes and smart quotes with a 403 challenge; far better to fail before
# the network round-trip than to debug a Cloudflare block.
#
# `tr -d '\000-\177'` deletes ALL ASCII bytes; anything left is non-ASCII.
# This is portable across GNU and BSD/macOS (unlike `grep -P`, which is GNU-only).
cov_require_ascii() {
    local s="$1"
    if LC_ALL=C printf '%s' "${s}" | LC_ALL=C tr -d '\000-\177' | grep -q .; then
        echo -e "${COV_RED}[ERROR]${COV_NC} Comment contains non-ASCII characters. Cloudflare blocks them. Replace em-dashes with '--' and curly quotes with straight quotes." >&2
        return 1
    fi
}

# CID validation: Coverity CIDs are positive integers (>= 1). Reject anything
# else before interpolating into jq filters, URLs, or paths.
cov_require_numeric_cid() {
    local cid="$1"
    if [[ ! "${cid}" =~ ^[1-9][0-9]*$ ]]; then
        echo -e "${COV_RED}[ERROR]${COV_NC} CID must be a positive integer, got: '${cid}'" >&2
        return 1
    fi
}
