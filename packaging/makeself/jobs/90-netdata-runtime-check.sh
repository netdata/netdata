#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

dump_log() {
  cat ./netdata.log
}

trap dump_log EXIT

"${NETDATA_INSTALL_PATH}/bin/netdata" -D > ./netdata.log 2>&1 &

"${NETDATA_SOURCE_PATH}/packaging/runtime-check.sh" || exit 1

trap - EXIT
