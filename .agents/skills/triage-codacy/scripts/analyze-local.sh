#!/usr/bin/env bash
# analyze-local.sh -- run codacy-analysis-cli locally on the working tree.
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

Runs the official codacy-analysis-cli (https://github.com/codacy/codacy-analysis-cli)
on the current working tree and writes a JSON dump under
<repo>/.local/audits/codacy/. The cli respects the repo's .codacy.yml
exclude_paths.

Options:
  --tool <name>          run a single tool (e.g. shellcheck, markdownlint).
                         Omit to run all tools applicable to changed files.
  --directory <path>     analyze a subpath (default: <repo-root>)
  --format json|sarif    output format (default: json)
  --output PATH          explicit dump path (default: auto under .local/audits/codacy/)
  --runner docker|local  installer to use (default: auto -- prefer local
                         binary, fall back to docker, fall back to npm)
  -h, --help

Required tools: docker (default) OR a local codacy-analysis-cli binary.
EOF
}

TOOL=
SUBDIR=
FORMAT=json
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
    OUTPUT="${audit_dir}/local${suffix}-$(date -u +%Y%m%dT%H%M%SZ).${FORMAT}"
fi

# Pick a runner.
if [ "$RUNNER" = "auto" ]; then
    if command -v codacy-analysis-cli >/dev/null 2>&1; then
        RUNNER=local
    elif command -v docker >/dev/null 2>&1; then
        RUNNER=docker
    else
        echo -e "${CA_RED}[ERROR]${CA_NC} neither 'codacy-analysis-cli' nor 'docker' found in PATH." >&2
        echo "Install options:" >&2
        echo "  - docker:   https://docs.docker.com/get-docker/" >&2
        echo "  - cli:      https://github.com/codacy/codacy-analysis-cli#install" >&2
        exit 2
    fi
fi

echo -e "${CA_GRAY}[analyze-local] runner=${RUNNER} format=${FORMAT} dir=${SUBDIR}${CA_NC}" >&2

case "$RUNNER" in
    local)
        # Local binary expects host paths.
        local_args=(analyze --directory "$SUBDIR" --format "$FORMAT")
        [ -n "$TOOL" ] && local_args+=(--tool "$TOOL")
        if ! codacy-analysis-cli "${local_args[@]}" > "$OUTPUT" 2>/dev/null; then
            echo -e "${CA_YELLOW}[analyze-local] cli returned non-zero (this is normal when findings are present)${CA_NC}" >&2
        fi
        ;;
    docker)
        # Per https://github.com/codacy/codacy-analysis-cli the CLI
        # spawns one child container per tool and needs:
        #   - the host docker socket (docker-in-docker)
        #   - CODACY_CODE pointing at the host path of the source
        #   - the source bind-mounted at the SAME path inside the
        #     CLI container so child containers can resolve it
        cli_args=(analyze --directory "$SUBDIR" --format "$FORMAT")
        [ -n "$TOOL" ] && cli_args+=(--tool "$TOOL")
        if ! docker run --rm \
                --env CODACY_CODE="$SUBDIR" \
                --volume /var/run/docker.sock:/var/run/docker.sock \
                --volume "$SUBDIR":"$SUBDIR" \
                codacy/codacy-analysis-cli:latest \
                "${cli_args[@]}" > "$OUTPUT" 2>/dev/null; then
            echo -e "${CA_YELLOW}[analyze-local] cli returned non-zero (this is normal when findings are present)${CA_NC}" >&2
        fi
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

# Quick summary if format=json.
if [ "$FORMAT" = "json" ] && jq -e . "$OUTPUT" >/dev/null 2>&1; then
    n="$(jq 'if type=="array" then length elif type=="object" and has("issues") then (.issues|length) else 0 end' "$OUTPUT")"
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${n} finding(s) to ${OUTPUT}" >&2
else
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${OUTPUT} (${FORMAT} format)" >&2
fi

# Last line on stdout: the path. Pipe-friendly.
echo "$OUTPUT"
