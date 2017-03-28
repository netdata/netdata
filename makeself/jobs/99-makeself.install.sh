#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

"${NETDATA_MAKESELF_PATH}/makeself.sh" \
    --gzip \
    --notemp \
    --header "${NETDATA_MAKESELF_PATH}/makeself-header.sh" \
    --lsm "${NETDATA_MAKESELF_PATH}/makeself.lsm" \
    --license "${NETDATA_MAKESELF_PATH}/makeself-license.txt" \
    --help-header "${NETDATA_MAKESELF_PATH}/makeself-help-header.txt" \
    "${NETDATA_INSTALL_PATH}" \
    "${NETDATA_INSTALL_PATH}.gz.run" \
    "LABEL: netdata, real-time performance monitoring" \
    ./system/install-or-update.sh \
    ${NULL}


