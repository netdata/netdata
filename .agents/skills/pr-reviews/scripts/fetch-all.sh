#!/usr/bin/env bash
# Fetch ALL comments / reviews / review threads for a PR, with paranoid pagination.
#
# Usage:
#   fetch-all.sh <pr-number>
#
# Outputs (under .local/audits/pr-reviews/pr-<N>/):
#   pr.json                -- gh pr view dump (state, head sha, etc.)
#   issue-comments.json    -- /repos/{slug}/issues/{n}/comments       (top-level PR comments)
#   review-comments.json   -- /repos/{slug}/pulls/{n}/comments        (line-level inline comments)
#   reviews.json           -- /repos/{slug}/pulls/{n}/reviews         (review submissions with body)
#   review-threads.json    -- GraphQL reviewThreads (per-thread isResolved + comments[])
#   summary.txt            -- human-readable triage summary
#
# Pagination paranoia:
#   GitHub paginates everything. Default page size is 30, max 100. The skill's
#   #1 rule is: "do not stop at the pagination boundary -- if you see exactly
#   100/200/300 items the round number is suspect; fetch one more page even
#   when the Link header says no more, just to confirm."
#
#   This script:
#     1. Uses --paginate (gh follows Link rel="next" automatically).
#     2. Re-checks each result count against round-number multiples of 100.
#        If suspicious, explicitly requests page=N+1 and merges.
#     3. Logs the final count for each source so the caller can verify.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number>}"
pr_require_numeric "${PR}"
SLUG="$(pr_require_slug)"
DIR="$(pr_state_dir "${PR}")"

echo -e "${PR_GRAY}[fetch-all] PR ${SLUG}#${PR} -> ${DIR}${PR_NC}" >&2

# --- pr.json (state, head sha, draft, requested reviewers, ...) ------------
gh pr view "${PR}" --repo "${SLUG}" --json \
    number,title,state,isDraft,headRefName,headRefOid,baseRefName,baseRefOid,reviewDecision,reviewRequests,mergeable,mergeStateStatus,statusCheckRollup,labels,createdAt,updatedAt,author \
    > "${DIR}/pr.json"

# --- Helper: fetch a paginated REST endpoint with paranoia -----------------
# Args: <api-path> <output-file> <kind-label>
fetch_paranoid() {
    local path="$1" out="$2" kind="$3"
    # Ask for max page size and let gh follow rel=next.
    local sep
    if [[ "${path}" == *\?* ]]; then sep='&'; else sep='?'; fi
    # `gh api --paginate` writes the per-page JSON arrays back-to-back
    # (e.g. `[a,b,c][d,e]`), which is NOT a single valid JSON array. Pipe
    # through `jq -s 'add'` to slurp the multiple top-level values and
    # concatenate them into one array. (`gh api --paginate --jq '.[]'`
    # would emit JSONL but loses the array shape we need for the rest of
    # the loop.)
    gh api --paginate "${path}${sep}per_page=100" | jq -s 'add // []' > "${out}"

    # Did we end up with a JSON array?
    if ! jq -e 'type=="array"' "${out}" >/dev/null 2>&1; then
        echo -e "${PR_RED}[fetch-all] ${kind}: response is not a JSON array. Auth? Rate limit?${PR_NC}" >&2
        head -c 200 "${out}" >&2; echo >&2
        return 1
    fi

    local n
    n="$(jq 'length' "${out}")"
    echo -e "${PR_GRAY}[fetch-all] ${kind}: ${n} items${PR_NC}" >&2

    # Paranoia check: round multiples of 100 are suspicious. Explicitly
    # request page=N+1 to confirm we've reached the end.
    if (( n > 0 && n % 100 == 0 )); then
        local next_page=$(( n / 100 + 1 ))
        echo -e "${PR_YELLOW}[fetch-all] ${kind}: count is exactly ${n} (multiple of 100). Verifying with explicit page=${next_page}...${PR_NC}" >&2

        local probe
        probe="$(gh api "${path}${sep}per_page=100&page=${next_page}" 2>/dev/null || echo '[]')"
        local extra
        extra="$(jq 'length' <<< "${probe}")"
        if (( extra > 0 )); then
            echo -e "${PR_YELLOW}[fetch-all] ${kind}: page ${next_page} had ${extra} more items! Merging.${PR_NC}" >&2
            # Merge and continue probing further pages until empty.
            jq -s '.[0] + .[1]' "${out}" <(printf '%s' "${probe}") > "${out}.merged"
            mv "${out}.merged" "${out}"
            local p=$(( next_page + 1 ))
            while true; do
                probe="$(gh api "${path}${sep}per_page=100&page=${p}" 2>/dev/null || echo '[]')"
                extra="$(jq 'length' <<< "${probe}")"
                (( extra == 0 )) && break
                echo -e "${PR_YELLOW}[fetch-all] ${kind}: page ${p} had ${extra} more.${PR_NC}" >&2
                jq -s '.[0] + .[1]' "${out}" <(printf '%s' "${probe}") > "${out}.merged"
                mv "${out}.merged" "${out}"
                p=$(( p + 1 ))
            done
            n="$(jq 'length' "${out}")"
            echo -e "${PR_GREEN}[fetch-all] ${kind}: final count after probing: ${n}${PR_NC}" >&2
        else
            echo -e "${PR_GRAY}[fetch-all] ${kind}: page ${next_page} empty -- ${n} confirmed.${PR_NC}" >&2
        fi
    fi
}

