#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

set -x

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Building self-extracting archive" || true

run cd "${NETDATA_SOURCE_PATH}" || exit 1

# -----------------------------------------------------------------------------
# find the netdata version

VERSION="$("${NETDATA_INSTALL_PARENT}/netdata/bin/netdata" -v | cut -f 2 -d ' ')"
[ -z "${VERSION}" ] && VERSION="$(cat "${NETDATA_SOURCE_PATH}/packaging/version")"

if [ "${VERSION}" == "" ]; then
  echo >&2 "Cannot find version number. Create makeself executable from source code with git tree structure."
  exit 1
fi

# -----------------------------------------------------------------------------
# copy the files needed by makeself installation

run mkdir -p "${NETDATA_INSTALL_PATH}/system"

run cp \
  packaging/makeself/post-installer.sh \
  packaging/makeself/install-or-update.sh \
  packaging/installer/functions.sh \
  "${NETDATA_INSTALL_PATH}/system/"

# -----------------------------------------------------------------------------
# create a wrapper to start our netdata with a modified path

run mkdir -p "${NETDATA_INSTALL_PATH}/bin/srv"

run mv "${NETDATA_INSTALL_PATH}/bin/netdata" \
  "${NETDATA_INSTALL_PATH}/bin/srv/netdata" || exit 1

cat > "${NETDATA_INSTALL_PATH}/bin/netdata" << EOF
#!${NETDATA_INSTALL_PATH}/bin/bash
export NETDATA_BASH_LOADABLES="DISABLE"
export PATH="${NETDATA_INSTALL_PATH}/bin:\${PATH}"
exec "${NETDATA_INSTALL_PATH}/bin/srv/netdata" "\${@}"
EOF
run chmod 755 "${NETDATA_INSTALL_PATH}/bin/netdata"

# -----------------------------------------------------------------------------
# the claiming script must be in the same directory as the netdata binary for web-based claiming to work

run ln -s "${NETDATA_INSTALL_PATH}/bin/netdata-claim.sh" \
  "${NETDATA_INSTALL_PATH}/bin/srv/netdata-claim.sh" || exit 1

# -----------------------------------------------------------------------------
# copy the SSL/TLS configuration and certificates from the build system

run cp -a /etc/ssl "${NETDATA_INSTALL_PATH}/share/ssl"

# -----------------------------------------------------------------------------
# remove the links to allow untaring the archive

run rm "${NETDATA_INSTALL_PATH}/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/bin" \
  "${NETDATA_INSTALL_PATH}/usr/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/local"

# -----------------------------------------------------------------------------
# ensure required directories actually exist

for dir in var/lib/netdata var/cache/netdata var/log/netdata ; do
    run mkdir -p "${NETDATA_INSTALL_PATH}/${dir}"
    run touch "${NETDATA_INSTALL_PATH}/${dir}/.keep"
done

# -----------------------------------------------------------------------------
# create the makeself archive

run sed "s|NETDATA_VERSION|${VERSION}|g" < "${NETDATA_MAKESELF_PATH}/makeself.lsm" > "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp"

run "${NETDATA_MAKESELF_PATH}/makeself.sh" \
  --gzip \
  --complevel 9 \
  --notemp \
  --needroot \
  --target "${NETDATA_INSTALL_PATH}" \
  --header "${NETDATA_MAKESELF_PATH}/makeself-header.sh" \
  --lsm "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp" \
  --license "${NETDATA_MAKESELF_PATH}/makeself-license.txt" \
  --help-header "${NETDATA_MAKESELF_PATH}/makeself-help-header.txt" \
  "${NETDATA_INSTALL_PATH}" \
  "${NETDATA_INSTALL_PATH}.gz.run" \
  "Netdata, X-Ray Vision for your infrastructure" \
  ./system/post-installer.sh

run rm "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp"

# -----------------------------------------------------------------------------
# copy it to the netdata build dir

FILE="netdata-${BUILDARCH}-${VERSION}.gz.run"

run mkdir -p artifacts
run mv "${NETDATA_INSTALL_PATH}.gz.run" "artifacts/${FILE}"

[ -f "netdata-${BUILDARCH}-latest.gz.run" ] && rm "netdata-${BUILDARCH}-latest.gz.run"
run cp "artifacts/${FILE}" "netdata-${BUILDARCH}-latest.gz.run"

if [ "${BUILDARCH}" = "x86_64" ]; then
  [ -f "netdata-latest.gz.run" ] && rm "netdata-latest.gz.run"
  run cp "artifacts/${FILE}" "netdata-latest.gz.run"
  [ -f "artifacts/netdata-${VERSION}.gz.run" ] && rm "netdata-${VERSION}.gz.run"
  run cp "artifacts/${FILE}" "artifacts/netdata-${VERSION}.gz.run"
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true

echo >&2 "Self-extracting installer moved to 'artifacts/${FILE}'"
