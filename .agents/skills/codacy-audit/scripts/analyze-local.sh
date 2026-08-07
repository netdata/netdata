#!/usr/bin/env bash
# analyze-local.sh -- run codacy-analysis-cli locally on the working tree.
#
# Mirrors what Codacy CI would run on the same source. Useful BEFORE
# `git push` to catch findings in seconds, not minutes.
#
# Output: a JSON or SARIF report under <repo>/.local/audits/codacy/.
# stdout (last line): the report path.

set -euo pipefail

usage() {
    cat <<'EOF'
analyze-local.sh [options]

Runs the official codacy-analysis-cli (https://github.com/codacy/codacy-analysis-cli)
on the current working tree and writes a JSON or SARIF report under
<repo>/.local/audits/codacy/. The cli respects the repo's .codacy.yml
exclude_paths.

Options:
  --tool <name>          run a single tool (e.g. shellcheck, markdownlint).
                         Omit to run all configured tools applicable to the target.
  --directory <path>     analyze a subpath (default: <repo-root>)
  --format json|sarif    output format (default: json)
  --output PATH          explicit dump path (default: auto under .local/audits/codacy/)
  --runner docker|local  runner to use (default: auto -- prefer local
                         binary, fall back to docker)
  -h, --help

Required tools: jq, plus the supported local codacy-analysis-cli binary OR docker.
EOF
}

TOOL=
SUBDIR=
FORMAT=json
OUTPUT=
RUNNER=auto
RUNNER_TMP=
CODACY_ANALYSIS_CLI_VERSION=7.10.1
CODACY_ANALYSIS_CLI_DIGEST=sha256:d412b2a84e72d0b541e29dd6cdffa78a73afcf35d8aa546988cd2a44edaab15c
CODACY_ANALYSIS_CLI_IMAGE="codacy/codacy-analysis-cli:${CODACY_ANALYSIS_CLI_VERSION}@${CODACY_ANALYSIS_CLI_DIGEST}"

cleanup() {
    if [ -n "$RUNNER_TMP" ] && [ -d "$RUNNER_TMP" ]; then
        rm -rf -- "$RUNNER_TMP"
    fi
}
trap cleanup EXIT

missing_option_value() {
    local option="$1"

    echo "Missing value for option: $option" >&2
    usage >&2
    exit 2
}

reserve_auto_output() {
    local prefix="$1" candidate attempt

    for ((attempt = 0; attempt < 100; attempt++)); do
        candidate="${prefix}-$$-${RANDOM}.${FORMAT}"
        if (set -o noclobber; : > "$candidate") 2>/dev/null; then
            OUTPUT="$candidate"
            return 0
        fi
    done

    echo "[ERROR] cannot reserve a unique output file under ${audit_dir}" >&2
    exit 2
}

while [ $# -gt 0 ]; do
    case "$1" in
        --tool|--directory|--format|--output|--runner)
            option="$1"
            if [ "$#" -lt 2 ] || [ -z "$2" ]; then
                missing_option_value "$option"
            fi
            case "$2" in
                --tool|--directory|--format|--output|--runner|-h|--help)
                    missing_option_value "$option"
                    ;;
                *) : ;;
            esac
            case "$option" in
                --tool)      TOOL="$2" ;;
                --directory) SUBDIR="$2" ;;
                --format)    FORMAT="$2" ;;
                --output)    OUTPUT="$2" ;;
                --runner)    RUNNER="$2" ;;
                *)
                    echo "Internal parser error: unhandled option: $option" >&2
                    exit 2
                    ;;
            esac
            shift 2
            ;;
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

if ! command -v jq >/dev/null 2>&1; then
    echo "[ERROR] required tool 'jq' not found in PATH; install jq before running local Codacy analysis" >&2
    exit 2
fi

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
        *)  SUBDIR="$(cd -- "$SUBDIR" && pwd)" ;;
    esac
fi

# Resolve output path.
output_reserved=false
auto_output_prefix=
if [ -z "$OUTPUT" ]; then
    suffix=""
    [ -n "$TOOL" ] && suffix="-${TOOL}"
    auto_output_prefix="${audit_dir}/local${suffix}-$(date -u +%Y%m%dT%H%M%SZ)"
else
    if [ -e "$OUTPUT" ] && [ ! -f "$OUTPUT" ]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} output path must be a regular file: ${OUTPUT}" >&2
        exit 2
    fi
    output_parent="$(dirname -- "$OUTPUT")"
    if [ ! -d "$output_parent" ]; then
        echo -e "${CA_RED}[ERROR]${CA_NC} output parent directory does not exist: ${output_parent}" >&2
        exit 2
    fi
fi

