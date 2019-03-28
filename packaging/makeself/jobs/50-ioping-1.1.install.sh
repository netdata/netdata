#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

. $(dirname "${0}")/../functions.sh "${@}" || exit 1

if [ -d ioping-netdata ]
    then
    run rm -rf ioping-netdata || exit 1
fi

run git clone https://github.com/netdata/ioping.git ioping-netdata

run cd ioping-netdata || exit 1

export CFLAGS="-static"

run make clean
run make -j${SYSTEM_CPUS}
run mkdir -p ${NETDATA_INSTALL_PATH}/bin/
run install -m 0755 ioping ${NETDATA_INSTALL_PATH}/bin/

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    run strip ${NETDATA_INSTALL_PATH}/bin/ioping
fi
