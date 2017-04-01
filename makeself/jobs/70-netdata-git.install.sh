#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

export CFLAGS="-O3 -static"

run ./netdata-installer.sh --install "${NETDATA_INSTALL_PARENT}" \
    --dont-wait \
    --dont-start-it \
    ${NULL}