# --- Three REST sources ----------------------------------------------------
fetch_paranoid "/repos/${SLUG}/issues/${PR}/comments" "${DIR}/issue-comments.json"  "issue-comments"
fetch_paranoid "/repos/${SLUG}/pulls/${PR}/comments"  "${DIR}/review-comments.json" "review-comments"
fetch_paranoid "/repos/${SLUG}/pulls/${PR}/reviews"   "${DIR}/reviews.json"         "reviews"

# --- GraphQL reviewThreads (resolved state + thread IDs) -------------------
# REST does not expose review-thread IDs or isResolved -- we need GraphQL for
# resolve-thread.sh. Pagination uses cursors here, not pages.
echo -e "${PR_GRAY}[fetch-all] review-threads (GraphQL)${PR_NC}" >&2
owner="${SLUG%%/*}"
name="${SLUG##*/}"

threads_tmp="$(mktemp "${TMPDIR:-/tmp}/pr-threads-XXXXXX.json")"
trap 'rm -f "${threads_tmp}"' EXIT

cursor=""
echo '[]' > "${DIR}/review-threads.json"
while true; do
    cursor_args=()
    if [[ -n "${cursor}" ]]; then
        cursor_args+=(-F "after=${cursor}")
    fi
    # The single-quoted GraphQL string contains $owner/$name/$number as
    # GraphQL placeholders, not shell variables; SC2016 is expected here.
    # shellcheck disable=SC2016
    gh api graphql -F owner="${owner}" -F name="${name}" -F number="${PR}" "${cursor_args[@]}" -f query='
        query($owner:String!, $name:String!, $number:Int!, $after:String) {
            repository(owner:$owner, name:$name) {
                pullRequest(number:$number) {
                    reviewThreads(first:100, after:$after) {
                        pageInfo { hasNextPage endCursor }
                        nodes {
                            id
                            isResolved
                            isOutdated
                            path
                            line
                            comments(first:100) {
                                pageInfo { hasNextPage endCursor }
                                totalCount
                                nodes {
                                    id
                                    databaseId
                                    body
                                    author { login }
                                    createdAt
                                    url
                                }
                            }
                        }
                    }
                }
            }
        }
    ' > "${threads_tmp}"

    page_nodes="$(jq '.data.repository.pullRequest.reviewThreads.nodes' "${threads_tmp}")"
    n_page="$(jq 'length' <<< "${page_nodes}")"
    # Append to running file
    jq -s '.[0] + .[1]' "${DIR}/review-threads.json" <(printf '%s' "${page_nodes}") > "${DIR}/review-threads.json.merged"
    mv "${DIR}/review-threads.json.merged" "${DIR}/review-threads.json"

    # Warn if any thread on this page has more than 100 comments -- the
    # inner connection is fetched first:100 only, so the tail is cut.
    truncated_threads="$(jq -r '
        [.data.repository.pullRequest.reviewThreads.nodes[]
         | select(.comments.pageInfo.hasNextPage)
         | .id] | join(", ")
    ' "${threads_tmp}")"
    if [[ -n "${truncated_threads}" ]]; then
        echo -e "${PR_YELLOW}[fetch-all] review-threads: nested comments truncated (>100) for: ${truncated_threads}${PR_NC}" >&2
    fi

    has_next="$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.hasNextPage' "${threads_tmp}")"
    cursor="$(jq -r '.data.repository.pullRequest.reviewThreads.pageInfo.endCursor' "${threads_tmp}")"
    echo -e "${PR_GRAY}[fetch-all] review-threads: +${n_page} (hasNext=${has_next})${PR_NC}" >&2
    [[ "${has_next}" == "true" ]] || break
