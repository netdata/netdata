#!/usr/bin/env bash
#
# Entry point script for netdata
#
# Copyright: SPDX-License-Identifier: GPL-3.0-or-later
#
# Author  : Pavlos Emm. Katsoulakis <paul@netdata.cloud>
set -e

echo "Netdata entrypoint script starting"
if [ ${RESCRAMBLE+x} ]; then
	echo "Reinstalling all packages to get the latest Polymorphic Linux scramble"
	apk upgrade --update-cache --available
fi

if [ -n "${PGID}" ]; then
    echo "Creating docker group ${PGID}"
    addgroup -g "${PGID}" "docker" || echo >&2 "Could not add group docker with ID ${PGID}, its already there probably"
    echo "Assign netdata user to docker group ${PGID}"
    usermod -a -G ${PGID} ${DOCKER_USR} || echo >&2 "Could not add netdata user to group docker with ID ${PGID}"
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_PORT}" -W set web "web files group" root -W set web "web files owner" root "$@"

echo "Netdata entrypoint script, completed!"
