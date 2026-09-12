#!/usr/bin/env bash
# analyze-local.sh -- run Codacy CLI v2 locally on the working tree.
#
# Mirrors what Codacy CI would run on the same source. Useful BEFORE
# `git push` to catch findings in seconds, not minutes.
#
# Output: a JSON dump under <repo>/.local/audits/codacy/.
# stdout (last line): the dump path.

set -euo pipefail

usage() {
    cat <<'EOF'
analyze-local.sh [options]

Runs Codacy CLI v2 (https://github.com/codacy/codacy-cli-v2) on the target
directory and writes a dump under
<repo>/.local/audits/codacy/.

Analysis is full-tree: every file under <target> is eligible, not only the files
changed on the branch. The CLI reads <target>/.codacy/codacy.yaml, which must be
a reviewed file -- this script never generates configuration.

Options:
  --tool <name>          run a single local CLI tool (e.g. shellcheck,
                         markdownlint). These are the CLI's own analyzers, not
                         Codacy Cloud parity; omit to run every tool configured
                         for the target tree.
  --directory <path>     analyze a subpath (default: <repo-root>)
  --format json|sarif    output format (default: sarif)
  --output PATH          explicit dump path (default: auto under .local/audits/codacy/)
  --runner local         Codacy CLI v2 executable to use (default: auto)
  -h, --help

Required tool: a local codacy-cli executable and a reviewed
<target>/.codacy/codacy.yaml. Set CODACY_CLI_V2_VERSION to verify the installed
executable before analysis; this script does not download, upgrade, or initialize
tools or configuration.
EOF
}

TOOL=
SUBDIR=
FORMAT=sarif
OUTPUT=
RUNNER=auto

while [ $# -gt 0 ]; do
    case "$1" in
        --tool)      TOOL="$2"; shift 2 ;;
        --directory) SUBDIR="$2"; shift 2 ;;
        --format)    FORMAT="$2"; shift 2 ;;
        --output)    OUTPUT="$2"; shift 2 ;;
        --runner)    RUNNER="$2"; shift 2 ;;
        -h|--help)   usage; exit 0 ;;
        *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
    esac
done

# shellcheck source=SCRIPTDIR/_lib.sh disable=SC1091
source "$(cd "$(dirname "$0")" && pwd)/_lib.sh"

# We do not require CODACY_TOKEN here -- the CLI runs without it
# for read-only local analysis. Skip env load to avoid forcing
# users without a token to set one just to run pre-push checks.

repo_root="$(codacyaudit_repo_root)"
audit_dir="$(codacyaudit_audit_dir)"

# Resolve target directory.
if [ -z "$SUBDIR" ]; then
    SUBDIR="$repo_root"
