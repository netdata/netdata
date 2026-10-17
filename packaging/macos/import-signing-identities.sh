#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Make Netdata's macOS signing credentials available to create-pkg.sh on a
# build machine, then remove them again.
#
# Usage: import-signing-identities.sh <work-dir>           import
#        import-signing-identities.sh --remove <work-dir>  clean up
#
# Import reads the credentials from the environment:
#
#   MACOS_APP_P12                base64 .p12: Developer ID Application
#   MACOS_APP_P12_PASSWORD       its export password
#   MACOS_INSTALLER_P12          base64 .p12: Developer ID Installer
#   MACOS_INSTALLER_P12_PASSWORD its export password
#   MACOS_NOTARY_KEY_P8          App Store Connect team API key (.p8 text)
#   MACOS_NOTARY_KEY_ID          its Key ID
#   MACOS_NOTARY_ISSUER          the team's Issuer ID
#
# It puts both identities into a throwaway keychain inside <work-dir>, adds
# Apple's Developer ID G2 intermediate (build machines without Xcode's
# certificates cannot otherwise build a trusted chain), writes the API key
# next to it, and prints the NETDATA_MACOS_* settings create-pkg.sh reads, as
# NAME=value lines suitable for $GITHUB_ENV. Diagnostics go to stderr.

set -eu
umask 077

G2_URL="https://www.apple.com/certificateauthority/DeveloperIDG2CA.cer"
G2_SHA256="f16cd3c54c7f83cea4bf1a3e6a0819c8aaa8e4a1528fd144715f350643d2df3a"
G2_SHA1="5B45F61068B29FCC8FFFF1A7E99B78DA9E9C4635"

usage() {
    echo "usage: $0 <work-dir> | --remove <work-dir>" >&2
    exit 2
}

REMOVE=0
if [ "${1:-}" = "--remove" ]; then
    REMOVE=1
    shift
