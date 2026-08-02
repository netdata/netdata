#!/usr/bin/env bash
# shellcheck disable=SC2016 # SARIF fixtures must preserve the literal $schema key.

set -Eeuo pipefail

repo_root="$(git rev-parse --show-toplevel)"
script="${repo_root}/.agents/skills/codacy-audit/scripts/analyze-local.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/codacy-analyze-local-test.XXXXXX")"
trap 'rm -rf -- "$tmp"' EXIT

mkdir -p "$tmp/bin"
cat > "$tmp/bin/codacy-analysis-cli" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
if [ "${1:-}" = "-v" ]; then
    printf 'codacy-analysis-cli is on version %s\n' "${CODACY_TEST_VERSION:-7.10.1}"
    exit 0
fi
[ -z "${CODACY_TEST_STDERR:-}" ] || printf '%s\n' "$CODACY_TEST_STDERR" >&2
cat "$CODACY_TEST_REPORT"
exit "${CODACY_TEST_EXIT:-0}"
EOF
chmod 0700 "$tmp/bin/codacy-analysis-cli"

cat > "$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' "$@" > "$CODACY_TEST_DOCKER_ARGS"
[ -z "${CODACY_TEST_STDERR:-}" ] || printf '%s\n' "$CODACY_TEST_STDERR" >&2
cat "$CODACY_TEST_REPORT"
exit "${CODACY_TEST_EXIT:-0}"
EOF
chmod 0700 "$tmp/bin/docker"

case_number=0

run_case() {
    local expectation="$1"
    local name="$2"
    local format="$3"
    local report="$4"
    local message="$5"
    local runner_exit="${6:-0}"
    local report_path output_path stdout_path stderr_path stdout stderr status runner_diagnostic

    case_number=$((case_number + 1))
    report_path="$tmp/report-${case_number}.${format}"
    output_path="$tmp/output-${case_number}.${format}"
    stdout_path="$tmp/stdout-${case_number}.txt"
    stderr_path="$tmp/stderr-${case_number}.txt"
    runner_diagnostic="synthetic local runner diagnostic ${case_number}"
    printf '%s\n' "$report" > "$report_path"

    set +e
    PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$report_path" CODACY_TEST_STDERR="$runner_diagnostic" \
        CODACY_TEST_EXIT="$runner_exit" \
        "$script" --runner local --format "$format" --output "$output_path" \
        > "$stdout_path" 2> "$stderr_path"
    status=$?
    set -e
    stdout="$(cat "$stdout_path")"
    stderr="$(cat "$stderr_path")"

    case "$expectation" in
        success)
            if [ "$status" -ne 0 ]; then
                printf >&2 '[FAIL] %s: expected success, got %d\nstdout:\n%s\nstderr:\n%s\n' \
                    "$name" "$status" "$stdout" "$stderr"
                return 1
            fi
            if [ "$stdout" != "$output_path" ]; then
                printf >&2 '[FAIL] %s: stdout must contain exactly the dump path\nstdout:\n%s\n' "$name" "$stdout"
                return 1
            fi
            ;;
        failure)
            if [ "$status" -eq 0 ]; then
                printf >&2 '[FAIL] %s: expected failure\nstdout:\n%s\nstderr:\n%s\n' "$name" "$stdout" "$stderr"
                return 1
            fi
            if [ -n "$stdout" ]; then
                printf >&2 '[FAIL] %s: failure wrote to stdout\n%s\n' "$name" "$stdout"
                return 1
            fi
            ;;
        *)
            printf >&2 '[FAIL] %s: unknown expectation %q\n' "$name" "$expectation"
            return 1
            ;;
    esac
    if [[ "$stderr" != *"$message"* ]]; then
        printf >&2 '[FAIL] %s: missing stderr diagnostic %q\n%s\n' "$name" "$message" "$stderr"
        return 1
    fi
    if [[ "$stderr" != *"$runner_diagnostic"* ]]; then
        printf >&2 '[FAIL] %s: runner stderr was not preserved\n%s\n' "$name" "$stderr"
        return 1
    fi
    printf '[PASS] %s\n' "$name"
}

run_case success 'empty JSON result array' json '[]' \
    'wrote 0 issue(s), 0 duplication clone(s), 0 file error(s), and 0 file-metric record(s)'

