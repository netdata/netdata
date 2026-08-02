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

run_case success 'valid SARIF report' sarif \
    '{"version":"2.1.0","runs":[]}' 'wrote '
run_case failure 'wrong SARIF version' sarif \
    '{"version":"2.0.0","runs":[]}' 'invalid sarif report'

set +e
unsupported_output="$({ "$script" --format yaml; } 2>&1)"
unsupported_status=$?
set -e
if [ "$unsupported_status" -ne 2 ] || [[ "$unsupported_output" != *'expected json or sarif'* ]]; then
    printf >&2 '[FAIL] unsupported format: status=%d\n%s\n' "$unsupported_status" "$unsupported_output"
    exit 1
fi
printf '[PASS] unsupported format\n'

printf '[PASS] %d analyze-local contract cases\n' "$((case_number + 1))"
