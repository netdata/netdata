#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

[ -n "${GITHUB_ACTIONS}" ] && echo "::group::Building self-extracting archive"

run cd "${NETDATA_SOURCE_PATH}" || exit 1

# -----------------------------------------------------------------------------
# find the netdata version

VERSION="$(git describe 2> /dev/null)"
if [ -z "${VERSION}" ]; then
  VERSION=$(cat packaging/version)
fi

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
  configs.signatures \
  system/netdata-init-d \
  system/netdata-lsb \
  system/netdata-openrc \
  system/netdata.logrotate \
  system/netdata.service \
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
# copy the SSL/TLS configuration and certificates from the build system

run cp -a /etc/ssl "${NETDATA_INSTALL_PATH}/share/ssl"

# -----------------------------------------------------------------------------
# remove the links to allow untaring the archive

run rm "${NETDATA_INSTALL_PATH}/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/bin" \
  "${NETDATA_INSTALL_PATH}/usr/sbin" \
  "${NETDATA_INSTALL_PATH}/usr/local"

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
  "netdata, the real-time performance and health monitoring system" \
  ./system/post-installer.sh

run rm "${NETDATA_MAKESELF_PATH}/makeself.lsm.tmp"

# -----------------------------------------------------------------------------
# copy it to the netdata build dir

FILE="netdata-${BUILDARCH}-${VERSION}.gz.run"

run mkdir -p artifacts
run mv "${NETDATA_INSTALL_PATH}.gz.run" "artifacts/${FILE}"

[ -f "netdata-${BUILDARCH}-latest.gz.run" ] && rm "netdata-${BUILDARCH}-latest.gz.run"
run ln -s "artifacts/${FILE}" "netdata-${BUILDARCH}-latest.gz.run"

if [ "${BUILDARCH}" = "x86_64" ]; then
  [ -f "netdata-latest.gz.run" ] && rm "netdata-latest.gz.run"
  run ln -s "artifacts/${FILE}" "netdata-latest.gz.run"
  [ -f "artifacts/netdata-${VERSION}.gz.run" ] && rm "netdata-${VERSION}.gz.run"
  run ln -s "./${FILE}" "artifacts/netdata-${VERSION}.gz.run"
fi

[ -n "${GITHUB_ACTIONS}" ] && echo "::endgroup::"

echo >&2 "Self-extracting installer moved to 'artifacts/${FILE}'"
