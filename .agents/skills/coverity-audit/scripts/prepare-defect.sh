#!/usr/bin/env bash
# Bundle one Coverity defect into a per-CID working directory.
#
# Usage:
#   prepare-defect.sh [--force] <cid> [<scope>]
#
# <scope> defaults to "outstanding" -- it's only used as a subdirectory name
# for organizing artifacts, not a Coverity API parameter.
#
# Inputs (must already exist; produced by fetch-table.sh + fetch-details.sh):
#   .local/audits/coverity/raw/<scope>-all.json
#   .local/audits/coverity/details/<scope>/cid-<N>.json    (or details/cid-<N>.json)
#
# Outputs (under <audit-dir>/triage/<scope>/cid-<N>/):
#   defect-summary.json   -- the row from the table dump
#   defect-details.json   -- the per-defect details
#   source-context.c      -- ~150 lines around the main event in the flagged file
#   TODO.md               -- per-defect scratch (so review work doesn't pile at repo root)
#
# Idempotent. Re-running keeps existing files; pass --force to regenerate.
#
# This script does NOT prescribe a review pipeline. After preparing the bundle,
# how you triage it (single model, multiple models, manual review, etc.) is
# adhoc and should be agreed with the user.

set -euo pipefail

# shellcheck source=./_lib.sh
# shellcheck disable=SC1091
source "$(dirname "$0")/_lib.sh"

FORCE=0
if [[ "${1:-}" == "--force" ]]; then
    FORCE=1; shift
fi

CID="${1:?usage: $0 [--force] <cid> [<scope>]}"
SCOPE="${2:-outstanding}"

cov_require_numeric_cid "${CID}"

# Scope is used as a path component (.../triage/<scope>/cid-N/...). Reject
# anything that could path-escape; allow only simple lowercase identifiers.
if [[ ! "${SCOPE}" =~ ^[a-z][a-z0-9_-]*$ ]]; then
    echo -e "${COV_RED}[ERROR]${COV_NC} scope must match ^[a-z][a-z0-9_-]*\$ (got: '${SCOPE}')" >&2
    exit 1
fi

ROOT="$(cov_repo_root)"
AUDIT="$(cov_audit_dir)"

OUT_DIR="${AUDIT}/triage/${SCOPE}/cid-${CID}"
ROW_FILE="${AUDIT}/raw/${SCOPE}-all.json"
DETAILS_SRC="${AUDIT}/details/${SCOPE}/cid-${CID}.json"

if [[ ( ! -f "${DETAILS_SRC}" || ! -r "${DETAILS_SRC}" ) \
   && -f "${AUDIT}/details/cid-${CID}.json" \
   && -r "${AUDIT}/details/cid-${CID}.json" ]]; then
    DETAILS_SRC="${AUDIT}/details/cid-${CID}.json"
fi

if [[ ! -f "${ROW_FILE}" || ! -r "${ROW_FILE}" ]]; then
    echo -e "${COV_RED}Missing ${ROW_FILE}. Run fetch-table.sh first.${COV_NC}" >&2
    exit 1
fi
if [[ ! -f "${DETAILS_SRC}" || ! -r "${DETAILS_SRC}" ]]; then
    echo -e "${COV_RED}Missing ${DETAILS_SRC}. Run fetch-details.sh first.${COV_NC}" >&2
    exit 1
fi

mkdir -p "${OUT_DIR}"

# defect-summary.json -- CID is validated numeric above, --argjson is safe.
summary="${OUT_DIR}/defect-summary.json"
if [[ ! -s "${summary}" || "${FORCE}" == "1" ]]; then
    jq --argjson cid "${CID}" '.[] | select(.cid==$cid)' "${ROW_FILE}" > "${summary}"
    if [[ ! -s "${summary}" ]]; then
        echo -e "${COV_RED}CID ${CID} not found in ${ROW_FILE}. Wrong scope, or table is stale?${COV_NC}" >&2
        rm -f "${summary}"
        exit 2
    fi
fi

# defect-details.json
details="${OUT_DIR}/defect-details.json"
if [[ ! -s "${details}" || "${FORCE}" == "1" ]]; then
    cp "${DETAILS_SRC}" "${details}"
fi

# source-context.c
display_file="$(jq -r '.displayFile // ""' "${summary}")"
repo_file="${display_file#/}"  # strip leading slash; paths are repo-relative
main_line="$(jq -r '
    (.occurrences[0].eventSets[0].eventTree | map(select(.main==true))[0]
     // .occurrences[0].eventSets[0].eventTree[-1])
    | .lineNumber // 1
' "${details}")"

ctx_file="${OUT_DIR}/source-context.c"
if [[ ! -s "${ctx_file}" || "${FORCE}" == "1" ]]; then
    # Validation:
    # 1. Non-empty: an empty displayFile would resolve `${ROOT}/${repo_file}`
    #    to `${ROOT}/`, which IS a readable directory.
    # 2. No `..`: prevent path traversal escaping the repo. Coverity's
    #    displayFile is its source-tree path; legitimate values never
    #    contain `..`. Reject anything that does.
    # 3. Regular file + readable.
    repo_file_ok=1
    [[ -z "${repo_file}" ]] && repo_file_ok=0
    [[ "${repo_file}" == *..* ]] && repo_file_ok=0
    [[ -f "${ROOT}/${repo_file}" && -r "${ROOT}/${repo_file}" ]] || repo_file_ok=0
    if (( repo_file_ok )); then
        start=$((main_line - 100))
        (( start < 1 )) && start=1
        end=$((main_line + 50))
        {
            printf '// Extracted from %s (lines %d..%d; main event at line %d).\n' \
                "${repo_file}" "${start}" "${end}" "${main_line}"
            printf '// Line numbers are the original file line numbers.\n\n'
            awk -v s="${start}" -v e="${end}" 'NR>=s && NR<=e {printf "%5d  %s\n", NR, $0}' \
                "${ROOT}/${repo_file}"
        } > "${ctx_file}"
    else
        echo -e "${COV_YELLOW}Source ${repo_file} not in tree -- CODE_GONE candidate.${COV_NC}" >&2
        printf '// Source file %s not present in the current tree.\n// Candidate CODE_GONE.\n' \
            "${repo_file}" > "${ctx_file}"
    fi
fi

# TODO.md (so review work doesn't create a TODO at repo root)
per_todo="${OUT_DIR}/TODO.md"
if [[ ! -s "${per_todo}" || "${FORCE}" == "1" ]]; then
    short_type="$(jq -r '.displayType // "unknown"' "${summary}")"
    impact="$(jq -r '.displayImpact // "unspecified"' "${summary}")"
    cat > "${per_todo}" <<EOF
# CID ${CID} -- ${short_type} (${impact} impact)

File:  ${repo_file}
Line:  ${main_line}

Bundle:
- defect-summary.json
- defect-details.json
- source-context.c

(Use this file for per-defect notes, plan, decisions. Keep all per-defect
artifacts inside this directory.)
EOF
fi

echo -e "${COV_GREEN}Prepared ${OUT_DIR}${COV_NC}" >&2
ls -1 "${OUT_DIR}"
