#!/usr/bin/env bash
# Common helpers for pr-reviews scripts.
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# IMPORTANT: define with $'...' so the variables contain real ESC bytes,
# not the literal four-character string "\033". This way both `echo -e
# "${PR_RED}..."` and `printf '%s' "${PR_RED}..."` render correctly --
# without forcing every printf format string to be the variable itself
# (which trips shellcheck SC2059) or %b (which adds inconsistency).
#
# Color vars are referenced by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
PR_RED=$'\033[0;31m'
# shellcheck disable=SC2034
PR_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
PR_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
PR_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
PR_NC=$'\033[0m'

pr_repo_root() {
    git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel
}

# Owner/repo of the upstream remote (or origin if no upstream).
# Override with PR_REPO_SLUG=owner/repo for cross-repo work.
# Uses bash parameter expansion so repo names containing dots parse correctly.
# Returns empty if the URL is not a github.com remote (this skill only
# supports GitHub).
pr_repo_slug() {
    if [[ -n "${PR_REPO_SLUG:-}" ]]; then
        # Validate the override too so callers can't smuggle whitespace or
        # shell metacharacters in via env. Allowed: owner/repo with
        # alphanumerics, dot, underscore, hyphen.
        if [[ ! "${PR_REPO_SLUG}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
            echo "" >&2
            return
        fi
        echo "${PR_REPO_SLUG}"
        return
    fi
    local root url
    root="$(pr_repo_root)"
    url="$(git -C "${root}" config --get remote.upstream.url 2>/dev/null \
         || git -C "${root}" config --get remote.origin.url)"
    # Match github.com only as a host -- a substring match would accept
    # `notgithub.com`, `github.com.attacker.example.com`, etc.
    # Accepted forms (covering all common gh / git remote outputs):
    #   git@github.com:owner/repo[.git]           (SCP-style ssh)
    #   ssh://git@github.com/owner/repo[.git]     (URL-style ssh)
    #   https://github.com/owner/repo[.git]       (anonymous https)
    #   https://x-access-token:TOK@github.com/... (credentialed https, gh auth)
    if [[ "${url}" != *@github.com:* \
       && "${url}" != *://github.com/* \
       && "${url}" != *@github.com/* ]]; then
        echo ""
        return
    fi
    url="${url%.git}"               # strip trailing .git
    url="${url#*github.com[:/]}"    # strip everything up to and including github.com:/
    echo "${url}"
}

# Audit/state directory for pr-reviews artifacts.
pr_audit_dir() {
    local root dir
    root="$(pr_repo_root)"
    dir="${root}/.local/audits/pr-reviews"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Per-PR working directory.
pr_state_dir() {
    local pr="${1:?usage: pr_state_dir <pr-number>}"
    local dir
    dir="$(pr_audit_dir)/pr-${pr}"
    mkdir -p "${dir}"
    echo "${dir}"
}

# Verify gh is available and authenticated. Bail loudly otherwise.
pr_require_gh() {
    if ! command -v gh >/dev/null; then
        echo -e "${PR_RED}[ERROR]${PR_NC} 'gh' CLI not installed. https://cli.github.com/" >&2
        return 1
    fi
    if ! gh auth status >/dev/null 2>&1; then
        echo -e "${PR_RED}[ERROR]${PR_NC} 'gh' is not authenticated. Run 'gh auth login'." >&2
        return 1
    fi
}

# Bot logins recognized by the skill. The first regex matches the AI reviewers
# the skill iterates with autonomously; the second matches CI/quality bots
# whose comments are informational (sonar quality gate, etc.).
PR_AI_BOT_RE='^(cubic-dev-ai|copilot|copilot-pull-request-reviewer|github-copilot)\[bot\]$'
PR_INFO_BOT_RE='^(sonarqubecloud|netdata-bot|github-actions|coderabbitai)\[bot\]$'

# Classify a login -> "ai_bot" | "info_bot" | "human".
pr_classify_author() {
    local login="$1"
    if [[ "${login}" =~ ${PR_AI_BOT_RE} ]]; then
        echo "ai_bot"
    elif [[ "${login}" =~ ${PR_INFO_BOT_RE} ]]; then
        echo "info_bot"
    else
        echo "human"
    fi
}

# Pretty timestamp in UTC.
pr_now_utc() {
    date -u +%Y-%m-%dT%H:%M:%SZ
}

# PR numbers are positive integers (GitHub assigns 1+). Reject anything
# else before interpolating into REST paths or `gh` arguments. Even though
# URL interpolation isn't a shell-injection vector, malformed input causes
# confusing API errors that look like permission/auth issues.
pr_require_numeric() {
    local n="$1" name="${2:-PR}"
    if [[ ! "${n}" =~ ^[1-9][0-9]*$ ]]; then
        echo -e "${PR_RED}[ERROR]${PR_NC} ${name} must be a positive integer, got: '${n}'" >&2
        return 1
    fi
}

# Resolve and validate the repo slug. Returns "owner/repo" on stdout or
# exits non-zero if no slug could be derived (no remotes configured, or
# the URL didn't match github.com).
pr_require_slug() {
    local slug
    slug="$(pr_repo_slug)"
    if [[ -z "${slug}" || "${slug}" != */* ]]; then
        echo -e "${PR_RED}[ERROR]${PR_NC} could not derive owner/repo from git remotes (got: '${slug}'). This skill only supports github.com remotes; set PR_REPO_SLUG=owner/repo or fix the remote URL." >&2
        return 1
    fi
    printf '%s' "${slug}"
}
