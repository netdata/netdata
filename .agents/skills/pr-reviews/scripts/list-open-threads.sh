#!/usr/bin/env bash
# List open (unresolved) review threads on a PR with the comments inside each.
# Usage:
#   list-open-threads.sh <pr-number>            # full bodies
#   list-open-threads.sh <pr-number> --short    # one-line per thread
#
# Reads from the cached fetch-all.sh dump. Run fetch-all.sh first.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"

PR="${1:?usage: $0 <pr-number> [--short]}"
pr_require_numeric "${PR}"
SHORT=0
[[ "${2:-}" == "--short" ]] && SHORT=1

DIR="$(pr_state_dir "${PR}")"
FILE="${DIR}/review-threads.json"
if [[ ! -f "${FILE}" || ! -r "${FILE}" || ! -s "${FILE}" ]]; then
    echo -e "${PR_RED}Missing ${FILE}. Run fetch-all.sh ${PR} first.${PR_NC}" >&2
    exit 1
fi

if (( SHORT )); then
    jq -r '
        .[]
        | select(.isResolved | not)
        | "\(.id) | \(.path):\(.line // "?") | \(.comments.nodes[0].author.login)"
    ' "${FILE}" | column -t -s '|'
else
    jq -r '
        .[]
        | select(.isResolved | not)
        | "================================================================\nTHREAD: \(.id)\nFile:   \(.path):\(.line // "?")\nOutdated: \(.isOutdated)\n----------------------------------------------------------------\n" + (
            [.comments.nodes[] | "[\(.author.login) at \(.createdAt)]\n\(.body)\n  -> \(.url)\n"] | join("\n")
        )
    ' "${FILE}"
fi
