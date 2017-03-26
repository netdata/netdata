#!/usr/bin/env bash

. ${NETDATA_MAKESELF_PATH}/functions.sh "${@}" || exit 1

run cd "${NETDATA_INSTALL_PATH}"
run ln -s etc/netdata netdata-configs
run ln -s usr/share/netdata/web netdata-web-files
run ln -s usr/libexec/netdata netdata-plugins
run ln -s var/lib/netdata netdata-dbs
run ln -s var/cache/netdata netdata-metrics
run ln -s var/log/netdata netdata-logs


