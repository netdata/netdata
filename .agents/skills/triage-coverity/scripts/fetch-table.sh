#!/usr/bin/env bash
# Fetch all pages of a Coverity Scan view table.
#
# Usage:
#   fetch-table.sh <viewId> <pages> <output-prefix>
# Example:
#   fetch-table.sh 41549 7 .local/audits/coverity/raw/outstanding
#
# Coverity's UI uses two steps per page:
#   1) POST /views/table.json    {projectId, viewId, pageNum}    -- updates server view state
#   2) GET  /reports/table.json?projectId=X&viewId=Y             -- fetches rows for current state
#
# The script combines all pages into <prefix>-all.json (a flat array).
# Pages already on disk are skipped (idempotent).

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"
cov_load_env

VIEW_ID="${1:?usage: $0 <viewId> <pages> <output-prefix>}"
PAGES="${2:?usage: $0 <viewId> <pages> <output-prefix>}"
PREFIX="${3:?usage: $0 <viewId> <pages> <output-prefix>}"

# View IDs and page counts are positive integers. Reject anything else
# before the loop -- otherwise non-numeric PAGES would make `seq` produce
# no output, and the final jq merge would block waiting on stdin.
if [[ ! "${VIEW_ID}" =~ ^[1-9][0-9]*$ ]]; then
    echo -e "${COV_RED}[ERROR]${COV_NC} viewId must be a positive integer, got: '${VIEW_ID}'" >&2
    exit 1
fi
if [[ ! "${PAGES}" =~ ^[1-9][0-9]*$ ]]; then
    echo -e "${COV_RED}[ERROR]${COV_NC} pages must be a positive integer, got: '${PAGES}'" >&2
    exit 1
fi

mkdir -p "$(dirname "${PREFIX}")"

for page in $(seq 1 "${PAGES}"); do
    out="${PREFIX}-page${page}.json"
    if [[ -s "${out}" ]]; then
        echo -e "${COV_GRAY}[page ${page}/${PAGES}] cached ${out}${COV_NC}" >&2
        continue
    fi

    # Step 1: set server-side page state.
    post_body="{\"projectId\":${COVERITY_PROJECT_ID},\"viewId\":${VIEW_ID},\"pageNum\":${page}}"
    post_http=$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
        -H "accept: application/json, text/plain, */*" \
        -H "content-type: application/json" \
        -H "origin: ${COVERITY_HOST}" \
        -H "referer: ${COVERITY_HOST}/" \
        -H "user-agent: ${COVERITY_USER_AGENT}" \
        -H "x-xsrf-token: ${COVERITY_XSRF}" \
        -b "${COVERITY_COOKIE}" \
        --data-raw "${post_body}" \
        "${COVERITY_HOST}/views/table.json")
    if [[ "${post_http}" != "200" ]]; then
        echo -e "${COV_RED}[page ${page}] POST failed HTTP=${post_http}${COV_NC}" >&2
        exit 2
    fi

    # Step 2: fetch the now-current page.
    # Capture exit status of curl explicitly so a transport failure (timeout,
    # connection reset, DNS) doesn't leave a non-empty partial file behind
    # that the next run would treat as cached.
    get_http=$(curl -sS -o "${out}" -w '%{http_code}' \
        -H "accept: application/json, text/plain, */*" \
        -H "referer: ${COVERITY_HOST}/" \
        -H "user-agent: ${COVERITY_USER_AGENT}" \
        -b "${COVERITY_COOKIE}" \
        "${COVERITY_HOST}/reports/table.json?projectId=${COVERITY_PROJECT_ID}&viewId=${VIEW_ID}" \
        || echo "000")
    if [[ "${get_http}" != "200" ]]; then
        echo -e "${COV_RED}[page ${page}] GET failed HTTP=${get_http}${COV_NC}" >&2
        rm -f "${out}"
        exit 3
    fi
    # Validate response is JSON with the expected shape -- a Cloudflare
    # challenge or a 200-with-HTML-body would otherwise be cached as
    # 'valid' and confuse subsequent runs.
    if ! jq -e '.resultSet.results | type == "array"' "${out}" >/dev/null 2>&1; then
        echo -e "${COV_RED}[page ${page}] response not in expected shape; first 200 chars:${COV_NC}" >&2
        head -c 200 "${out}" >&2; echo >&2
        rm -f "${out}"
        exit 3
    fi

    count=$(jq '.resultSet.results | length' "${out}")
    total=$(jq '.resultSet.totalCount' "${out}")
    echo -e "${COV_GRAY}[page ${page}/${PAGES}] fetched ${count} rows (total=${total})${COV_NC}" >&2
    sleep 0.3
done

combined="${PREFIX}-all.json"
# Build the explicit page-file list. A glob (`-page*.json`) would pick up
# stale page files from a previous larger fetch (e.g. running with PAGES=5
# after a prior run with PAGES=7 would merge 7 pages into "all"). Enumerate
# only the pages we asked for in this invocation.
page_files=()
for page in $(seq 1 "${PAGES}"); do
    page_files+=("${PREFIX}-page${page}.json")
done
jq -s '[.[].resultSet.results[]]' "${page_files[@]}" > "${combined}"
echo -e "${COV_GREEN}Combined $(jq 'length' "${combined}") defects into ${combined}${COV_NC}" >&2
