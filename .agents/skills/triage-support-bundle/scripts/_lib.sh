#!/usr/bin/env bash
# Common helpers for triage-support-bundle scripts.
#
# bundle-summary.sh reads a bundle from the local filesystem and makes no
# network calls. analyze-bundle.sh additionally calls an OpenAI-compatible
# endpoint, so this lib carries a credential.
#
# Token safety: helpers that touch credential bytes are prefixed with an
# underscore, are internal-only, and return values through namerefs into the
# caller's locals - never to stdout. The public wrapper emits only the response
# body. sb_selftest_no_token_leak drives the wrapper with a sentinel and asserts
# the sentinel never reaches captured output.
#
# The bundle CONTENT is customer data - callers keep output under the skill's
# audit directory and never publish it.
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

# True when $2 resolves to $1 or something beneath it. pwd -P resolves symlinks,
# so a path that pivots out through a link fails this test.
_sb_path_inside() {
    local root_real target_real
    root_real="$(cd "$1" 2>/dev/null && pwd -P)" || return 1
    target_real="$(cd "$2" 2>/dev/null && pwd -P)" || return 1
    case "$target_real" in
        "$root_real"|"$root_real"/*) return 0 ;;
        *) return 1 ;;
    esac
}

# Windows bundles produced under PowerShell 5.1 store zip entries with
# backslash separators. The ZIP spec requires forward slashes, so unzip treats
# each entry as one flat filename containing backslashes and the bundle tree
# never materialises. Rebuild the tree so such a bundle is readable here.
#
# Security: unzip refuses traversal in real path components, but a backslash
# entry reaches it as ONE flat filename, so its own guard never applies. Undoing
# the escaping therefore has to re-apply that guard: an entry normalising to an
# absolute path or containing a ".." component is rejected, not moved. Without
# this, a crafted bundle could write anywhere the invoking user can.
_sb_flatten_backslash_entries() {
    local root="$1" f rel dest seg parent
    # A support bundle never legitimately contains a symlink: the producer
    # withholds symlinked sources rather than following them. Anything of that
    # shape in the archive is a pivot for escaping the tree, so drop it first.
    find "$root" -type l -exec rm -f {} + 2>/dev/null || true
    while IFS= read -r -d '' f; do
        rel="${f#"$root"/}"
        case "$rel" in
            *\\*) : ;;
            *) continue ;;
        esac
        rel="${rel//\\//}"

        local unsafe=0
        case "$rel" in
            /*|[A-Za-z]:/*) unsafe=1 ;;
        esac
        local IFS_SAVE="$IFS"; IFS='/'
        for seg in $rel; do
            [ "$seg" = ".." ] && unsafe=1
        done
        IFS="$IFS_SAVE"

        if [ "$unsafe" -eq 1 ]; then
            sb_warn "rejected unsafe bundle entry (path traversal): ${rel}"
            rm -f "$f"
            continue
        fi

        dest="${root}/${rel}"
        # Rejecting ".." is not sufficient on its own: an archive can also ship a
        # symlink entry and then a backslash entry that normalises THROUGH it,
        # so a purely lexical check still lands outside the tree. Create the
        # parent, then confirm its resolved path is still inside root.
        # mkdir is guarded: under errexit a failure here would abort before the
        # caller installs its cleanup trap, stranding extracted customer data.
        parent="$(dirname "$dest")"
        if ! mkdir -p "$parent" 2>/dev/null; then
            sb_warn "could not create ${parent}; dropping entry: ${rel}"
            rm -f "$f"
            continue
        fi
        if ! _sb_path_inside "$root" "$parent"; then
            sb_warn "rejected unsafe bundle entry (escapes via a link): ${rel}"
            rm -f "$f"
            continue
        fi
        mv -f "$f" "$dest"
    done < <(find "$root" -maxdepth 1 -type f -name '*\\*' -print0 2>/dev/null)
}

# Resolve a bundle argument to a directory containing MANIFEST.json.
#
# Accepts an already-extracted directory, or an archive which is extracted into
# a caller-owned scratch directory. Sets SB_BUNDLE_DIR and SB_BUNDLE_TMP (empty
# when nothing was extracted, so the caller knows whether to clean up).
sb_resolve_bundle() {
    local input="$1" tmp root
    # These are this function's return values: the sourcing scripts read them.
    # shellcheck disable=SC2034
    SB_BUNDLE_DIR=""
    SB_BUNDLE_TMP=""

    [ -e "$input" ] || sb_die "no such bundle: ${input}"
    # A FIFO or device passes -e and then blocks tar/unzip forever.
    [ -d "$input" ] || [ -f "$input" ] \
        || sb_die "not a regular file or directory: ${input}"
    [ -d "$input" ] || [ -s "$input" ] || sb_die "empty bundle file: ${input}"
    [ -r "$input" ] || sb_die "not readable: ${input}"

    if [ -d "$input" ]; then
        # A symlinked manifest or artifact would send every later read outside
        # the bundle. A real bundle contains no links, so refuse rather than follow.
        # No depth limit: bundle-summary.sh resolves every manifest path under
        # this root, so a link at any depth can redirect one of those reads.
        if [ -n "$(find "$input" -type l -print -quit 2>/dev/null)" ]; then
            sb_die "bundle directory contains symlinks; refusing to follow them: ${input}"
        fi
        if [ -f "${input}/MANIFEST.json" ] && [ ! -h "${input}/MANIFEST.json" ]; then
            # shellcheck disable=SC2034
            SB_BUNDLE_DIR="$input"
        else
            # a directory holding a single extracted bundle
            root="$(find "$input" -maxdepth 2 -name MANIFEST.json -print -quit)"
            [ -n "$root" ] || sb_die "no MANIFEST.json under ${input}"
            # shellcheck disable=SC2034
            SB_BUNDLE_DIR="$(dirname "$root")"
        fi
        return 0
    fi

    tmp="$(mktemp -d "${TMPDIR:-/tmp}/netdata-bundle.XXXXXX")"
    SB_BUNDLE_TMP="$tmp"
    # Callers install their cleanup trap only after this function returns, so
    # every failure below has to remove its own scratch directory.
    _sb_resolve_fail() { rm -rf "$tmp"; SB_BUNDLE_TMP=""; sb_die "$@"; }
    case "$input" in
        *.tar.zst)
            sb_need tar
            tar --zstd -xf "$input" -C "$tmp" 2>/dev/null \
                || { command -v zstd >/dev/null 2>&1 || _sb_resolve_fail "'zstd' is required to read ${input}"
                     zstd -dc "$input" | tar -xf - -C "$tmp"; } \
                || _sb_resolve_fail "could not extract ${input}" ;;
        *.tar.gz|*.tgz) sb_need tar; tar -xzf "$input" -C "$tmp" || _sb_resolve_fail "could not extract ${input}" ;;
        *.zip)
            sb_need unzip
            # unzip exits 1 for warnings, which is what a backslash-separator
            # bundle produces. Anything above that is a real extraction failure
            # (corrupt archive, CRC error) and must not be analysed as if it
            # were complete - the parity check compares paths, not contents, so
            # it would happily report parity over truncated files.
            set +e; unzip -q "$input" -d "$tmp" 2>/dev/null; _sb_rc=$?; set -e
            if [ "$_sb_rc" -gt 1 ]; then
                _sb_resolve_fail "zip extraction failed (unzip exit ${_sb_rc}); the archive is damaged or truncated."
            fi
            _sb_flatten_backslash_entries "$tmp" ;;
        *)              _sb_resolve_fail "unrecognized bundle format: ${input}" ;;
    esac

    # tar archives can carry links too; the zip path already sweeps them.
    find "$tmp" -type l -exec rm -f {} + 2>/dev/null || true
    root="$(find "$tmp" -maxdepth 3 -type f -name MANIFEST.json -print -quit)"
    [ -n "$root" ] || _sb_resolve_fail "no MANIFEST.json inside ${input}"
    # shellcheck disable=SC2034
    SB_BUNDLE_DIR="$(dirname "$root")"
}

sb_cleanup_bundle() {
    [ -n "${SB_BUNDLE_TMP:-}" ] && [ -d "${SB_BUNDLE_TMP}" ] && rm -rf "${SB_BUNDLE_TMP}"
    return 0
}

# --------------------------------------------------------------------------
# Model endpoint (OpenAI-compatible)
# --------------------------------------------------------------------------

# Load the model configuration from <repo>/.env into the caller's locals.
# Internal: the third nameref receives the API key and must stay a local.
# Usage: local ep mo key; _sb_load_llm_env ep mo key
_sb_load_llm_env() {
    local -n _ep_ref="$1" _model_ref="$2" _key_ref="$3"
    local root env
    root="$(sb_repo_root)"
    env="${root}/.env"
    [ -f "$env" ] && [ -r "$env" ] \
        || sb_die "missing ${env}. See <repo>/.agents/ENV.md for the setup guide."
    set -a
    # shellcheck disable=SC1090
    . "$env"
    set +a
    : "${NETDATA_LLM_ENDPOINT:?NETDATA_LLM_ENDPOINT is empty - see .agents/ENV.md}"
    : "${NETDATA_LLM_MODEL:?NETDATA_LLM_MODEL is empty - see .agents/ENV.md}"
    : "${NETDATA_LLM_API_KEY:?NETDATA_LLM_API_KEY is empty - see .agents/ENV.md}"
    _ep_ref="${NETDATA_LLM_ENDPOINT%/}"
    _model_ref="${NETDATA_LLM_MODEL}"
    _key_ref="${NETDATA_LLM_API_KEY}"
    # do not leave the credential in the environment of anything we spawn
    unset NETDATA_LLM_API_KEY
}

# POST a chat-completions request body and emit ONLY the assistant message
# content on stdout. The credential is passed to curl through a header file on
# a private descriptor, so it never appears in the process table, in the shell
# trace, or in any diagnostic this script prints.
#
# Usage: sb_llm_chat <request-body.json>            (config from .env)
#        SB_LLM_ENDPOINT/_MODEL/_KEY may be preset to override .env (self-test).
sb_llm_chat() {
    local body="$1"
    local ep model key hdr rc out
    [ -f "$body" ] || sb_die "no such request body: ${body}"

    if [ -n "${SB_LLM_ENDPOINT:-}" ] && [ -n "${SB_LLM_KEY:-}" ]; then
        ep="${SB_LLM_ENDPOINT%/}"; model="${SB_LLM_MODEL:-unset}"; key="${SB_LLM_KEY}"
    else
        _sb_load_llm_env ep model key
    fi

    hdr="$(mktemp "${TMPDIR:-/tmp}/sb-hdr.XXXXXX")"
    chmod 600 "$hdr"
    printf 'Authorization: Bearer %s\n' "$key" > "$hdr"
    unset key

    out="$(mktemp "${TMPDIR:-/tmp}/sb-resp.XXXXXX")"
    set +e
    curl -sS --max-time "${SB_LLM_TIMEOUT:-180}" \
        -H @"$hdr" -H 'Content-Type: application/json' \
        --data-binary @"$body" \
        "${ep}/v1/chat/completions" -o "$out"
    rc=$?
    set -e
    rm -f "$hdr"

    if [ "$rc" -ne 0 ]; then
        rm -f "$out"
        sb_die "model request failed (curl exit ${rc}); endpoint or network unreachable."
    fi
    if jq -e '.error' "$out" >/dev/null 2>&1; then
        local msg; msg="$(jq -r '.error.message // "unknown error"' "$out")"
        rm -f "$out"
        sb_die "model endpoint returned an error: ${msg}"
    fi
    jq -r '.choices[0].message.content // empty' "$out"
    rm -f "$out"
}

# Drive the public wrapper with a sentinel credential and assert the sentinel
# never reaches captured stdout or stderr. Runs offline against an endpoint that
# cannot resolve; the failure path is exactly where a naive implementation would
# echo the request it tried to make.
sb_selftest_no_token_leak() {
    local sentinel='SENTINEL-TOKEN-do-not-log-0000' body captured rc=0
    body="$(mktemp "${TMPDIR:-/tmp}/sb-selftest.XXXXXX")"
    printf '{"model":"selftest","messages":[{"role":"user","content":"ping"}]}\n' > "$body"

    # sb_die exits the substitution subshell outright, so an inner "|| true"
    # would never run; the guard belongs on the assignment itself.
    captured="$(
        SB_LLM_ENDPOINT='http://127.0.0.1:1' \
        SB_LLM_MODEL='selftest' \
        SB_LLM_KEY="$sentinel" \
        sb_llm_chat "$body" 2>&1
    )" || true
    rm -f "$body"

    if printf '%s' "$captured" | grep -qF "$sentinel"; then
        echo -e "${SB_RED}FAIL${SB_NC} sentinel credential reached captured output" >&2
        rc=1
    else
        echo -e "${SB_GREEN}PASS${SB_NC} credential never reaches stdout or stderr"
    fi
    return "$rc"
}
