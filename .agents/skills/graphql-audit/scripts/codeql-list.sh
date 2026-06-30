#!/usr/bin/env bash
# List GitHub Code Scanning alerts (CodeQL with security-extended) for the repo.
#
# Usage:
#   codeql-list.sh                        # all OPEN alerts (default)
#   codeql-list.sh --state=open|fixed|dismissed
#   codeql-list.sh --severity=critical|high|medium|low|warning|note|error
#   codeql-list.sh --tool=CodeQL          # filter by tool name
#   codeql-list.sh --raw                  # emit raw JSON instead of summary
#
# Authentication uses the `gh` CLI's stored credentials.
# This is a READ-ONLY script.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"

state="open"
severity=""
tool=""
raw=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --state=*) state="${1#*=}"; shift ;;
        --severity=*) severity="${1#*=}"; shift ;;
        --tool=*) tool="${1#*=}"; shift ;;
        --raw) raw=1; shift ;;
        -h|--help)
            sed -n '2,12p' "$0"; exit 0 ;;
        *) echo "Unknown arg: $1" >&2; exit 2 ;;
    esac
done

slug="$(gh_require_slug)"

# Build the API path.
path="/repos/${slug}/code-scanning/alerts?state=${state}&per_page=100"
[[ -n "${severity}" ]] && path="${path}&severity=${severity}"
[[ -n "${tool}" ]] && path="${path}&tool_name=${tool}"

# `gh api --paginate` writes the per-page JSON arrays back-to-back
# (e.g. `[a,b,c][d,e]`), which is NOT a single valid JSON array. Pipe
# through `jq -s 'add'` to slurp the multiple top-level values into one
# array. Without this, downstream `jq '.[]'` only sees the first page.
echo -e "${GH_GRAY}> gh api --paginate ${path}${GH_NC}" >&2
data="$(gh_api api --paginate "${path}" | jq -s 'add // []')"

if (( raw )); then
    printf '%s\n' "${data}"
    exit 0
fi

# Compact summary: count by rule.
printf '%s\n' "${data}" \
    | jq -r '.[] | "\(.rule.id)|\(.rule.severity)|\(.most_recent_instance.location.path):\(.most_recent_instance.location.start_line)"' \
    | awk -F'|' '{c[$1"|"$2]++} END{for (k in c) print c[k], k}' \
    | sort -rn \
    | head -40 \
    | column -t -s'|'
