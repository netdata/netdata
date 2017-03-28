#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

cd .. || exit 1

export CFLAGS="-O3 -static"
run ./netdata-installer.sh --install /opt --dont-wait --dont-start-it

mkdir -p "${NETDATA_INSTALL_PATH}/system"

cp \
    system/netdata-init-d \
    system/netdata-lsb \
    system/netdata-openrc \
    system/netdata.logrotate \
    system/netdata.service \
    "${NETDATA_INSTALL_PATH}/system/"

cp \
    makeself/post-installer.sh \
    makeself/install-or-update.sh \
    installer/functions.sh \
    "${NETDATA_INSTALL_PATH}/system/"
