#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    export CFLAGS="-static -O3"
else
    export CFLAGS="-static -O1 -ggdb -Wall -Wextra -Wformat-signedness -fstack-protector-all -D_FORTIFY_SOURCE=2 -DNETDATA_INTERNAL_CHECKS=1"
#    export CFLAGS="-static -O1 -ggdb -Wall -Wextra -Wformat-signedness"
fi

if [ ! -z "${NETDATA_INSTALL_PATH}" -a -d "${NETDATA_INSTALL_PATH}/etc" ]
    then
    # make sure we don't have an old etc path, so that the installer
    # will install all files without examining changes
    run mv "${NETDATA_INSTALL_PATH}/etc" "${NETDATA_INSTALL_PATH}/etc.new"
fi

run ./netdata-installer.sh --install "${NETDATA_INSTALL_PARENT}" \
    --dont-wait \
    --dont-start-it \
    ${NULL}

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]
then
    run strip ${NETDATA_INSTALL_PATH}/bin/netdata
    run strip ${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/apps.plugin
    run strip ${NETDATA_INSTALL_PATH}/usr/libexec/netdata/plugins.d/cgroup-network
fi
