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
RUNNER_TMP=

cleanup() {
    if [ -n "$RUNNER_TMP" ] && [ -d "$RUNNER_TMP" ]; then
        rm -rf -- "$RUNNER_TMP"
    fi
}
trap cleanup EXIT

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

case "$FORMAT" in
    json|sarif) ;;
    *)
        echo "Unknown format: $FORMAT (expected json or sarif)" >&2
        usage >&2
        exit 2
        ;;
esac

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
        # The CLI uses the host Docker socket to launch tool containers. Its
        # generated analyzer config must therefore exist at the same absolute
        # path on the host and in the CLI container; a private container /tmp
        # makes Docker turn the missing bind source into a directory.
        RUNNER_TMP="$(mktemp -d "${audit_dir}/runner.XXXXXX")"
        chmod 0755 "$RUNNER_TMP"
        cli_args=(analyze --directory "$SUBDIR" --format "$FORMAT")
        [ -n "$TOOL" ] && cli_args+=(--tool "$TOOL")
        if ! docker run --rm \
                --env CODACY_CODE="$SUBDIR" \
                --env "JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=$RUNNER_TMP" \
                --volume /var/run/docker.sock:/var/run/docker.sock \
                --volume "$SUBDIR":"$SUBDIR" \
                --volume "$RUNNER_TMP":"$RUNNER_TMP" \
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

# JSON and SARIF must never contain prepended tool-runner logs or a well-formed
# error object. A non-zero CLI status can mean findings, so validate the selected
# formatter's top-level contract instead of trusting the process status alone.
if [ "$FORMAT" = "json" ]; then
    # $fields and $object are jq variables, not shell expansions.
    # shellcheck disable=SC2016
    report_shape='
        def object_has($fields):
            if type != "object" then false
            else . as $object | all($fields[]; . as $field | $object | has($field))
            end;
        def integer: type == "number" and floor == .;
        def nullable_integer: . == null or integer;
        def valid_location:
            if type != "object" then false
            elif keys == ["LineLocation"] then
                .LineLocation
                | object_has(["line"])
                  and (.line | integer)
            elif keys == ["FullLocation"] then
                .FullLocation
                | object_has(["line", "column"])
                  and (.line | integer)
                  and (.column | integer)
            else false
            end;
        def valid_result:
            if type != "object" then false
            elif keys == ["Issue"] then
                .Issue
                | object_has(["patternId", "filename", "message", "level", "category", "location", "sourceId"])
                  and (.patternId | type == "object")
                  and (.patternId.value | type == "string")
                  and (.filename | type == "string")
                  and (.message | type == "object")
                  and (.message.text | type == "string")
                  and (.level | type == "string")
                  and (.category | . == null or type == "string")
                  and (.location | valid_location)
                  and (.sourceId | . == null or type == "string")
            elif keys == ["DuplicationClone"] then
                .DuplicationClone
                | object_has(["cloneLines", "nrTokens", "nrLines", "files"])
                  and (.cloneLines | type == "string")
                  and (.nrTokens | integer)
                  and (.nrLines | integer)
                  and (.files | type == "array")
                  and (.files | all(.[];
                      object_has(["filePath", "startLine", "endLine"])
                      and (.filePath | type == "string")
                      and (.startLine | integer)
                      and (.endLine | integer)))
            elif keys == ["FileError"] then
                .FileError
                | object_has(["filename", "message"])
                  and (.filename | type == "string")
                  and (.message | type == "string")
            elif keys == ["FileMetrics"] then
                .FileMetrics
                | object_has(["filename", "complexity", "loc", "cloc", "nrMethods", "nrClasses", "lineComplexities"])
                  and (.filename | type == "string")
                  and (.complexity | nullable_integer)
                  and (.loc | nullable_integer)
                  and (.cloc | nullable_integer)
                  and (.nrMethods | nullable_integer)
                  and (.nrClasses | nullable_integer)
                  and (.lineComplexities | type == "array")
                  and (.lineComplexities | all(.[];
                      object_has(["line", "value"])
                      and (.line | integer)
                      and (.value | integer)))
            else false
            end;
        type == "array" and all(.[]; valid_result)
    '
else
    report_shape='type == "object" and .version == "2.1.0" and (.runs | type == "array")'
fi
if ! jq -e "$report_shape" "$OUTPUT" >/dev/null 2>&1; then
    echo -e "${CA_RED}[ERROR]${CA_NC} invalid ${FORMAT} report at ${OUTPUT}; inspect the first tool-runner error before trusting findings" >&2
    exit 1
fi

# Quick summary if format=json.
if [ "$FORMAT" = "json" ]; then
    IFS=$'\t' read -r n_issues n_duplications n_file_errors n_file_metrics < <(
        jq -r '[
            ([.[] | select(keys == ["Issue"])] | length),
            ([.[] | select(keys == ["DuplicationClone"])] | length),
            ([.[] | select(keys == ["FileError"])] | length),
            ([.[] | select(keys == ["FileMetrics"])] | length)
        ] | @tsv' "$OUTPUT"
    )
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${n_issues} issue(s)," \
        "${n_duplications} duplication clone(s), ${n_file_errors} file error(s)," \
        "and ${n_file_metrics} file-metric record(s) to ${OUTPUT}" >&2
    if [ "$n_file_errors" -ne 0 ]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} analysis is incomplete:" \
            "${n_file_errors} file error(s) are recorded in ${OUTPUT}" >&2
        exit 1
    fi
else
    echo -e "${CA_GREEN}[analyze-local]${CA_NC} wrote ${OUTPUT} (${FORMAT} format)" >&2
fi

# Last line on stdout: the path. Pipe-friendly.
echo "$OUTPUT"
