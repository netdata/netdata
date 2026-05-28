#!/usr/bin/env bash
# Update Coverity Scan triage for one CID (low-level — usually called by finalize-defect.sh).
#
# Usage:
#   update-triage.sh <cid> <classification> <severity> <action> <comment-file>
#
# Coverity attribute IDs (not names):
#   classification (attr 3): 20=Unclassified  21=Pending  22=FalsePositive  23=Intentional  24=Bug
#   severity       (attr 1): 10=Unspecified   11=Major    12=Moderate       13=Minor
#   action         (attr 2): 1=Undecided      2=FixRequired  3=FixSubmitted  4=ModelingRequired  5=Ignore
#   external ref   (attr 4): always sent null here
#
# Comment is read from <comment-file>. MUST be ASCII (Cloudflare blocks em-dashes
# and smart quotes with a 403 Cloudflare challenge — see SKILL.md).

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
cov_load_env

CID="${1:?usage: $0 <cid> <classification> <severity> <action> <comment-file>}"
CLASS_ID="${2:?usage}"
SEV_ID="${3:?usage}"
ACT_ID="${4:?usage}"
COMMENT_FILE="${5:?usage}"

cov_require_numeric_cid "${CID}"
for v in CLASS_ID SEV_ID ACT_ID; do
    if [[ ! "${!v}" =~ ^[1-9][0-9]*$ ]]; then
        echo -e "${COV_RED}[ERROR]${COV_NC} ${v} must be a positive integer, got: '${!v}'" >&2
        exit 1
    fi
done

if [[ ! -f "${COMMENT_FILE}" || ! -r "${COMMENT_FILE}" ]]; then
    echo -e "${COV_RED}Comment file not readable: ${COMMENT_FILE}${COV_NC}" >&2
    exit 1
fi

# ASCII-only check on the comment body. `tr -d '\000-\177'` is portable across
# GNU and BSD/macOS (`grep -P` is GNU-only).
if LC_ALL=C tr -d '\000-\177' < "${COMMENT_FILE}" | grep -q .; then
    echo -e "${COV_RED}[ERROR]${COV_NC} ${COMMENT_FILE} contains non-ASCII bytes. Cloudflare will block. Replace em-dashes (--) and smart quotes." >&2
    exit 1
fi

# cid + project are numeric (validated above); attribute values must be
# strings per the API.
payload="$(jq -n \
    --argjson cid "${CID}" \
    --arg class "${CLASS_ID}" \
    --arg sev "${SEV_ID}" \
    --arg act "${ACT_ID}" \
    --argjson project "${COVERITY_PROJECT_ID}" \
    --rawfile comment "${COMMENT_FILE}" \
    '{
        triageValues: [
            {attributeId: 3, attributeValue: $class},
            {attributeId: 1, attributeValue: $sev},
            {attributeId: 2, attributeValue: $act},
            {attributeId: 4, attributeValue: null}
        ],
        comment: $comment,
        mergedDefectIds: [$cid],
        ownerId: -1,
        projectId: $project,
        triageStoreIds: [],
        type: "apply"
    }')"

echo -e "${COV_GRAY}Posting triage update for CID ${CID} (class=${CLASS_ID} sev=${SEV_ID} act=${ACT_ID})...${COV_NC}" >&2

response_file="$(mktemp "${TMPDIR:-/tmp}/cov-triage-XXXXXX.json")"
trap 'rm -f "${response_file}"' EXIT

http=$(curl -sS -X POST -o "${response_file}" -w '%{http_code}' \
    -H "accept: application/json, text/plain, */*" \
    -H "content-type: application/json" \
    -H "origin: ${COVERITY_HOST}" \
    -H "referer: ${COVERITY_HOST}/" \
    -H "user-agent: ${COVERITY_USER_AGENT}" \
    -H "x-xsrf-token: ${COVERITY_XSRF}" \
    -b "${COVERITY_COOKIE}" \
    --data-raw "${payload}" \
    "${COVERITY_HOST}/sourcebrowser/updatedefecttriage.json")

if [[ "${http}" != "200" ]]; then
    echo -e "${COV_RED}HTTP ${http} -- update failed${COV_NC}" >&2
    head -c 500 "${response_file}" >&2; echo >&2
    if [[ "${http}" == "401" || "${http}" == "403" || "${http}" == "302" ]]; then
        echo -e "${COV_RED}Session likely expired. Recapture cookie from browser and update .env.${COV_NC}" >&2
    fi
    exit 2
fi

echo -e "${COV_GREEN}OK -- triage updated for CID ${CID}.${COV_NC}" >&2
jq -c '{defectStatus, lastTriaged, updatedValuesByCid}' "${response_file}" 2>/dev/null || head -c 300 "${response_file}"
echo
