#!/usr/bin/env bash

set -Eeuo pipefail

repo_root="$(git rev-parse --show-toplevel)"
script="${repo_root}/.agents/skills/codacy-audit/scripts/analyze-local.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/codacy-analyze-local-test.XXXXXX")"
trap 'rm -rf -- "$tmp"' EXIT

mkdir -p "$tmp/bin"
cat > "$tmp/bin/codacy-analysis-cli" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
cat "$CODACY_TEST_REPORT"
EOF
chmod 0700 "$tmp/bin/codacy-analysis-cli"

case_number=0

run_case() {
    local expectation="$1"
    local name="$2"
    local format="$3"
    local report="$4"
    local message="$5"
    local report_path output_path output status

    case_number=$((case_number + 1))
    report_path="$tmp/report-${case_number}.${format}"
    output_path="$tmp/output-${case_number}.${format}"
    printf '%s\n' "$report" > "$report_path"

    set +e
    output="$({
        PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$report_path" \
            "$script" --runner local --format "$format" --output "$output_path"
    } 2>&1)"
    status=$?
    set -e

    case "$expectation" in
        success)
            if [ "$status" -ne 0 ]; then
                printf >&2 '[FAIL] %s: expected success, got %d\n%s\n' "$name" "$status" "$output"
                return 1
            fi
            ;;
        failure)
            if [ "$status" -eq 0 ]; then
                printf >&2 '[FAIL] %s: expected failure\n%s\n' "$name" "$output"
                return 1
            fi
            ;;
        *)
            printf >&2 '[FAIL] %s: unknown expectation %q\n' "$name" "$expectation"
            return 1
            ;;
    esac
    if [[ "$output" != *"$message"* ]]; then
        printf >&2 '[FAIL] %s: missing diagnostic %q\n%s\n' "$name" "$message" "$output"
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
]' 'wrote 1 issue(s), 1 duplication clone(s), 0 file error(s), and 1 file-metric record(s)'

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
set +e
directory_output="$({
    PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$tmp/report-1.json" \
        "$script" --runner local --format json --output "$output_directory"
} 2>&1)"
directory_status=$?
set -e
if [ "$directory_status" -ne 2 ] || [[ "$directory_output" != *'output path must be a regular file'* ]]; then
    printf >&2 '[FAIL] output directory rejection: status=%d\n%s\n' "$directory_status" "$directory_output"
    exit 1
fi
printf '[PASS] output directory rejection\n'

stale_output="$tmp/stale-output.json"
printf '%s\n' '[]' > "$stale_output"
printf '%s\n' 'set -o noclobber' > "$tmp/noclobber-env"
set +e
stale_result="$({
    BASH_ENV="$tmp/noclobber-env" PATH="$tmp/bin:$PATH" CODACY_TEST_REPORT="$tmp/report-1.json" \
        "$script" --runner local --format json --output "$stale_output"
} 2>&1)"
stale_status=$?
set -e
if [ "$stale_status" -ne 2 ] || [[ "$stale_result" != *'cannot initialize output file'* ]]; then
    printf >&2 '[FAIL] stale-output initialization rejection: status=%d\n%s\n' "$stale_status" "$stale_result"
    exit 1
fi
printf '[PASS] stale-output initialization rejection\n'

set +e
unsupported_output="$({ "$script" --format yaml; } 2>&1)"
unsupported_status=$?
set -e
if [ "$unsupported_status" -ne 2 ] || [[ "$unsupported_output" != *'expected json or sarif'* ]]; then
    printf >&2 '[FAIL] unsupported format: status=%d\n%s\n' "$unsupported_status" "$unsupported_output"
    exit 1
fi
printf '[PASS] unsupported format\n'

printf '[PASS] %d analyze-local contract cases\n' "$((case_number + 3))"
