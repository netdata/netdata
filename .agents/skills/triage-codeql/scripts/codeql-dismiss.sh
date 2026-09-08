#!/usr/bin/env bash
# Dismiss one GitHub Code Scanning alert.
#
# Usage:
#   codeql-dismiss.sh <alert_number> <reason> "<comment>"
#
# Reasons (per GitHub API):
#   false positive    -- alert is incorrect
#   won't fix         -- alert is correct but won't be fixed
#   used in tests     -- alert appears only in test code
#
# This script is WRITE-side: it changes the state of an alert. Use codeql-list.sh
# (read-only) to find alert numbers first.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"

NUMBER="${1:?usage: $0 <alert_number> <reason> '<comment>'}"
REASON="${2:?usage}"
COMMENT="${3:?usage}"

if [[ ! "${NUMBER}" =~ ^[1-9][0-9]*$ ]]; then
    echo -e "${GH_RED}[ERROR]${GH_NC} alert_number must be a positive integer, got: '${NUMBER}'" >&2
    exit 1
fi

case "${REASON}" in
    "false positive"|"won't fix"|"used in tests") ;;
    *)
        echo -e "${GH_RED}[ERROR]${GH_NC} Reason must be one of: 'false positive', \"won't fix\", 'used in tests'." >&2
        exit 2
        ;;
esac

slug="$(gh_require_slug)"
echo -e "${GH_GRAY}> Dismissing alert #${NUMBER} on ${slug}: ${REASON}${GH_NC}" >&2
echo -e "${GH_GRAY}  comment: ${COMMENT}${GH_NC}" >&2

gh_api api --method PATCH "/repos/${slug}/code-scanning/alerts/${NUMBER}" \
    -f state=dismissed \
    -f dismissed_reason="${REASON}" \
    -f dismissed_comment="${COMMENT}" \
    --jq '{number, state, dismissed_reason, dismissed_comment, html_url}'
