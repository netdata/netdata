#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Confirming that Netdata is statically linked" || true

# Ensure the netdata binary is in fact statically linked
if run readelf -l "${NETDATA_INSTALL_PATH}/bin/netdata" | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${NETDATA_INSTALL_PATH}/bin/netdata"
  ldd "${NETDATA_INSTALL_PATH}/bin/netdata"
  exit 1
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
