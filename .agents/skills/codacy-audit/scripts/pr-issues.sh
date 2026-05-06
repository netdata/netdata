#!/usr/bin/env bash
# pr-issues.sh -- fetch Codacy issues for a PR and emit a clustered summary.
#
# Output:
#   - Full JSON dump of issues:
#       <repo>/.local/audits/codacy/pr-<NNN>-<timestamp>.json
#   - TSV summary clustering by tool / pattern / severity / file
#     printed to stdout.

set -euo pipefail

usage() {
    cat <<'EOF'
pr-issues.sh <pr-number> [options]

Fetches the Codacy v3 PR issues list for the given PR, paginating
through every result, and writes a JSON dump under
<repo>/.local/audits/codacy/. Emits a TSV summary on stdout
clustering issues by tool / pattern / severity / file.

Options:
  --output PATH      explicit output path for the JSON dump
  --by <dim>         summary dimension: tool|pattern|severity|file|category
                     (default: pattern)
  --top N            top-N rows in the summary (default: 25)
  -h, --help

Required env: CODACY_TOKEN (see <repo>/.agents/ENV.md)
Defaults: org=netdata, repo=netdata, provider=gh
          (override via CODACY_ORG / CODACY_REPO / CODACY_PROVIDER in .env)
EOF
}

PR=
OUTPUT=
BY=pattern
TOP=25

while [ $# -gt 0 ]; do
    case "$1" in
        --output)  OUTPUT="$2"; shift 2 ;;
        --by)      BY="$2"; shift 2 ;;
        --top)     TOP="$2"; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        -*)        echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
        *)
            if [ -z "$PR" ]; then PR="$1"
            else echo "Unexpected positional arg: $1" >&2; usage >&2; exit 2
            fi
            shift
            ;;
    esac
done

if [ -z "$PR" ]; then
    usage >&2
    exit 2
fi

# shellcheck source=SCRIPTDIR/_lib.sh disable=SC1091
source "$(cd "$(dirname "$0")" && pwd)/_lib.sh"
codacyaudit_load_env

# ---------------------------------------------------------------
# Output path.

if [ -z "$OUTPUT" ]; then
    audit_dir="$(codacyaudit_audit_dir)"
    OUTPUT="${audit_dir}/pr-${PR}-$(date -u +%Y%m%dT%H%M%SZ).json"
fi

echo -e "${CA_GRAY}[pr-issues] fetching issues for PR #${PR} ...${CA_NC}" >&2

# Wrap raw issues array into an envelope with metadata.
issues_array="$(codacyaudit_pr_issues "$PR")"
total="$(printf '%s' "$issues_array" | jq 'length')"

jq -n \
   --arg pr "$PR" \
   --arg fetched_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
   --argjson total "$total" \
   --argjson data "$issues_array" \
   '{
       pr: ($pr | tonumber),
       fetched_at: $fetched_at,
       total: $total,
       data: $data
    }' > "$OUTPUT"

echo -e "${CA_GREEN}[pr-issues]${CA_NC} wrote ${total} issue(s) to ${OUTPUT}" >&2
echo "$OUTPUT"

# ---------------------------------------------------------------
# Summary.

if [ "$total" -eq 0 ]; then
    echo
    echo -e "${CA_GREEN}No issues on PR #${PR}.${CA_NC}"
    exit 0
fi

field_for_dim() {
    local dim="$1"
    case "$dim" in
        tool)     echo '.commitIssue.toolInfo.name' ;;
        pattern)  echo '.commitIssue.patternInfo.id' ;;
        severity) echo '.commitIssue.patternInfo.severityLevel' ;;
        file)     echo '.commitIssue.filePath' ;;
        category) echo '.commitIssue.patternInfo.category' ;;
        *)        echo "Unknown --by '$dim'" >&2; exit 2 ;;
    esac
}

field_path="$(field_for_dim "$BY")"

echo
echo -e "${CA_CYAN}Top ${TOP} by ${BY} (PR #${PR}, ${total} total issue(s)):${CA_NC}"
printf '%s\n' "------------------------------------------------------------"

jq -r --argjson top "$TOP" "
    .data
    | group_by(${field_path} // \"(none)\")
    | map({key: (.[0] | ${field_path} // \"(none)\"), count: length})
    | sort_by(-.count)
    | .[:\$top]
    | .[]
    | [.count, .key] | @tsv
" "$OUTPUT" \
| awk -F'\t' '{ printf "%8d  %s\n", $1, $2 }'

printf '%s\n' "------------------------------------------------------------"
