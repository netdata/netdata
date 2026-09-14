#!/usr/bin/env bash
# Re-trigger cubic-dev-ai to re-review the PR.
#
# Cubic does not react to comments inside threads. It re-reviews when a NEW
# top-level PR comment mentions it. Posting the comment is the trigger.
#
# Usage:
#   trigger-cubic.sh <pr-number> [<extra-message>]
#
# Default body is "@cubic-dev-ai please review again". Override by passing a
# second argument; the @cubic-dev-ai mention is always prepended so the bot
# is guaranteed to see it.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number> [<extra-message>]}"
pr_require_numeric "${PR}"
EXTRA="${2:-please review again}"

SLUG="$(pr_require_slug)"
BODY="@cubic-dev-ai ${EXTRA}"

echo -e "${PR_GRAY}[trigger-cubic] PR ${SLUG}#${PR}: ${BODY}${PR_NC}" >&2

gh api --method POST "/repos/${SLUG}/issues/${PR}/comments" \
    -f body="${BODY}" \
    --jq '"posted comment id=\(.id) url=\(.html_url)"'
