#!/usr/bin/env bash
# Report current CI status for a PR head commit.
#
# Usage:
#   ci-status.sh <pr-number>          # human-readable summary
#   ci-status.sh <pr-number> --json   # raw JSON
#
# This is read-only. Use it BEFORE every push to find FAILURES that must
# be addressed in the same push. Do NOT wait for in-progress checks
# between iterations -- the next push will trigger a fresh CI run on the
# new code, which is what matters. See SKILL.md Step 4b for the policy.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
pr_require_gh

PR="${1:?usage: $0 <pr-number> [--json]}"
pr_require_numeric "${PR}"
JSON=0
[[ "${2:-}" == "--json" ]] && JSON=1

SLUG="$(pr_require_slug)"

# pr view --json statusCheckRollup gives the per-check breakdown.
data="$(gh pr view "${PR}" --repo "${SLUG}" --json statusCheckRollup,headRefOid,mergeStateStatus)"

if (( JSON )); then
    printf '%s\n' "${data}"
    exit 0
fi

head_sha="$(jq -r '.headRefOid' <<< "${data}" | head -c 10)"
merge_state="$(jq -r '.mergeStateStatus' <<< "${data}")"

echo "Head: ${head_sha}   Merge state: ${merge_state}"
echo

# Group checks by conclusion / status.
jq -r '
    # StatusContext nodes (commit-status API: Codacy and other non-Actions integrations)
    # carry .state, not .conclusion/.status, and gh reports an in-progress CheckRun with
    # conclusion "". Empty strings are truthy in jq, so // alone picks the wrong field.
    def nz: if . == null or . == "" then empty else . end;
    (.statusCheckRollup // [])
    | map(. + {key: ((.conclusion|nz) // (.state|nz) // (.status|nz) // "PENDING")})
    | group_by(.key)
    | map({key: .[0].key, count: length, names: [.[] | .name // .context]})
    | sort_by(.key)
    | .[]
    | "\(.key)\t\(.count)\t\((.names | sort | unique | join(", ")[0:200]))"
' <<< "${data}" | column -t -s $'\t'

echo
total=$(jq '(.statusCheckRollup // []) | length' <<< "${data}")
fail=$(jq '[(.statusCheckRollup // [])[] | select(((.conclusion // "")|test("FAILURE|TIMED_OUT|CANCELLED|ACTION_REQUIRED|STARTUP_FAILURE|STALE")) or ((.state // "")|test("^(FAILURE|ERROR)$")))] | length' <<< "${data}")
running=$(jq '[(.statusCheckRollup // [])[] | select(((.status // "")|test("^(IN_PROGRESS|QUEUED|WAITING|PENDING|REQUESTED)$")) or ((.state // "")|test("^(PENDING|EXPECTED)$")))] | length' <<< "${data}")
echo "Total checks: ${total}   Failing: ${fail}   Running: ${running}"

# Failures are decided first: a PR with failures and running checks must exit 3, because
# the skill tells the caller to ignore exit 2 and push.
if (( fail > 0 )); then
    echo -e "${PR_RED}WARNING: ${fail} check(s) failing. Address these BEFORE pushing.${PR_NC}" >&2
    exit 3
fi
if (( running > 0 )); then
    # Exit 2 is informational, not a "do not push" signal. The next push
    # will start a fresh CI run on top of the new code -- that's what we
    # actually want to verify. The skill's policy is to push anyway.
    echo -e "${PR_GRAY}Note: ${running} check(s) still running; the next push will start a fresh CI run.${PR_NC}" >&2
    exit 2
fi
exit 0