# Pick a runner.
auto_selected=false
if [ "$RUNNER" = "auto" ]; then
    auto_selected=true
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

case "$RUNNER" in
    local|docker) ;;
    *)
        echo -e "${CA_RED}[ERROR]${CA_NC} unknown --runner '${RUNNER}'" >&2
        exit 2
        ;;
esac

# The JSON/SARIF validators below intentionally model one stable CLI release.
# Keep the local runner on that release; the Docker path is pinned to the same
# version so a moving image cannot silently change the report contract.
if [ "$RUNNER" = "local" ]; then
    expected_version="codacy-analysis-cli is on version ${CODACY_ANALYSIS_CLI_VERSION}"
    local_version_error=
    if ! local_version_output="$(codacy-analysis-cli -v 2>&1)"; then
        local_version_error="cannot determine local codacy-analysis-cli version"
    elif ! grep -Fqx "$expected_version" <<< "$local_version_output"; then
        local_version_error="unsupported local codacy-analysis-cli version; expected ${CODACY_ANALYSIS_CLI_VERSION}"
    fi
    if [ -n "$local_version_error" ]; then
        if [ "$auto_selected" = true ] && command -v docker >/dev/null 2>&1; then
            echo -e "${CA_YELLOW}[analyze-local] ${local_version_error}; falling back to the digest-pinned Docker runner${CA_NC}" >&2
            RUNNER=docker
        else
            echo -e "${CA_RED}[ERROR]${CA_NC} ${local_version_error}" >&2
            exit 2
        fi
    fi
fi

if [ -n "$auto_output_prefix" ]; then
    reserve_auto_output "$auto_output_prefix"
    output_reserved=true
fi

# Explicit destinations are truncated before the runner starts. If shell
# redirection cannot initialize one, a readable stale report must not survive
# to the shape check. Auto destinations are reserved only after runner preflight.
if [ "$output_reserved" != true ]; then
    if ! : > "$OUTPUT"; then
        echo -e "${CA_RED}[ERROR]${CA_NC} cannot initialize output file: ${OUTPUT}" >&2
        exit 2
    fi
fi
if [ ! -f "$OUTPUT" ]; then
    echo -e "${CA_RED}[ERROR]${CA_NC} output path must be a regular file: ${OUTPUT}" >&2
    exit 2
fi

echo -e "${CA_GRAY}[analyze-local] runner=${RUNNER} format=${FORMAT} dir=${SUBDIR}${CA_NC}" >&2

runner_status=0
case "$RUNNER" in
    local)
        # Local binary expects host paths.
        local_args=(analyze --directory "$SUBDIR" --format "$FORMAT" --fail-if-incomplete)
        [ -n "$TOOL" ] && local_args+=(--tool "$TOOL")
        codacy-analysis-cli "${local_args[@]}" >| "$OUTPUT" || runner_status=$?
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
        cli_args=(analyze --directory "$SUBDIR" --format "$FORMAT" --fail-if-incomplete)
        [ -n "$TOOL" ] && cli_args+=(--tool "$TOOL")
        docker run --rm \
                --env CODACY_CODE="$SUBDIR" \
                --env "JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=\"$RUNNER_TMP\"" \
                --volume /var/run/docker.sock:/var/run/docker.sock \
                --volume "$SUBDIR":"$SUBDIR" \
                --volume "$RUNNER_TMP":"$RUNNER_TMP" \
                "$CODACY_ANALYSIS_CLI_IMAGE" \
                "${cli_args[@]}" >| "$OUTPUT" || runner_status=$?
        ;;
    *)
        echo -e "${CA_RED}[ERROR]${CA_NC} unknown --runner '${RUNNER}'" >&2
        exit 2
        ;;
esac

# With the default zero issue threshold, 102 means a complete analysis found
# issues. --fail-if-incomplete makes a tool failure return 101 instead, which
# must never be accepted even when the remaining tools emitted valid JSON.
case "$runner_status" in
    0|102) ;;
    *)
        echo -e "${CA_RED}[ERROR]${CA_NC} analysis runner failed with status ${runner_status}; inspect the tool-runner diagnostics above" >&2
        exit 1
        ;;
esac

# Sanity check the output. Repeat the regular-file check after the runner so a
# non-regular runner result is not handed to jq.
if [ ! -f "$OUTPUT" ] || [ ! -r "$OUTPUT" ]; then
    echo -e "${CA_RED}[ERROR]${CA_NC} runner output is not a readable regular file: ${OUTPUT}" >&2
    exit 1
fi
if [ ! -s "$OUTPUT" ]; then
    echo -e "${CA_RED}[ERROR]${CA_NC} empty output at ${OUTPUT}; check the runner above" >&2
    exit 1
