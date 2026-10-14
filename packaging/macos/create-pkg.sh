#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Build the native macOS package from an already-built pkg-kind build tree.
#
# Usage: create-pkg.sh <build-dir> <distribution> <resources-dir> \
#                     <scripts-dir> <identifier> <version> <output>
#
# Driven by the package-macos target NetdataMacOSPackage.cmake defines; the
# arguments arrive from the configured build, not from a person. CPack's
# productbuild generator is deliberately not used: with a monolithic
# (component-less) payload it writes a distribution that references no
# package at all, and it creates its component-package output directory
# inside the very root pkgbuild is scanning, so partial junk lands under
# /Contents in the payload. Driving pkgbuild and productbuild directly is
# deterministic and lets the payload pass the artifact gate as part of every
# package build.
#
# Signing and notarization come from the environment, never from the CMake
# cache, so credentials do not end up in the build tree:
#
#   NETDATA_MACOS_SIGN_APP_IDENTITY        Developer ID Application identity
#                                          (name or SHA-1) for every Mach-O
#   NETDATA_MACOS_SIGN_INSTALLER_IDENTITY  Developer ID Installer identity
#                                          (name or SHA-1) for the package
#   NETDATA_MACOS_NOTARY_KEY               path to the App Store Connect team
#                                          API key (AuthKey_<id>.p8)
#   NETDATA_MACOS_NOTARY_KEY_ID            that key's Key ID
#   NETDATA_MACOS_NOTARY_ISSUER            the team's Issuer ID
#
# With none of them set the package is built unsigned. With all of them set it
# is signed, notarized, stapled and verified. A partial set is an error: a
# half-configured release must not quietly ship unsigned or unnotarized.

set -eu

BUILD_DIR="${1:?usage: create-pkg.sh <build-dir> <distribution> <resources-dir> <scripts-dir> <identifier> <version> <output>}"
DISTRIBUTION="${2:?missing distribution}"
RESOURCES="${3:?missing resources dir}"
SCRIPTS="${4:?missing scripts dir}"
IDENTIFIER="${5:?missing identifier}"
VERSION="${6:?missing version}"
OUTPUT="${7:?missing output}"

SCRIPT_DIR="$(cd "$(dirname "${0}")" && pwd -P)"
WORK_DIR="${BUILD_DIR}/macos-pkg"
STAGING="${WORK_DIR}/root"
INNER_DIR="${WORK_DIR}/packages"
MACHO_LIST="${WORK_DIR}/macho.list"

SIGN_VARS="NETDATA_MACOS_SIGN_APP_IDENTITY NETDATA_MACOS_SIGN_INSTALLER_IDENTITY NETDATA_MACOS_NOTARY_KEY NETDATA_MACOS_NOTARY_KEY_ID NETDATA_MACOS_NOTARY_ISSUER"
sign_set=""
sign_missing=""
for var in ${SIGN_VARS}; do
    eval "value=\${${var}:-}"
    if [ -n "${value}" ]; then
        sign_set="${sign_set} ${var}"
    else
        sign_missing="${sign_missing} ${var}"
    fi
done
if [ -z "${sign_set}" ]; then
    SIGN=0
elif [ -z "${sign_missing}" ]; then
    SIGN=1
    if [ ! -r "${NETDATA_MACOS_NOTARY_KEY}" ]; then
        echo "create-pkg: NETDATA_MACOS_NOTARY_KEY is not a readable file: ${NETDATA_MACOS_NOTARY_KEY}" >&2
        exit 1
    fi
else
    echo "create-pkg: package signing is partially configured; also set:${sign_missing}" >&2
    exit 1
fi

echo "create-pkg: staging install tree"
rm -rf "${STAGING}" "${INNER_DIR}" "${OUTPUT}"
mkdir -p "${STAGING}" "${INNER_DIR}" "$(dirname "${OUTPUT}")"
DESTDIR="${STAGING}" cmake --install "${BUILD_DIR}" > "${WORK_DIR}/install.log" 2>&1

echo "create-pkg: running the payload artifact gate"
"${SCRIPT_DIR}/artifact-gate.sh" --prefix /opt/netdata "${STAGING}/opt/netdata"

