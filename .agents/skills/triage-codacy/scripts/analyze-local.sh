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
                         binary, fall back to docker)
  -h, --help

Required tools: docker (default) OR a local codacy-analysis-cli binary.
CODACY_CLI_VERSION=<tag> in the environment pins the docker image tag (default: latest);
this script does not read .env.
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

case "$FORMAT" in
    json|sarif) ;;
    *) echo -e "${CA_RED}[ERROR]${CA_NC} unknown --format '${FORMAT}' (expected json or sarif)" >&2; exit 2 ;;
esac

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
        rc=0
        codacy-analysis-cli "${local_args[@]}" > "$OUTPUT" 2> "$OUTPUT.log" || rc=$?
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
        # The container gets the host docker socket, so a moving tag is a supply-chain
        # surface; CODACY_CLI_VERSION lets a caller pin it without changing the default.
        rc=0
        docker run --rm \
                --env CODACY_CODE="$SUBDIR" \
                --volume /var/run/docker.sock:/var/run/docker.sock \
                --volume "$SUBDIR":"$SUBDIR" \
                "codacy/codacy-analysis-cli:${CODACY_CLI_VERSION:-latest}" \
                "${cli_args[@]}" > "$OUTPUT" 2> "$OUTPUT.log" || rc=$?
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
    rm -f "$OUTPUT.log"  # the CLI's stderr is kept only when the run failed
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
    rm -f "$OUTPUT.log"
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${n} finding(s) to ${OUTPUT} (${FORMAT} format)" >&2
fi

# Last line on stdout: the path. Pipe-friendly.
echo "$OUTPUT"
