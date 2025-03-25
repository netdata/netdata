#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

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

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