fi
[ $# -eq 1 ] || usage
WORK_DIR="${1}"
KEYCHAIN="${WORK_DIR}/netdata-signing.keychain-db"
FILES="${WORK_DIR}/netdata-signing"

if [ "${REMOVE}" -eq 1 ]; then
    # The API key goes first, so a failing keychain deletion cannot leave it behind.
    rm -rf "${FILES}"
    if [ -e "${KEYCHAIN}" ]; then
        security delete-keychain "${KEYCHAIN}"
    fi
    echo "import-signing-identities: removed" >&2
    exit 0
fi

for var in MACOS_APP_P12 MACOS_APP_P12_PASSWORD MACOS_INSTALLER_P12 MACOS_INSTALLER_P12_PASSWORD \
           MACOS_NOTARY_KEY_P8 MACOS_NOTARY_KEY_ID MACOS_NOTARY_ISSUER; do
    eval "value=\${${var}:-}"
    if [ -z "${value}" ]; then
        echo "import-signing-identities: ${var} is not set" >&2
        exit 1
    fi
done
# Both values end up in NAME=value lines and in a file name.
if ! printf '%s\n' "${MACOS_NOTARY_KEY_ID}" | grep -Eqx '[A-Z0-9]{10}'; then
    echo "import-signing-identities: MACOS_NOTARY_KEY_ID is not a 10-character App Store Connect Key ID" >&2
    exit 1
fi
if ! printf '%s\n' "${MACOS_NOTARY_ISSUER}" | grep -Eqx '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'; then
    echo "import-signing-identities: MACOS_NOTARY_ISSUER is not an Issuer ID (lowercase UUID)" >&2
    exit 1
fi
if [ -e "${KEYCHAIN}" ] || [ -e "${FILES}" ]; then
    echo "import-signing-identities: ${WORK_DIR} already holds signing material; run --remove first" >&2
    exit 1
fi

mkdir -p "${FILES}"
printf '%s' "${MACOS_APP_P12}" | base64 --decode > "${FILES}/application.p12"
printf '%s' "${MACOS_INSTALLER_P12}" | base64 --decode > "${FILES}/installer.p12"

# The keychain password only has to outlive this script: the keychain stays
# unlocked (no timeout) until --remove deletes it.
keychain_password="$(openssl rand -hex 32)"
security create-keychain -p "${keychain_password}" "${KEYCHAIN}"
security set-keychain-settings "${KEYCHAIN}"
security unlock-keychain -p "${keychain_password}" "${KEYCHAIN}"
keychain_certs() {
    security find-certificate -a "${KEYCHAIN}" | sed -n 's/^ *"labl"<blob>="\(.*\)"$/\1/p' | paste -sd ';' -
}
echo "import-signing-identities: importing the Developer ID Application identity" >&2
security import "${FILES}/application.p12" -k "${KEYCHAIN}" -f pkcs12 -P "${MACOS_APP_P12_PASSWORD}" \
    -T /usr/bin/codesign >/dev/null
echo "import-signing-identities: keychain holds: $(keychain_certs)" >&2
echo "import-signing-identities: importing the Developer ID Installer identity" >&2
security import "${FILES}/installer.p12" -k "${KEYCHAIN}" -f pkcs12 -P "${MACOS_INSTALLER_P12_PASSWORD}" \
    -T /usr/bin/productbuild >/dev/null
echo "import-signing-identities: keychain holds: $(keychain_certs)" >&2
# A .p12 exported from a Mac that has the intermediate already carries it.
if security find-certificate -a -Z "${KEYCHAIN}" | grep -q "^SHA-1 hash: ${G2_SHA1}$"; then
    echo "import-signing-identities: the Developer ID G2 intermediate came with the identities" >&2
else
    echo "import-signing-identities: importing the Developer ID G2 intermediate" >&2
    curl -fsSL --retry 3 -o "${FILES}/DeveloperIDG2CA.cer" "${G2_URL}"
    if [ "$(shasum -a 256 "${FILES}/DeveloperIDG2CA.cer" | cut -d' ' -f1)" != "${G2_SHA256}" ]; then
        echo "import-signing-identities: ${G2_URL} does not match the pinned SHA-256" >&2
        exit 1
    fi
    security import "${FILES}/DeveloperIDG2CA.cer" -k "${KEYCHAIN}" >/dev/null
fi
# Without this, codesign and productbuild stop at an access prompt nobody can
# answer on a build machine.
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "${keychain_password}" "${KEYCHAIN}" >/dev/null
rm -f "${FILES}/application.p12" "${FILES}/installer.p12"

# codesign and productbuild look identities up in the search list. The current
# entries are kept one per line, so paths with spaces survive.
security list-keychains -d user | sed -e 's/^ *"//' -e 's/" *$//' > "${FILES}/search-list"
set -- "${KEYCHAIN}"
while IFS= read -r entry; do
    [ -n "${entry}" ] && set -- "$@" "${entry}"
done < "${FILES}/search-list"
security list-keychains -d user -s "$@"

# Exactly one valid identity of each kind, from the same team.
app="$(security find-identity -v -p codesigning "${KEYCHAIN}" | grep '"Developer ID Application: ' || true)"
installer="$(security find-identity -v "${KEYCHAIN}" | grep '"Developer ID Installer: ' || true)"
if [ "$(printf '%s' "${app}" | grep -c .)" -ne 1 ] || [ "$(printf '%s' "${installer}" | grep -c .)" -ne 1 ]; then
    echo "import-signing-identities: expected one valid Developer ID Application and one Developer ID Installer identity, found:" >&2
    security find-identity "${KEYCHAIN}" >&2
    exit 1
fi
app_team="$(printf '%s\n' "${app}" | sed -n 's/.*(\([A-Z0-9]*\))"$/\1/p')"
installer_team="$(printf '%s\n' "${installer}" | sed -n 's/.*(\([A-Z0-9]*\))"$/\1/p')"
if [ -z "${app_team}" ] || [ "${app_team}" != "${installer_team}" ]; then
    echo "import-signing-identities: the identities belong to different teams (${app_team:-?} / ${installer_team:-?})" >&2
    exit 1
fi

key_file="${FILES}/AuthKey_${MACOS_NOTARY_KEY_ID}.p8"
printf '%s\n' "${MACOS_NOTARY_KEY_P8}" > "${key_file}"

echo "import-signing-identities: imported Developer ID identities of team ${app_team}" >&2
echo "NETDATA_MACOS_SIGN_APP_IDENTITY=$(printf '%s\n' "${app}" | awk '{print $2}')"
echo "NETDATA_MACOS_SIGN_INSTALLER_IDENTITY=$(printf '%s\n' "${installer}" | awk '{print $2}')"
echo "NETDATA_MACOS_NOTARY_KEY=${key_file}"
echo "NETDATA_MACOS_NOTARY_KEY_ID=${MACOS_NOTARY_KEY_ID}"
echo "NETDATA_MACOS_NOTARY_ISSUER=${MACOS_NOTARY_ISSUER}"