fi

# JSON and SARIF must never contain prepended tool-runner logs or a well-formed
# error object. Accepted status 102 means findings, so validate the selected
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
    # codacy-analysis-cli 7.10.1 serializes this closed SARIF model. Validate
    # nested records and their rule/artifact references so truncated runner
    # output cannot be mistaken for a clean analysis.
    # shellcheck disable=SC2016
    report_shape='
        def object_has($fields):
            if type != "object" then false
            else . as $object | all($fields[]; . as $field | $object | has($field))
            end;
        def integer: type == "number" and floor == .;
        def nonnegative_integer: integer and . >= 0;
        def positive_integer: integer and . >= 1;
        def valid_message:
            object_has(["text"])
            and (.text | type == "string")
            and (.markdown? | . == null or type == "string");
        def valid_rule:
            object_has(["id", "name", "shortDescription", "help", "properties"])
            and (.id | type == "string")
            and (.name | type == "string")
            and (.shortDescription | valid_message)
            and (.help | valid_message)
            and (.properties
                 | object_has(["category"])
                   and (.category | type == "string"));
        def valid_artifact:
            object_has(["location"])
            and (.location
                 | object_has(["uri"])
                   and (.uri | type == "string"));
        def valid_invocation:
            object_has(["executionSuccessful", "workingDirectory"])
            and .executionSuccessful == true
            and (.workingDirectory
                 | object_has(["uri"])
                   and (.uri | type == "string"));
        def valid_location($artifacts):
            object_has(["physicalLocation"])
            and (.physicalLocation
                 | object_has(["artifactLocation", "region"])
                   and (.artifactLocation
                        | object_has(["index", "uri"])
                          and (.index | nonnegative_integer)
                          and (.index < ($artifacts | length))
                          and (.uri | type == "string")
                          and (.uri == $artifacts[.index].location.uri))
                   and (.region
                        | object_has(["startLine", "startColumn"])
                          and (.startLine | positive_integer)
                          and (.startColumn | positive_integer)));
        # Codacy 7.10.1 publishes security rules before non-security rules, but
        # indexes non-security results within the hidden subset. The serialized
        # category is not the partition source of truth, so the unique ruleId
        # is authoritative while ruleIndex remains range-checked.
        def valid_rule_reference($rules):
            .ruleId as $id
            | any($rules[]; .id == $id);
        def valid_result($rules; $artifacts):
            object_has(["ruleIndex", "ruleId", "message", "level", "locations", "partialFingerprints"])
            and (.ruleIndex | nonnegative_integer)
            and (.ruleIndex < ($rules | length))
            and (.ruleId | type == "string")
            and valid_rule_reference($rules)
            and (.message | valid_message)
            and (.level | type == "string" and IN("error", "warning", "note", "none"))
            and (.locations | type == "array")
            and (.locations | length == 1)
            and (.locations | all(.[]; valid_location($artifacts)))
            and (.partialFingerprints
                 | object_has(["primaryLocationStartColumnFingerprint", "primaryLocationLineHash"])
                   and (.primaryLocationStartColumnFingerprint | type == "string")
                   and (.primaryLocationLineHash | type == "string"));
        def valid_run:
            object_has(["tool", "results", "invocations", "artifacts"])
            and (.tool
                 | object_has(["driver"])
                   and (.driver
                        | object_has(["name", "version", "informationUri", "rules"])
                          and (.name | type == "string")
                          and (.version | type == "string")
                          and (.informationUri | type == "string")
                          and (.rules | type == "array")
                          and (.rules | all(.[]; valid_rule))))
            and (.tool.driver.rules
                 | [.[].id] as $ids
                   | ($ids | length) == ($ids | unique | length))
            and (.results | type == "array")
            and (.invocations | type == "array")
            and (.invocations | length == 1)
            and (.invocations | all(.[]; valid_invocation))
            and (.artifacts | type == "array")
            and (.artifacts | all(.[]; valid_artifact))
            and (. as $run
                 | all($run.results[];
                       valid_result($run.tool.driver.rules; $run.artifacts)));
        type == "object"
        and .["$schema"] == "https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json"
        and .version == "2.1.0"
        and (.runs | type == "array")
        and (.runs | all(.[]; valid_run))
    '
fi
if ! jq -e "$report_shape" -- "$OUTPUT" >/dev/null 2>&1; then
    echo -e "${CA_RED}[ERROR]${CA_NC} invalid ${FORMAT} report at ${OUTPUT}; inspect the tool-runner diagnostics above before trusting findings" >&2
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
        ] | @tsv' -- "$OUTPUT"
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
printf '%s\n' "$OUTPUT"
