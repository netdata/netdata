#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Build the native macOS package from an already-built pkg-kind build tree.
#
# Usage: create-pkg.sh <build-dir> <distribution> <resources-dir> \
#                     <identifier> <version> <output>
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

set -eu

BUILD_DIR="${1:?usage: create-pkg.sh <build-dir> <distribution> <resources-dir> <identifier> <version> <output>}"
DISTRIBUTION="${2:?missing distribution}"
RESOURCES="${3:?missing resources dir}"
IDENTIFIER="${4:?missing identifier}"
VERSION="${5:?missing version}"
OUTPUT="${6:?missing output}"

SCRIPT_DIR="$(cd "$(dirname "${0}")" && pwd -P)"
STAGING="${BUILD_DIR}/macos-pkg/root"
INNER_DIR="${BUILD_DIR}/macos-pkg/packages"

echo "create-pkg: staging install tree"
rm -rf "${STAGING}" "${INNER_DIR}"
mkdir -p "${STAGING}" "${INNER_DIR}" "$(dirname "${OUTPUT}")"
DESTDIR="${STAGING}" cmake --install "${BUILD_DIR}" > "${BUILD_DIR}/macos-pkg/install.log" 2>&1

echo "create-pkg: running the payload artifact gate"
"${SCRIPT_DIR}/artifact-gate.sh" --prefix /opt/netdata "${STAGING}/opt/netdata"

echo "create-pkg: building the component package"
pkgbuild \
    --root "${STAGING}" \
    --identifier "${IDENTIFIER}" \
    --version "${VERSION}" \
    --install-location / \
    "${INNER_DIR}/netdata-agent.pkg"

echo "create-pkg: building the product archive"
productbuild \
    --distribution "${DISTRIBUTION}" \
    --package-path "${INNER_DIR}" \
    --resources "${RESOURCES}" \
    --version "${VERSION}" \
    --identifier "${IDENTIFIER}" \
    "${OUTPUT}"

echo "create-pkg: ${OUTPUT}"
