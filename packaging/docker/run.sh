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

create_group_and_assign_to_user() {
	local local_DOCKER_GROUP="$1"
	local local_DOCKER_GID="$2"
	local local_DOCKER_USR="$3"

	echo >&2 "Adding group with ID ${local_DOCKER_GID} and name '${local_DOCKER_GROUP}'"
	addgroup -g "${local_DOCKER_GID}" "${local_DOCKER_GROUP}" || echo >&2 "Could not add group ${local_DOCKER_GROUP} with ID ${local_DOCKER_GID}, its already there probably"

	echo >&2 "Adding user '${local_DOCKER_USR}' to group '${local_DOCKER_GROUP}/${local_DOCKER_GID}'"
	sed -i "s/:${local_DOCKER_GID}:$/:${local_DOCKER_GID}:${local_DOCKER_USR}/g" /etc/group

	# Make sure we use the right docker group
	GRP_TO_ASSIGN="$(grep ":x:${local_DOCKER_GID}:" /etc/group | cut -d':' -f1)"
	if [ -z "${GRP_TO_ASSIGN}" ]; then
		echo >&2 "Could not find group ID ${local_DOCKER_GID} in /etc/group. Check your logs and report it if this is an unrecovereable error"
	else
		echo >&2 "Group creation and assignment completed, netdata was assigned to group ${GRP_TO_ASSIGN}/${local_DOCKER_GID}"
		echo "${GRP_TO_ASSIGN}"
	fi
}

DOCKER_USR="netdata"
DOCKER_SOCKET="/var/run/docker.sock"
DOCKER_GROUP="docker"

if [ -S "${DOCKER_SOCKET}" ] && [ -n "${PGID}" ]; then
	GRP=$(create_group_and_assign_to_user "${DOCKER_GROUP}" "${PGID}" "${DOCKER_USR}")
	if [ -n "${GRP}" ]; then
		echo "Adjusting ownership of mapped docker socket '${DOCKER_SOCKET}' to root:${GRP}"
		chown "root:${GRP}" "${DOCKER_SOCKET}"
	fi
fi

exec /usr/sbin/netdata -u "${DOCKER_USR}" -D -s /host -p "${NETDATA_PORT}" "$@"

echo "Netdata entrypoint script, completed!"
