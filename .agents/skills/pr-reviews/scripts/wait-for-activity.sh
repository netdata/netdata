#!/usr/bin/env bash
# Block until new activity appears on a PR (new comment, new review, new
# commit), or until the timeout fires.
#
# Usage:
#   wait-for-activity.sh <pr-number> [<timeout-seconds>] [<poll-interval-seconds>]
#
# Defaults: timeout=1800 (30 min), poll=30s.
#
# Establishes a baseline by reading the cached fetch-all.sh dump (or fetching
# fresh if missing). Then polls every <poll-interval> seconds for changes.
# Exits 0 when something new is found; exits 124 on timeout.
#
# "New" means any of:
#   - issue-comments count changed
#   - review-comments count changed
#   - reviews count changed
#   - PR head sha changed (new push)
#   - reviewThreads isResolved transitions
#
# Both cubic-dev-ai and copilot post a comment when they have nothing new
# to add -- so this loop ends naturally on a clean re-review.
#
# Note about Costa's rule: "the PR should never be left with unaddressed
# comments". After this returns, run fetch-all.sh again, classify the new
# activity, and address it.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number> [<timeout-seconds>] [<poll-interval-seconds>]}"
pr_require_numeric "${PR}"
TIMEOUT="${2:-1800}"
POLL="${3:-30}"

pr_require_numeric "${TIMEOUT}" "timeout-seconds"
pr_require_numeric "${POLL}"    "poll-interval-seconds"

SLUG="$(pr_require_slug)"

snapshot() {
    # Quick snapshot for change detection -- not a full fetch.
    #
    # `gh api --paginate ... --jq '.field'` emits one value per page (JSONL),
    # so we sum them with awk.  `gh api --paginate ... | jq 'length'` would
    # NOT work here -- the concatenated arrays from paginate are not a
    # single JSON value.
    gh pr view "${PR}" --repo "${SLUG}" --json headRefOid \
        --jq '"head=\(.headRefOid)"'
    gh api --paginate "/repos/${SLUG}/issues/${PR}/comments?per_page=100" --jq 'if type=="array" then length else 0 end' 2>/dev/null \
        | awk 'BEGIN{t=0} {t+=$1} END{print "n_issue="t}'
    gh api --paginate "/repos/${SLUG}/pulls/${PR}/comments?per_page=100" --jq 'if type=="array" then length else 0 end' 2>/dev/null \
        | awk 'BEGIN{t=0} {t+=$1} END{print "n_review_comment="t}'
    gh api --paginate "/repos/${SLUG}/pulls/${PR}/reviews?per_page=100" --jq 'if type=="array" then length else 0 end' 2>/dev/null \
        | awk 'BEGIN{t=0} {t+=$1} END{print "n_review="t}'
    # Track resolve/unresolve transitions via the GraphQL reviewThreads
    # connection -- counts of resolved/open threads. A thread getting
    # resolved/unresolved is real PR activity that the REST counts above
    # would miss.
    #
    # IMPORTANT: this line MUST always be present in the snapshot, even on
    # transient GraphQL failure. If it disappears intermittently, the
    # snapshot diff would falsely show "new activity" each time it returns.
    # We collect the GraphQL output into a variable, fall back to a fixed
    # literal on any failure, and emit one canonical line.
    local owner name graphql_out
    owner="${SLUG%%/*}"
    name="${SLUG##*/}"
    # Cursor-paginate threads so PRs with >100 threads are tracked correctly.
    local cursor='' resolved=0 open=0
    while :; do
        local cursor_args=()
        [[ -n "${cursor}" ]] && cursor_args+=(-F "after=${cursor}")
        # GraphQL string has $owner/$name/$number/$after as placeholders.
        # shellcheck disable=SC2016
        graphql_out="$(gh api graphql -F owner="${owner}" -F name="${name}" -F number="${PR}" \
            "${cursor_args[@]}" -f query='
            query($owner:String!, $name:String!, $number:Int!, $after:String) {
                repository(owner:$owner, name:$name) {
                    pullRequest(number:$number) {
                        reviewThreads(first:100, after:$after) {
                            pageInfo { hasNextPage endCursor }
                            nodes { isResolved }
                        }
                    }
                }
            }' 2>/dev/null)" || { echo "threads_resolved=ERR_open=ERR"; return 0; }
        local r o
        r=$(jq -r '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved)] | length' <<< "${graphql_out}" 2>/dev/null) || r=0
        o=$(jq -r '[.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved | not)] | length' <<< "${graphql_out}" 2>/dev/null) || o=0
        resolved=$(( resolved + r ))
        open=$(( open + o ))
        local hasnext nextcur
        hasnext=$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage // false' <<< "${graphql_out}" 2>/dev/null)
        nextcur=$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor // ""' <<< "${graphql_out}" 2>/dev/null)
        [[ "${hasnext}" == "true" ]] || break
        cursor="${nextcur}"
    done
    echo "threads_resolved=${resolved}_open=${open}"
}

echo -e "${PR_GRAY}[wait] PR ${SLUG}#${PR}  timeout=${TIMEOUT}s  poll=${POLL}s${PR_NC}" >&2

baseline="$(snapshot)"
echo -e "${PR_GRAY}[wait] baseline:${PR_NC}" >&2
echo "${baseline}" | sed 's/^/    /' >&2

start=$(date +%s)
while true; do
    sleep "${POLL}"
    now=$(date +%s)
    elapsed=$(( now - start ))
    if (( elapsed >= TIMEOUT )); then
        echo -e "${PR_YELLOW}[wait] timeout after ${elapsed}s -- no new activity${PR_NC}" >&2
        exit 124
    fi
    current="$(snapshot)"
    if [[ "${current}" != "${baseline}" ]]; then
        echo -e "${PR_GREEN}[wait] new activity detected after ${elapsed}s:${PR_NC}" >&2
        # diff exits 1 when inputs differ (that's the success case here);
        # set -euo pipefail would trip on it. Force exit 0 from the
        # pipeline so the script proper can return 0 cleanly.
        { diff <(printf '%s' "${baseline}") <(printf '%s' "${current}") || true; } \
            | sed 's/^/    /' >&2
        exit 0
    fi
    echo -e "${PR_GRAY}[wait] ${elapsed}s elapsed, no change yet...${PR_NC}" >&2
done
