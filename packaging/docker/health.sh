#!/bin/sh
#
# This is the script that gets run for our Docker image health checks.

if [ -z "${NETDATA_HEALTH_CHECK}" ] ; then
    # If users didn't request something else, query `/api/v1/info`.
    NETDATA_HEALTH_CHECK="http://localhost:19999/api/v1/info"
fi

case "${NETDATA_HEALTH_CHECK}" in
    cli)
        netdatacli ping || exit 1
        ;;
    *)
        curl -sS "${NETDATA_HEALTH_CHECK}" || exit 1
        ;;
esac
