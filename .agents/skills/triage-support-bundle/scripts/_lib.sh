#!/usr/bin/env bash
# Common helpers for triage-support-bundle scripts.
#
# These scripts read a support bundle from the local filesystem. They make no
# network calls and handle no credentials, so this lib intentionally ships no
# load_env and no masked-token run wrappers: there is no token to mask. The
# bundle CONTENT is customer data - callers keep output under the skill's audit
# directory and never publish it.
#
# Sourced from the per-action scripts; not executed directly.

set -euo pipefail

# ANSI colors. Real ESC bytes via $'...' so the variables work uniformly with
# echo -e and printf. Referenced by sourcing scripts; shellcheck cannot see that.
# shellcheck disable=SC2034
SB_RED=$'\033[0;31m'
# shellcheck disable=SC2034
SB_GREEN=$'\033[0;32m'
# shellcheck disable=SC2034
SB_YELLOW=$'\033[1;33m'
# shellcheck disable=SC2034
SB_GRAY=$'\033[0;90m'
# shellcheck disable=SC2034
SB_CYAN=$'\033[0;36m'
# shellcheck disable=SC2034
SB_NC=$'\033[0m'

# Resolve this lib's path (zsh + bash compatible), mirroring the idiom the other
# skills use so sourcing from either shell works without warnings.
if [ -n "${ZSH_VERSION-}" ]; then
    eval '_sb_lib_self="${(%):-%x}"'
elif [ -n "${BASH_VERSION-}" ]; then
    _sb_lib_self="${BASH_SOURCE[0]}"
else
    _sb_lib_self="$0"
fi
_sb_lib_dir="$(cd "$(dirname "$_sb_lib_self")" && pwd)"

sb_repo_root() {
    git -C "$_sb_lib_dir" rev-parse --show-toplevel
}

# <repo>/.local/audits/support-bundle/ - see AGENTS.md#local-only-working-directory.
sb_audit_dir() {
    local root dir
    root="$(sb_repo_root)"
    dir="${root}/.local/audits/support-bundle"
    mkdir -p "${dir}"
    printf '%s\n' "${dir}"
}

sb_die() {
    echo -e "${SB_RED}[ERROR]${SB_NC} $*" >&2
    exit 1
}

sb_warn() {
    echo -e "${SB_YELLOW}[WARN]${SB_NC} $*" >&2
}

sb_need() {
    command -v "$1" >/dev/null 2>&1 || sb_die "'$1' is required but not installed."
}

# Resolve a bundle argument to a directory containing MANIFEST.json.
#
# Accepts an already-extracted directory, or an archive which is extracted into
# a caller-owned scratch directory. Sets SB_BUNDLE_DIR and SB_BUNDLE_TMP (empty
# when nothing was extracted, so the caller knows whether to clean up).
sb_resolve_bundle() {
    local input="$1" tmp root
    SB_BUNDLE_DIR=""
    SB_BUNDLE_TMP=""

    [ -e "$input" ] || sb_die "no such bundle: ${input}"

    if [ -d "$input" ]; then
        if [ -f "${input}/MANIFEST.json" ]; then
            SB_BUNDLE_DIR="$input"
        else
            # a directory holding a single extracted bundle
            root="$(find "$input" -maxdepth 2 -name MANIFEST.json -print -quit)"
            [ -n "$root" ] || sb_die "no MANIFEST.json under ${input}"
            SB_BUNDLE_DIR="$(dirname "$root")"
        fi
        return 0
    fi

    tmp="$(mktemp -d "${TMPDIR:-/tmp}/netdata-bundle.XXXXXX")"
    SB_BUNDLE_TMP="$tmp"
    case "$input" in
        *.tar.zst)
            sb_need tar
            tar --zstd -xf "$input" -C "$tmp" 2>/dev/null \
                || { sb_need zstd; zstd -dc "$input" | tar -xf - -C "$tmp"; } ;;
        *.tar.gz|*.tgz) sb_need tar; tar -xzf "$input" -C "$tmp" ;;
        *.zip)          sb_need unzip; unzip -q "$input" -d "$tmp" ;;
        *)              sb_die "unrecognized bundle format: ${input}" ;;
    esac

    root="$(find "$tmp" -maxdepth 3 -name MANIFEST.json -print -quit)"
    [ -n "$root" ] || sb_die "no MANIFEST.json inside ${input}"
    SB_BUNDLE_DIR="$(dirname "$root")"
}

sb_cleanup_bundle() {
    [ -n "${SB_BUNDLE_TMP:-}" ] && [ -d "${SB_BUNDLE_TMP}" ] && rm -rf "${SB_BUNDLE_TMP}"
    return 0
}
