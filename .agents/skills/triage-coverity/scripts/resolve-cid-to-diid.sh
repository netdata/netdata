#!/usr/bin/env bash
# Resolve a CID to its current defectInstanceId via /reports/defects.json.
#
# Why: many Coverity endpoints (defectdetails.json, the source-browser deep
# links) take a defectInstanceId, NOT a CID. The CID is stable across runs;
# the defectInstanceId is per-scan. When you only have a CID (e.g. you're
# acting on a stale list, or processing per-cid output from a diff tool),
# you need to look up the current defectInstanceId.
#
# Usage:
#   resolve-cid-to-diid.sh <cid>
#
# Prints the defectInstanceId on stdout if resolved, or "GONE" if Coverity has
# no current defect instance for this CID (the underlying code was removed or
# the scan no longer reports it).
#
# This call is INDEPENDENT of view state -- it queries the defect by CID
# directly, so it does not interact with the server-side view pagination.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
cov_load_env

CID="${1:?usage: $0 <cid>}"
cov_require_numeric_cid "${CID}"

url="$(curl -sS --max-time 30 \
    -H "accept: application/json, text/plain, */*" \
    -H "referer: ${COVERITY_HOST}/" \
    -H "user-agent: ${COVERITY_USER_AGENT}" \
    -b "${COVERITY_COOKIE}" \
    "${COVERITY_HOST}/reports/defects.json?projectId=${COVERITY_PROJECT_ID}&cid=${CID}" \
    | jq -r '.url // ""')"

if [[ -z "${url}" ]]; then
    echo "GONE"
    exit 0
fi

# The .url field looks like:
#   /reports.htm#v70389/p15826/defectInstanceId=14451237&fileInstanceId=...&mergedDefectId=...
diid="$(printf '%s' "${url}" | sed -n 's/.*defectInstanceId=\([0-9]*\).*/\1/p')"
if [[ -z "${diid}" ]]; then
    echo "GONE"
    exit 0
fi

echo "${diid}"
