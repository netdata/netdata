#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

fetch "ioping-1.2" "https://github.com/koct9i/ioping/archive/v1.2.tar.gz"

export CFLAGS="-static"

run make clean
run make -j "$(nproc)"
run mkdir -p "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/
run install -o root -g root -m 4750 ioping "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/ioping
fi
