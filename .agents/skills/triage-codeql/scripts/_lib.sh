#!/usr/bin/env bash
# Common helpers for triage-codeql scripts.
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# IMPORTANT: define with $'...' so the variables contain real ESC bytes,
# not the literal four-character string "\033". This way both `echo -e
# "${GH_RED}..."` and `printf '%s' "${GH_RED}..."` render correctly --
# without forcing every printf format string to be the variable itself
# (which trips shellcheck SC2059) or %b (which adds inconsistency).
#
# Color vars are referenced by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
GH_RED=$'\033[0;31m'
# shellcheck disable=SC2034
GH_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
GH_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
GH_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
GH_NC=$'\033[0m'

gh_repo_root() {
    # Walk up from this _lib.sh; that's stable regardless of caller layout.
    git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel
}

gh_repo_slug() {
    # Resolve repository identity for gh API calls, not clone transport or SSH login validity.
    # The configured remote transport and username are not used for the API request.
    # Parse the authority and exactly two path components; a GitHub-looking
    # substring in another host's path is not a GitHub remote.
    local root url slug
    root="$(gh_repo_root)"
    url="$(git -C "${root}" config --get remote.upstream.url 2>/dev/null \
         || git -C "${root}" config --get remote.origin.url)"
    if [[ "${url}" =~ ^[^/@:[:space:]]+@github\.com:([A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+)$ ]]; then
        slug="${BASH_REMATCH[1]}"
    elif [[ "${url}" =~ ^(https?|ssh)://([^/@?#[:space:]]+@)?github\.com/([A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.-]+)$ ]]; then
        slug="${BASH_REMATCH[3]}"
    else
        return 0
    fi
    slug="${slug%.git}"
    case "${slug#*/}" in ''|.|..) return 0 ;; esac
    printf '%s\n' "${slug}"
}

# Resolve and validate the repo slug. Returns "owner/repo" on stdout or
# exits non-zero if no slug could be derived.
gh_require_slug() {
    local slug
    slug="$(gh_repo_slug)"
    if [[ -z "${slug}" || "${slug}" != */* ]]; then
        echo -e "${GH_RED}[ERROR]${GH_NC} could not derive owner/repo from git remotes (got: '${slug}'). Fix the upstream/origin remote URL." >&2
        return 1
    fi
    printf '%s' "${slug}"
}

gh_audit_dir() {
    local root dir
    root="$(gh_repo_root)"
    # Kept across the skill rename so per-user caches survive; see AGENTS.md, Local-Only Working Directory.
    dir="${root}/.local/audits/graphql"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Run gh against the GitHub API using gh-managed credentials or environment auth.
# No token is required in .env when using the gh CLI directly.
gh_api() {
    if ! command -v gh >/dev/null; then
        echo -e "${GH_RED}[ERROR]${GH_NC} 'gh' CLI is not installed. Install from https://cli.github.com/." >&2
        return 1
    fi
    gh "$@"
}