done
n_threads="$(jq 'length' "${DIR}/review-threads.json")"
echo -e "${PR_GRAY}[fetch-all] review-threads: ${n_threads} threads total${PR_NC}" >&2

# --- summary.txt -----------------------------------------------------------
{
    echo "PR ${SLUG}#${PR} -- snapshot $(pr_now_utc)"
    state=$(jq -r '.state' "${DIR}/pr.json")
    is_draft=$(jq -r '.isDraft' "${DIR}/pr.json")
    head_oid=$(jq -r '.headRefOid' "${DIR}/pr.json")
    head_ref=$(jq -r '.headRefName' "${DIR}/pr.json")
    base_oid=$(jq -r '.baseRefOid' "${DIR}/pr.json")
    base_ref=$(jq -r '.baseRefName' "${DIR}/pr.json")
    decision=$(jq -r '.reviewDecision // "none"' "${DIR}/pr.json")
    mergeable=$(jq -r '.mergeable' "${DIR}/pr.json")
    merge_state=$(jq -r '.mergeStateStatus' "${DIR}/pr.json")
    [[ "${is_draft}" == "true" ]] && state="${state} (draft)"
    printf 'State:    %s\nHead:     %s on %s\nBase:     %s on %s\nDecision: %s\nMerge:    %s / %s\n' \
        "${state}" "${head_oid:0:10}" "${head_ref}" "${base_oid:0:10}" "${base_ref}" \
        "${decision}" "${mergeable}" "${merge_state}"
    echo
    echo "Reviewers requested:"
    jq -r '(.reviewRequests // [])[] | "  - " + (.login // .name // "?")' "${DIR}/pr.json"
    echo
    echo "Counts:"
    printf '  issue-comments  : %d\n' "$(jq 'length' "${DIR}/issue-comments.json")"
    printf '  review-comments : %d\n' "$(jq 'length' "${DIR}/review-comments.json")"
    printf '  reviews         : %d\n' "$(jq 'length' "${DIR}/reviews.json")"
    n_resolved=$(jq '[.[] | select(.isResolved)] | length' "${DIR}/review-threads.json")
    n_open=$(jq '[.[] | select(.isResolved | not)] | length' "${DIR}/review-threads.json")
    printf '  review-threads  : %d (resolved: %d, open: %d)\n' "${n_threads}" "${n_resolved}" "${n_open}"
    echo
    echo "Authors involved (count of items per author across all sources):"
    {
        jq -r '.[] | .user.login' "${DIR}/issue-comments.json"
        jq -r '.[] | .user.login' "${DIR}/review-comments.json"
        jq -r '.[] | .user.login' "${DIR}/reviews.json"
    } | sort | uniq -c | sort -rn | sed 's/^/  /'
    echo
    echo "Open review threads (need attention):"
    jq -r '.[] | select(.isResolved | not)
        | "  THREAD " + .id
        + "  " + .path + ":" + ((.line // "?") | tostring)
        + "  comments=" + ((.comments.nodes | length) | tostring)
        + "  by=" + (.comments.nodes[0].author.login // "?")' \
        "${DIR}/review-threads.json"
} > "${DIR}/summary.txt"

echo
cat "${DIR}/summary.txt"
