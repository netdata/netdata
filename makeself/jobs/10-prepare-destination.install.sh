#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0+

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

[ -d "${NETDATA_INSTALL_PATH}.old" ] && run rm -rf "${NETDATA_INSTALL_PATH}.old"
[ -d "${NETDATA_INSTALL_PATH}" ] && run mv -f "${NETDATA_INSTALL_PATH}" "${NETDATA_INSTALL_PATH}.old"

run mkdir -p "${NETDATA_INSTALL_PATH}/bin"
run mkdir -p "${NETDATA_INSTALL_PATH}/usr"
run cd "${NETDATA_INSTALL_PATH}"
run ln -s bin sbin
run cd "${NETDATA_INSTALL_PATH}/usr"
run ln -s ../bin bin
run ln -s ../sbin sbin
run ln -s . local