if [ "${SIGN}" -eq 1 ]; then
    find "${STAGING}" -type f > "${WORK_DIR}/files.list"
    : > "${MACHO_LIST}"
    while IFS= read -r f; do
        case "$(file -b "${f}")" in
            Mach-O*object*) ;; # codesign does not sign object files
            Mach-O*) printf '%s\n' "${f}" >> "${MACHO_LIST}" ;;
            *) ;;
        esac
    done < "${WORK_DIR}/files.list"
    if [ ! -s "${MACHO_LIST}" ]; then
        echo "create-pkg: found no Mach-O files to sign under ${STAGING}" >&2
        exit 1
    fi

    # Signing identifiers are derived from file names, and codesign warns that
    # two programs must never share one.
    dups="$(sed 's|.*/||' "${MACHO_LIST}" | sort | uniq -d)"
    if [ -n "${dups}" ]; then
        echo "create-pkg: Mach-O files share a name, so their signing identifiers would collide: ${dups}" >&2
        exit 1
    fi

    echo "create-pkg: signing $(wc -l < "${MACHO_LIST}" | tr -d ' ') Mach-O files"
    # Notarization requires the hardened runtime and a secure timestamp.
    while IFS= read -r f; do
        codesign --force --sign "${NETDATA_MACOS_SIGN_APP_IDENTITY}" \
            --options runtime --timestamp \
            --identifier "${IDENTIFIER}.$(basename "${f}")" "${f}"
    done < "${MACHO_LIST}"

    team=""
    while IFS= read -r f; do
        codesign --verify --strict "${f}"
        info="$(codesign -dv "${f}" 2>&1)"
        f_team="$(printf '%s\n' "${info}" | sed -n 's/^TeamIdentifier=//p')"
        if ! printf '%s\n' "${info}" | grep -Eq '^CodeDirectory .*flags=0x[0-9a-f]+\([^)]*runtime' \
            || ! printf '%s\n' "${info}" | grep -q '^Timestamp=' \
            || ! printf '%s\n' "${info}" | grep -qxF "Identifier=${IDENTIFIER}.$(basename "${f}")" \
            || [ -z "${f_team}" ] || [ "${f_team}" = "not set" ]; then
            echo "create-pkg: ${f} lacks the hardened runtime, a secure timestamp, its identifier or a Team ID" >&2
            exit 1
        fi
        if [ -z "${team}" ]; then
            team="${f_team}"
        elif [ "${f_team}" != "${team}" ]; then
            echo "create-pkg: ${f} is signed by team ${f_team}, the rest by ${team}" >&2
            exit 1
        fi
    done < "${MACHO_LIST}"
fi

echo "create-pkg: building the component package"
pkgbuild \
    --root "${STAGING}" \
    --scripts "${SCRIPTS}" \
    --identifier "${IDENTIFIER}" \
    --version "${VERSION}" \
    --install-location / \
    "${INNER_DIR}/netdata-agent.pkg"

if [ "${SIGN}" -eq 1 ]; then
    set -- --sign "${NETDATA_MACOS_SIGN_INSTALLER_IDENTITY}" --timestamp
else
    set --
fi

echo "create-pkg: building the product archive"
productbuild \
    --distribution "${DISTRIBUTION}" \
    --package-path "${INNER_DIR}" \
    --resources "${RESOURCES}" \
    --version "${VERSION}" \
    --identifier "${IDENTIFIER}" \
    "$@" \
    "${OUTPUT}"

if [ "${SIGN}" -eq 1 ]; then
    # A signed package that is not notarized and verified must not be left
    # where it could be mistaken for a release candidate.
    trap 'rm -f "${OUTPUT}"' EXIT

    echo "create-pkg: notarizing (this waits for Apple, usually a few minutes)"
    result="${WORK_DIR}/notarization.plist"
    # notarytool's exit status is not documented to reflect the verdict, so the
    # verdict is read from its output.
    xcrun notarytool submit "${OUTPUT}" \
        --key "${NETDATA_MACOS_NOTARY_KEY}" \
        --key-id "${NETDATA_MACOS_NOTARY_KEY_ID}" \
        --issuer "${NETDATA_MACOS_NOTARY_ISSUER}" \
        --wait --timeout 1h --output-format plist > "${result}" || true
    status="$(plutil -extract status raw -o - "${result}" 2>/dev/null || true)"
    submission="$(plutil -extract id raw -o - "${result}" 2>/dev/null || true)"
    if [ "${status}" != "Accepted" ]; then
        echo "create-pkg: notarization did not succeed (status: ${status:-unknown}, submission: ${submission:-none})" >&2
        cat "${result}" >&2
        if [ -n "${submission}" ]; then
            xcrun notarytool log "${submission}" \
                --key "${NETDATA_MACOS_NOTARY_KEY}" \
                --key-id "${NETDATA_MACOS_NOTARY_KEY_ID}" \
                --issuer "${NETDATA_MACOS_NOTARY_ISSUER}" >&2 || true
        fi
        exit 1
    fi
    echo "create-pkg: notarized (submission ${submission})"

    # The ticket can take a little while to become available after acceptance.
    tries=0
    until xcrun stapler staple "${OUTPUT}"; do
        tries=$((tries + 1))
        if [ "${tries}" -ge 5 ]; then
            echo "create-pkg: could not staple the notarization ticket" >&2
            exit 1
        fi
        sleep 30
    done
    xcrun stapler validate "${OUTPUT}"

    sig="$(pkgutil --check-signature "${OUTPUT}")"
    if ! printf '%s\n' "${sig}" | grep -q "Developer ID Installer: .*(${team})"; then
        echo "create-pkg: the package is not signed by a Developer ID Installer of team ${team}:" >&2
        printf '%s\n' "${sig}" >&2
        exit 1
    fi
    assessment="$(spctl --assess --type install --verbose=2 "${OUTPUT}" 2>&1)" || {
        echo "create-pkg: Gatekeeper rejects the package:" >&2
        printf '%s\n' "${assessment}" >&2
        exit 1
    }
    if ! printf '%s\n' "${assessment}" | grep -q 'source=Notarized Developer ID'; then
        echo "create-pkg: Gatekeeper accepts the package, but not as notarized Developer ID software:" >&2
        printf '%s\n' "${assessment}" >&2
        exit 1
    fi
    trap - EXIT
    echo "create-pkg: signed and notarized for team ${team}"
fi

echo "create-pkg: ${OUTPUT}"
