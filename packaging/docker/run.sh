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

DOCKER_USR="netdata"
DOCKER_SOCKET="/var/run/docker.sock"
DOCKER_GROUP="docker"

if [ -S "${DOCKER_SOCKET}" ] && [ -n "${PGID}" ]; then
	echo "Adding group with ID ${PGID} and name '${DOCKER_GROUP}'"
	addgroup -g "${PGID}" "${DOCKER_GROUP}"

	echo "Adding user '${DOCKER_USR}' to group '${DOCKER_GROUP}'"
	sed -i "s/${DOCKER_GID}:$/${DOCKER_GID}:${DOCKER_USR}/g" /etc/group

	echo "Adjusting ownership of mapped docker socket '${DOCKER_SOCKET}'"
	chown "root:${DOCKER_GROUP}" "${DOCKER_SOCKET}"
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_PORT}" "$@"

echo "Netdata entrypoint script, completed!"
