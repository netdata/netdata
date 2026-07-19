#!/bin/sh
# Compare two directories of netdata RPMs — a reference set (built from
# netdata.spec.in via rpmbuild) and a candidate set (built via CPack) — for
# packaging parity: package set, header metadata, dependencies, per-file
# modes/ownership/flags/capabilities, scriptlets, and changelog.
#
# Usage: compare-rpms.sh <reference-dir> <candidate-dir> [allowlist]
#
# The optional allowlist is a file of extended regexes; unified-diff lines
# matching any of them are ignored. Use it for reviewed, intentionally
# accepted deviations only — every entry should carry a comment in the file
# explaining why the deviation is acceptable.
#
# Exit status: 0 on parity (modulo allowlist), 1 otherwise.

set -eu

REF_DIR="${1:?usage: compare-rpms.sh <reference-dir> <candidate-dir> [allowlist]}"
NEW_DIR="${2:?usage: compare-rpms.sh <reference-dir> <candidate-dir> [allowlist]}"
ALLOWLIST="${3:-}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT INT TERM

# Render one RPM into a normalized text report.
report_rpm() {
    _pkg="$1"

    rpm -qp --qf 'Name: %{NAME}\nVersion: %{VERSION}\nRelease: %{RELEASE}\nArch: %{ARCH}\nLicense: %{LICENSE}\nGroup: %{GROUP}\nURL: %{URL}\nSummary: %{SUMMARY}\n' "${_pkg}" 2>/dev/null

    for _tag in requires provides conflicts obsoletes recommends suggests supplements enhances; do
        echo "== ${_tag}"
        rpm -qp "--${_tag}" "${_pkg}" 2>/dev/null | sort
    done

    echo "== files"
    rpm -qp --qf '[%{FILEMODES:octal}|%{FILEUSERNAME}:%{FILEGROUPNAME}|%{FILEFLAGS:fflags}|%{FILECAPS}|%{FILENAMES}\n]' "${_pkg}" 2>/dev/null | sort -t'|' -k5

    echo "== scripts"
    rpm -qp --scripts "${_pkg}" 2>/dev/null

    echo "== changelog"
    rpm -qp --changelog "${_pkg}" 2>/dev/null
}

render_dir() {
    _dir="$1"
    _out="$2"
    mkdir -p "${_out}"

    for _pkg in "${_dir}"/*.rpm; do
        [ -e "${_pkg}" ] || continue
        _name="$(rpm -qp --qf '%{NAME}' "${_pkg}" 2>/dev/null)"
        report_rpm "${_pkg}" > "${_out}/${_name}.report"
        echo "${_name}"
    done | sort > "${_out}/.package-set"
}

render_dir "${REF_DIR}" "${WORK_DIR}/ref"
render_dir "${NEW_DIR}" "${WORK_DIR}/new"

failed=0

if ! diff -u "${WORK_DIR}/ref/.package-set" "${WORK_DIR}/new/.package-set" > "${WORK_DIR}/package-set.diff"; then
    echo "PACKAGE SET MISMATCH:"
    cat "${WORK_DIR}/package-set.diff"
    failed=1
fi

# Comment and empty lines must not reach grep -f (an empty pattern matches
# every line).
ALLOW_PATTERNS="${WORK_DIR}/allow.patterns"
if [ -n "${ALLOWLIST}" ]; then
    grep -vE '^(#|[[:space:]]*$)' "${ALLOWLIST}" > "${ALLOW_PATTERNS}" || true
fi

filter_allowed() {
    # Keep the +++/--- file headers out of allowlist matching, but keep every
    # real +/- body line, including ones whose content itself starts with +
    # or -; a hunk whose every +/- line is allowlisted counts as clean.
    if [ -s "${ALLOW_PATTERNS}" ]; then
        grep -vE '^(\+\+\+ |--- )' | { grep -E '^[+-]' || true; } | { grep -vEf "${ALLOW_PATTERNS}" || true; }
    else
        grep -vE '^(\+\+\+ |--- )' | { grep -E '^[+-]' || true; }
    fi
}

# Compare the union of both package sets so a candidate-only package still
# gets its report shown (the package-set diff above already fails the run).
sort -u "${WORK_DIR}/ref/.package-set" "${WORK_DIR}/new/.package-set" > "${WORK_DIR}/.package-union"

while IFS= read -r _name; do
    _ref="${WORK_DIR}/ref/${_name}.report"
    _new="${WORK_DIR}/new/${_name}.report"
    [ -e "${_ref}" ] || _ref=/dev/null
    [ -e "${_new}" ] || _new=/dev/null

    if ! diff -u "${_ref}" "${_new}" > "${WORK_DIR}/${_name}.diff"; then
        _residual="$(filter_allowed < "${WORK_DIR}/${_name}.diff")"
        if [ -n "${_residual}" ]; then
            echo "PARITY MISMATCH: ${_name}"
            cat "${WORK_DIR}/${_name}.diff"
            echo
            failed=1
        fi
    fi
done < "${WORK_DIR}/.package-union"

if [ "${failed}" -eq 0 ]; then
    echo "PARITY OK: $(wc -l < "${WORK_DIR}/ref/.package-set") packages match."
fi

exit "${failed}"
