#!/usr/bin/env bash
# Fetch SonarCloud findings introduced by a specific pull request.
#
# Usage:
#   fetch-sonar-findings.sh <pr-number>
#
# SonarCloud does NOT post per-finding inline comments on the GitHub PR --
# only a QualityGate summary comment is delivered. The actual issue list
# lives behind /api/issues/search?pullRequest=<N> and /api/hotspots/search.
# This script pulls both, so the PR-reviews loop can address them.
#
# Outputs (under .local/audits/pr-reviews/pr-<N>/):
#   sonar-issues.json    -- all open issues this PR introduced
#   sonar-hotspots.json  -- all open security hotspots this PR introduced
#
# Reads SONAR_TOKEN, SONAR_HOST_URL, SONAR_PROJECT from <repo-root>/.env --
# the same .env entries the sonarqube-audit skill uses. If they're missing,
# this script prints what's needed and exits 1.
#
# Sourcing strategy: this script is part of pr-reviews, but it leans on
# the sonarqube-audit skill's `_lib.sh` helpers (sq_load_env, sq_paginate)
# so we get a single paginator + token-masking implementation, rather
# than duplicating the loop and risking drift.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"

# shellcheck disable=SC1091
source "$(dirname "$0")/../../sonarqube-audit/scripts/_lib.sh"

PR="${1:?usage: $0 <pr-number>}"
pr_require_numeric "${PR}"

# sq_load_env reads <repo-root>/.env; same .env the pr-reviews scripts use.
sq_load_env

DIR="$(pr_state_dir "${PR}")"

echo -e "${PR_GRAY}[fetch-sonar] PR ${PR} -> ${DIR}${PR_NC}" >&2

# Build per-source path. sq_paginate streams one JSON page per line; jq -s
# slurps them and concatenates the array under each result key.
sq_paginate "/api/issues/search?componentKeys=${SONAR_PROJECT}&pullRequest=${PR}&resolved=false" \
    | jq -s '[.[].issues[]]' > "${DIR}/sonar-issues.json"

sq_paginate "/api/hotspots/search?projectKey=${SONAR_PROJECT}&pullRequest=${PR}&status=TO_REVIEW" \
    | jq -s '[.[].hotspots[]]' > "${DIR}/sonar-hotspots.json"

# Summary
n_issues="$(jq 'length' "${DIR}/sonar-issues.json")"
n_hotspots="$(jq 'length' "${DIR}/sonar-hotspots.json")"

echo
echo "SonarCloud findings on PR ${PR}:"
echo "  issues:   ${n_issues}"
echo "  hotspots: ${n_hotspots}"
echo
if (( n_issues > 0 )); then
    echo "Issues by severity / rule:"
    jq -r 'group_by(.severity + " " + .rule) | .[] | "  \(.[0].severity)  \(.[0].rule)  x\(length)"' \
        "${DIR}/sonar-issues.json"
    echo
    echo "Issue details:"
    jq -r '.[] | "  \(.key)  \(.severity)  \(.rule)  \(.component | sub("^[^:]+:"; ""))\(if .line then ":" + (.line | tostring) else "" end)\n    \(.message)"' \
        "${DIR}/sonar-issues.json"
fi
if (( n_hotspots > 0 )); then
    echo
    echo "Hotspots:"
    jq -r '.[] | "  \(.key)  \(.vulnerabilityProbability)  \(.ruleKey)  \(.component | sub("^[^:]+:"; ""))\(if .line then ":" + (.line | tostring) else "" end)\n    \(.message)"' \
        "${DIR}/sonar-hotspots.json"
fi
