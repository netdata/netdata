#!/usr/bin/env bash

. $(dirname "${0}")/../functions.sh "${@}" || exit 1



"${NETDATA_MAKESELF_PATH}/makeself.sh" \
    --gzip \
    --notemp \
    --header "${NETDATA_MAKESELF_PATH}/makeself-header.sh" \
    --lsm "${NETDATA_MAKESELF_PATH}/makeself.lsm" \
    --license "${NETDATA_MAKESELF_PATH}/makeself-license.txt" \
    --help-header "${NETDATA_MAKESELF_PATH}/makeself-help-header.txt" \
    "${NETDATA_INSTALL_PATH}" \
    "${NETDATA_INSTALL_PATH}.gz.run" \
    "netdata, the real-time performance and health monitoring system" \
    ./system/post-installer.sh \
    ${NULL}


