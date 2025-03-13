#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Copying archive files" || true

run cd "${NETDATA_SOURCE_PATH}" || exit 1

VERSION="$("${NETDATA_INSTALL_PARENT}/netdata/bin/netdata" -v | cut -f 2 -d ' ')"
[ -z "${VERSION}" ] && VERSION="$(cat "${NETDATA_SOURCE_PATH}/packaging/version")"

if [ "${VERSION}" == "" ]; then
  echo >&2 "Cannot find version number. Create makeself executable from source code with git tree structure."
  exit 1
fi

FILE="netdata-${BUILDARCH}-${VERSION}.gz.run"

run mv "${NETDATA_INSTALL_PATH}.gz.run" "${NETDATA_SOURCE_PATH}/artifacts/${FILE}"

[ -f "netdata-${BUILDARCH}-latest.gz.run" ] && rm "netdata-${BUILDARCH}-latest.gz.run"
run cp "${NETDATA_SOURCE_PATH}/artifacts/${FILE}" "netdata-${BUILDARCH}-latest.gz.run"

if [ "${BUILDARCH}" = "x86_64" ]; then
  [ -f "netdata-latest.gz.run" ] && rm "netdata-latest.gz.run"
  run cp "${NETDATA_SOURCE_PATH}/artifacts/${FILE}" "netdata-latest.gz.run"
  [ -f "${NETDATA_SOURCE_PATH}/artifacts/netdata-${VERSION}.gz.run" ] && rm "netdata-${VERSION}.gz.run"
  run cp "${NETDATA_SOURCE_PATH}/artifacts/${FILE}" "${NETDATA_SOURCE_PATH}/artifacts/netdata-${VERSION}.gz.run"
fi

chown -R "$(stat -c '%u:%g' "${NETDATA_SOURCE_PATH}")" "${NETDATA_SOURCE_PATH}/artifacts"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