else
    case "$SUBDIR" in
        /*) : ;;                       # absolute
        *)  SUBDIR="$(cd "$SUBDIR" && pwd)" ;;
    esac
fi

# Resolve output path.
if [ -z "$OUTPUT" ]; then
    suffix=""
    [ -n "$TOOL" ] && suffix="-${TOOL}"
    OUTPUT="${audit_dir}/local${suffix}-$(date -u +%Y%m%dT%H%M%SZ)-$$.${FORMAT}"
fi

OUTPUT_DIR="$(dirname "$OUTPUT")"
mkdir -p "$OUTPUT_DIR"
OUTPUT="$(cd "$OUTPUT_DIR" && pwd)/$(basename "$OUTPUT")"

case "$FORMAT" in
    json|sarif) ;;
    *) echo -e "${CA_RED}[ERROR]${CA_NC} unknown --format '${FORMAT}' (expected json or sarif)" >&2; exit 2 ;;
esac

# Pick a runner.
if [ "$RUNNER" = "auto" ]; then
    if command -v codacy-analysis-cli >/dev/null 2>&1; then
        echo -e "${CA_RED}[ERROR]${CA_NC} legacy codacy-analysis-cli found; install Codacy CLI v2 as 'codacy-cli'." >&2
        exit 2
    elif command -v codacy-cli >/dev/null 2>&1; then
        RUNNER=local
    else
        echo -e "${CA_RED}[ERROR]${CA_NC} 'codacy-cli' (Codacy CLI v2) not found in PATH." >&2
        echo "Install: https://github.com/codacy/codacy-cli-v2#installation" >&2
        exit 2
    fi
fi

echo -e "${CA_GRAY}[analyze-local] runner=${RUNNER} format=${FORMAT} dir=${SUBDIR}${CA_NC}" >&2

case "$RUNNER" in
    local)
        if [ -n "${CODACY_CLI_V2_VERSION:-}" ]; then
            installed_version="$(codacy-cli version 2>/dev/null || true)"
            case "$installed_version" in
                *"${CODACY_CLI_V2_VERSION}"*) ;;
                *) echo -e "${CA_RED}[ERROR]${CA_NC} codacy-cli version does not match CODACY_CLI_V2_VERSION=${CODACY_CLI_V2_VERSION}" >&2; exit 2 ;;
            esac
        fi
        if [ ! -f "$SUBDIR/.codacy/codacy.yaml" ]; then
            echo -e "${CA_RED}[ERROR]${CA_NC} no reviewed config at ${SUBDIR}/.codacy/codacy.yaml" >&2
            echo "Create it once with 'codacy-cli init' (add --api-token/--provider/--organization/--repository for Codacy Cloud parity), review it, and commit or keep it as a reviewed artifact; this script never generates configuration." >&2
            exit 2
        fi
        (cd "$SUBDIR" && codacy-cli install) >"$OUTPUT.install.log" 2>&1
        local_args=(analyze "$SUBDIR" --format "$FORMAT" --output "$OUTPUT")
        [ -n "$TOOL" ] && local_args+=(--tool "$TOOL")
        rc=0
        codacy-cli "${local_args[@]}" > "$OUTPUT.stdout.log" 2> "$OUTPUT.log" || rc=$?
        ;;
    *)
        echo -e "${CA_RED}[ERROR]${CA_NC} unknown --runner '${RUNNER}'" >&2
        exit 2
        ;;
esac

# Sanity check the output.
if [ ! -s "$OUTPUT" ]; then
    echo -e "${CA_RED}[ERROR]${CA_NC} empty output at ${OUTPUT}; check the runner above" >&2
    exit 1
fi

# The CLI exits non-zero both when findings are present and when the analysis failed or
# partially failed (tool-runner logs instead of results). Only the first may pass: a run
# that produced no results must never be reported as a clean tree.
if [ "$FORMAT" = "json" ]; then
    if ! jq -e . "$OUTPUT" >/dev/null 2>&1; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc}: ${OUTPUT} is not JSON; the analysis did not complete (stderr: ${OUTPUT}.log)" >&2
        exit 4
    fi
    if ! jq -e 'type=="array" or (type=="object" and (.issues|type)=="array")' "$OUTPUT" >/dev/null 2>&1; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc}: ${OUTPUT} is JSON but not a findings array or an {issues} object" >&2
        exit 4
    fi
    n="$(jq 'if type=="array" then length else (.issues|length) end' "$OUTPUT")"
    if [ "$rc" -ne 0 ] && [ "$n" -eq 0 ]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc} with 0 findings: a failed analysis, not a clean tree" >&2
        exit 4
    fi
    if [ "$rc" -ne 0 ]; then
        echo -e "${CA_YELLOW}[analyze-local] cli exit ${rc} (expected when findings are present)${CA_NC}" >&2
    fi
    rm -f "$OUTPUT.log" "$OUTPUT.stdout.log" "$OUTPUT.install.log"
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${n} finding(s) to ${OUTPUT}" >&2
else
    # SARIF is JSON too; count results across runs so the same failed-versus-clean rule applies.
    if ! jq -e . "$OUTPUT" >/dev/null 2>&1; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc}: ${OUTPUT} is not parseable ${FORMAT}; the analysis did not complete (stderr: ${OUTPUT}.log)" >&2
        exit 4
    fi
    if ! jq -e 'type=="object" and (.runs|type)=="array"' "$OUTPUT" >/dev/null 2>&1; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc}: ${OUTPUT} is JSON but not a SARIF document (stderr: ${OUTPUT}.log)" >&2
        exit 4
    fi
    n="$(jq '[.runs[]?.results[]?] | length' "$OUTPUT")"
    if [ "$rc" -ne 0 ] && [ "$n" -eq 0 ]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cli exit ${rc} with 0 findings: a failed analysis, not a clean tree" >&2
        exit 4
    fi
    rm -f "$OUTPUT.log" "$OUTPUT.stdout.log" "$OUTPUT.install.log"
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${n} finding(s) to ${OUTPUT} (${FORMAT} format)" >&2
fi

# Last line on stdout: the path. Pipe-friendly.
echo "$OUTPUT"