run_case success 'mixed official JSON results' json '[
  {"Issue":{"patternId":{"value":"rule"},"filename":"file.go","message":{"text":"finding"},
    "level":"Info","category":null,"location":{"FullLocation":{"line":3,"column":7}},"sourceId":null}},
  {"DuplicationClone":{"cloneLines":"x","nrTokens":10,"nrLines":5,
    "files":[{"filePath":"file.go","startLine":3,"endLine":7}]}},
  {"FileMetrics":{"filename":"file.go","complexity":1,"loc":2,"cloc":null,"nrMethods":null,"nrClasses":null,
    "lineComplexities":[{"line":3,"value":1}]}}
]' 'wrote 1 issue(s), 1 duplication clone(s), 0 file error(s), and 1 file-metric record(s)' 102

run_case failure 'file errors make analysis incomplete' json \
    '[{"FileError":{"filename":"file.go","message":"could not parse"}}]' \
    'analysis is incomplete: 1 file error(s)'

run_case failure 'unknown JSON wrapper' json '[{"Unknown":{}}]' 'invalid json report'
run_case failure 'multiple JSON wrappers' json \
    '[{"Issue":{},"FileMetrics":{}}]' 'invalid json report'
run_case failure 'non-object JSON member' json '[1]' 'invalid json report'
run_case failure 'missing required JSON payload field' json \
    '[{"Issue":{"filename":"file.go"}}]' 'invalid json report'
run_case failure 'invalid nested JSON payload field' json \
    '[{"Issue":{"patternId":{"value":"rule"},"filename":"file.go","message":{"text":"finding"},
      "level":"Info","category":null,"location":{"LineLocation":{"line":"three"}},"sourceId":null}}]' \
    'invalid json report'
run_case failure 'missing nullable file-metric field' json \
    '[{"FileMetrics":{"filename":"file.go","lineComplexities":[]}}]' 'invalid json report'
run_case failure 'fractional integer field' json \
    '[{"DuplicationClone":{"cloneLines":"x","nrTokens":1.5,"nrLines":1,"files":[]}}]' 'invalid json report'
run_case failure 'top-level JSON error object' json '{"error":"runner failed"}' 'invalid json report'
run_case failure 'malformed JSON' json '[{"Issue":' 'invalid json report'
run_case failure 'partial analyzer failure with valid empty JSON' json '[]' \
    'analysis runner failed with status 101' 101

run_case success 'empty valid SARIF report' sarif \
    '{"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json","version":"2.1.0","runs":[]}' \
    'wrote '
run_case success 'nested valid SARIF report' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Shellcheck (reported by Codacy)","version":"1.0.0",
       "informationUri":"https://www.codacy.com","rules":[{
         "id":"rule","name":"rule","shortDescription":{"text":"short"},
         "help":{"text":"help","markdown":"help"},"properties":{"category":"CodeStyle"}}]}},
     "results":[{"ruleIndex":0,"ruleId":"rule","message":{"text":"finding"},"level":"warning",
       "locations":[{"physicalLocation":{"artifactLocation":{"index":0,"uri":"file.sh"},
         "region":{"startLine":1,"startColumn":1}}}],
       "partialFingerprints":{"primaryLocationStartColumnFingerprint":"1","primaryLocationLineHash":"abc"}}],
     "invocations":[{"executionSuccessful":true,"workingDirectory":{"uri":"file:///codacy"}}],
     "artifacts":[{"location":{"uri":"file.sh"}}]
   }]}' 'wrote '
run_case success 'mixed-category Codacy SARIF rule indexes' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[
       {"id":"security","name":"security","shortDescription":{"text":"short"},"help":{"text":"help"},
        "properties":{"category":"CodeStyle"}},
       {"id":"style","name":"style","shortDescription":{"text":"short"},"help":{"text":"help"},
        "properties":{"category":"Security"}}]}},
     "results":[{"ruleIndex":0,"ruleId":"style","message":{"text":"finding"},"level":"warning",
       "locations":[{"physicalLocation":{"artifactLocation":{"index":0,"uri":"file.sh"},
         "region":{"startLine":1,"startColumn":1}}}],
       "partialFingerprints":{"primaryLocationStartColumnFingerprint":"1","primaryLocationLineHash":"abc"}}],
     "invocations":[{"executionSuccessful":true,"workingDirectory":{"uri":"file:///codacy"}}],
     "artifacts":[{"location":{"uri":"file.sh"}}]
   }]}' 'wrote '
run_case failure 'wrong SARIF version' sarif \
    '{"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json","version":"2.0.0","runs":[]}' \
    'invalid sarif report'
run_case failure 'non-object SARIF run' sarif \
    '{"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json","version":"2.1.0","runs":[null]}' \
    'invalid sarif report'
run_case failure 'SARIF run missing tool driver' sarif \
    '{"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json","version":"2.1.0","runs":[{"tool":{},"results":[],"invocations":[],"artifacts":[]}]}' \
    'invalid sarif report'
