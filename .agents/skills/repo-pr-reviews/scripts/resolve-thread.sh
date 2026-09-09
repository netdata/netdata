#!/usr/bin/env bash
# Resolve a review thread on a PR (marks the conversation as done in the UI).
#
# Usage:
#   resolve-thread.sh <thread-node-id>
#
# <thread-node-id> is the GraphQL id from review-threads.json -> .[].id.
# It looks like "PRRT_kwDO..." -- not the numeric REST id.
#
# Resolving a thread does NOT require a reply, but the convention is to reply
# first then resolve. See SKILL.md.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

THREAD_ID="${1:?usage: $0 <thread-node-id>}"

# Review-thread node IDs follow the pattern `PRRT_<base64ish>`. Validate
# the prefix + the body charset so a malformed value can't reach the API.
if [[ ! "${THREAD_ID}" =~ ^PRRT_[A-Za-z0-9_-]+$ ]]; then
    echo -e "${PR_RED}[ERROR]${PR_NC} thread-node-id must look like 'PRRT_...' (got: '${THREAD_ID}')" >&2
    exit 1
fi

# Fail fast on non-GitHub remotes. The mutation operates by node ID and
# does not need the slug, but this script is GitHub-only -- making that
# explicit at entry catches misconfiguration earlier than letting the
# mutation hit a non-github API.
pr_require_slug >/dev/null

# The single-quoted GraphQL string has $threadId as a placeholder.
# shellcheck disable=SC2016
gh api graphql -F threadId="${THREAD_ID}" -f query='
    mutation($threadId:ID!) {
        resolveReviewThread(input: {threadId: $threadId}) {
            thread { id isResolved }
        }
    }
' --jq '.data.resolveReviewThread.thread | "resolved=\(.isResolved) id=\(.id)"'
