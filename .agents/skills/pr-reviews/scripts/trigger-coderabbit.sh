#!/usr/bin/env bash
# Re-trigger coderabbitai to re-review the PR.
#
# CodeRabbit does not re-review on a pushed commit or a thread reply alone. It
# acts on a NEW top-level PR comment addressed to it, and the command word
# matters: "@coderabbitai review" starts an incremental review of the commits
# since its last pass. Posting the comment is the trigger.
#
# Usage:
#   trigger-coderabbit.sh <pr-number> [<command>]
#
# Default command is "review" (incremental). Pass "full review" to force a
# re-review of the whole PR rather than only the new commits. The
# @coderabbitai mention is always prepended so the bot is guaranteed to see it.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number> [<command>]}"
pr_require_numeric "${PR}"
shift

# Join the remaining words so an unquoted `full review` is not silently
# truncated to `full`, which posts a command coderabbit ignores while gh api
# still reports success.
CMD="${*:-review}"

case "${CMD}" in
    review | "full review") ;;
    *)
        echo "[trigger-coderabbit] unknown command '${CMD}' (expected 'review' or 'full review')" >&2
        exit 1
        ;;
esac

SLUG="$(pr_require_slug)"
# The handle is @coderabbitai. @coderabbit is a different, unrelated GitHub
# user -- mentioning it pings a stranger and never reaches the bot.
BODY="@coderabbitai ${CMD}"

echo -e "${PR_GRAY}[trigger-coderabbit] PR ${SLUG}#${PR}: ${BODY}${PR_NC}" >&2

gh api --method POST "/repos/${SLUG}/issues/${PR}/comments" \
    -f body="${BODY}" \
    --jq '"posted comment id=\(.id) url=\(.html_url)"'