run_case failure 'SARIF result has invalid rule reference' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[]}},
     "results":[{"ruleIndex":0,"ruleId":"missing","message":{"text":"finding"},"level":"warning",
       "locations":[],"partialFingerprints":{"primaryLocationStartColumnFingerprint":"1",
       "primaryLocationLineHash":"abc"}}],"invocations":[],"artifacts":[]
   }]}' 'invalid sarif report'
run_case failure 'SARIF invocation reports failure' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[]}},
     "results":[],"invocations":[{"executionSuccessful":false,
       "workingDirectory":{"uri":"file:///codacy"}}],"artifacts":[]
   }]}' 'invalid sarif report'
run_case failure 'SARIF run omits invocation evidence' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[]}},
     "results":[],"artifacts":[]
   }]}' 'invalid sarif report'
run_case failure 'SARIF run has multiple invocations' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[]}},
     "results":[],"invocations":[
       {"executionSuccessful":true,"workingDirectory":{"uri":"file:///codacy"}},
       {"executionSuccessful":true,"workingDirectory":{"uri":"file:///codacy"}}
     ],"artifacts":[]
   }]}' 'invalid sarif report'
run_case failure 'SARIF result has no physical location' sarif '
  {"$schema":"https://docs.oasis-open.org/sarif/sarif/v2.1.0/cos02/schemas/sarif-schema-2.1.0.json",
   "version":"2.1.0","runs":[{
     "tool":{"driver":{"name":"Tool","version":"1","informationUri":"https://www.codacy.com","rules":[
       {"id":"rule","name":"rule","shortDescription":{"text":"short"},"help":{"text":"help"},
        "properties":{"category":"CodeStyle"}}]}},
     "results":[{"ruleIndex":0,"ruleId":"rule","message":{"text":"finding"},"level":"warning",
       "locations":[],"partialFingerprints":{"primaryLocationStartColumnFingerprint":"1",
       "primaryLocationLineHash":"abc"}}],
     "invocations":[{"executionSuccessful":true,"workingDirectory":{"uri":"file:///codacy"}}],"artifacts":[]
   }]}' 'invalid sarif report'

output_directory="$tmp/existing-output-directory"
mkdir "$output_directory"
directory_stdout_path="$tmp/directory.stdout"
directory_stderr_path="$tmp/directory.stderr"
set +e
PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$tmp/report-1.json" \
    "$script" --runner local --format json --output "$output_directory" \
    > "$directory_stdout_path" 2> "$directory_stderr_path"
directory_status=$?
set -e
directory_stdout="$(cat "$directory_stdout_path")"
directory_stderr="$(cat "$directory_stderr_path")"
if [ "$directory_status" -ne 2 ] || [ -n "$directory_stdout" ] || \
        [[ "$directory_stderr" != *'output path must be a regular file'* ]]; then
    printf >&2 '[FAIL] output directory rejection: status=%d\nstdout:\n%s\nstderr:\n%s\n' \
        "$directory_status" "$directory_stdout" "$directory_stderr"
    exit 1
fi
printf '[PASS] output directory rejection\n'

stale_output="$tmp/stale-output.json"
printf '%s\n' '[]' > "$stale_output"
printf '%s\n' 'set -o noclobber' > "$tmp/noclobber-env"
stale_stdout_path="$tmp/stale.stdout"
stale_stderr_path="$tmp/stale.stderr"
set +e
BASH_ENV="$tmp/noclobber-env" PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$tmp/report-1.json" \
    "$script" --runner local --format json --output "$stale_output" \
    > "$stale_stdout_path" 2> "$stale_stderr_path"
stale_status=$?
set -e
stale_stdout="$(cat "$stale_stdout_path")"
stale_stderr="$(cat "$stale_stderr_path")"
if [ "$stale_status" -ne 2 ] || [ -n "$stale_stdout" ] || \
        [[ "$stale_stderr" != *'cannot initialize output file'* ]]; then
    printf >&2 '[FAIL] stale-output initialization rejection: status=%d\nstdout:\n%s\nstderr:\n%s\n' \
        "$stale_status" "$stale_stdout" "$stale_stderr"
    exit 1
fi
printf '[PASS] stale-output initialization rejection\n'

unsupported_stdout_path="$tmp/unsupported.stdout"
unsupported_stderr_path="$tmp/unsupported.stderr"
set +e
"$script" --format yaml > "$unsupported_stdout_path" 2> "$unsupported_stderr_path"
unsupported_status=$?
set -e
unsupported_stdout="$(cat "$unsupported_stdout_path")"
unsupported_stderr="$(cat "$unsupported_stderr_path")"
if [ "$unsupported_status" -ne 2 ] || [ -n "$unsupported_stdout" ] || \
        [[ "$unsupported_stderr" != *'expected json or sarif'* ]]; then
    printf >&2 '[FAIL] unsupported format: status=%d\nstdout:\n%s\nstderr:\n%s\n' \
        "$unsupported_status" "$unsupported_stdout" "$unsupported_stderr"
    exit 1
