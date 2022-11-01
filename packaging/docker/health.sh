#!/bin/sh
#
# This is the script that gets run for our Docker image health checks.

if [ -z "${NETDATA_HEALTHCHECK_TARGET}" ] ; then
    # If users didn't request something else, query `/api/v1/info`.
    PORT=${NETDATA_LISTENER_PORT:-19999}
    NETDATA_HEALTHCHECK_TARGET="http://localhost:${PORT}/api/v1/info"
fi

case "${NETDATA_HEALTHCHECK_TARGET}" in
    cli)
        netdatacli ping || exit 1
        ;;
    *)
        curl -sSL "${NETDATA_HEALTHCHECK_TARGET}" || exit 1
        ;;
esac