fi
printf '[PASS] unsupported format\n'

version_stdout_path="$tmp/version.stdout"
version_stderr_path="$tmp/version.stderr"
set +e
PATH="$tmp/bin:$PATH" CODACY_TEST_VERSION=8.0.0 CODACY_TEST_REPORT="$tmp/report-1.json" \
    "$script" --runner local --format json --output "$tmp/version-output.json" \
    > "$version_stdout_path" 2> "$version_stderr_path"
version_status=$?
set -e
version_stdout="$(cat "$version_stdout_path")"
version_stderr="$(cat "$version_stderr_path")"
if [ "$version_status" -ne 2 ] || [ -n "$version_stdout" ] || \
        [[ "$version_stderr" != *'unsupported local codacy-analysis-cli version; expected 7.10.1'* ]]; then
    printf >&2 '[FAIL] local version rejection: status=%d\nstdout:\n%s\nstderr:\n%s\n' \
        "$version_status" "$version_stdout" "$version_stderr"
    exit 1
fi
if [ -e "$tmp/version-output.json" ]; then
    printf >&2 '[FAIL] local version rejection initialized the output path\n'
    exit 1
fi
printf '[PASS] local version rejection\n'

docker_report="$tmp/docker-report.json"
docker_output="$tmp/docker-output.json"
docker_stdout_path="$tmp/docker.stdout"
docker_stderr_path="$tmp/docker.stderr"
docker_args_path="$tmp/docker.args"
docker_diagnostic='synthetic Docker runner diagnostic'
docker_image='codacy/codacy-analysis-cli:7.10.1@sha256:d412b2a84e72d0b541e29dd6cdffa78a73afcf35d8aa546988cd2a44edaab15c'
printf '%s\n' '[]' > "$docker_report"
set +e
PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$docker_report" CODACY_TEST_STDERR="$docker_diagnostic" \
    CODACY_TEST_DOCKER_ARGS="$docker_args_path" CODACY_TEST_EXIT=0 \
    "$script" --runner docker --format json --output "$docker_output" \
    > "$docker_stdout_path" 2> "$docker_stderr_path"
docker_status=$?
set -e
docker_stdout="$(cat "$docker_stdout_path")"
docker_stderr="$(cat "$docker_stderr_path")"
mapfile -t docker_args < "$docker_args_path"
if [ "${#docker_args[@]}" -lt 6 ]; then
    printf >&2 '[FAIL] Docker runner emitted only %d arguments\n' "${#docker_args[@]}"
    exit 1
fi
runner_tmp="${docker_args[5]#JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=}"
case "$runner_tmp" in
    "$repo_root"/.local/audits/codacy/runner.*) ;;
    *)
        printf >&2 '[FAIL] Docker runner temp path is outside the audit directory: %q\n' "$runner_tmp"
        exit 1
        ;;
esac
expected_docker_args_path="$tmp/docker-expected.args"
printf '%s\n' \
    run \
    --rm \
    --env \
    "CODACY_CODE=$repo_root" \
    --env \
    "JAVA_TOOL_OPTIONS=-Djava.io.tmpdir=$runner_tmp" \
    --volume \
    /var/run/docker.sock:/var/run/docker.sock \
    --volume \
    "$repo_root:$repo_root" \
    --volume \
    "$runner_tmp:$runner_tmp" \
    "$docker_image" \
    analyze \
    --directory \
    "$repo_root" \
    --format \
    json \
    --fail-if-incomplete > "$expected_docker_args_path"
if [ "$docker_status" -ne 0 ] || [ "$docker_stdout" != "$docker_output" ] || \
        [[ "$docker_stderr" != *"$docker_diagnostic"* ]] || \
        [[ "$docker_stderr" != *'wrote 0 issue(s)'* ]] || \
        ! cmp -s -- "$expected_docker_args_path" "$docker_args_path"; then
    printf >&2 '[FAIL] Docker runner contract: status=%d\nstdout:\n%s\nstderr:\n%s\nargs:\n%s\n' \
        "$docker_status" "$docker_stdout" "$docker_stderr" "$(cat "$docker_args_path")"
    printf >&2 'expected args:\n%s\n' "$(cat "$expected_docker_args_path")"
    exit 1
fi
printf '[PASS] Docker digest and stderr contract\n'

printf '[PASS] %d analyze-local contract cases\n' "$((case_number + 5))"
